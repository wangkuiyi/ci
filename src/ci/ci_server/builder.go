// The utilities for running ci scripts.
package ci_server

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"text/template"
)

const ExitFlag = -1

// Builder. Start multiple go routine to executing ci scripts for each builds.
// For each build, builder will generate an shell script for execution. Then just execute this shell script.
type Builder struct {
	jobChan chan int64 // channel for build id

	opt *BuildOption // the build options.

	bootstrapTpl      *template.Template // the build bootstrap template, including setting environment, etc.
	pushEventCloneTpl *template.Template // git clone template for push event.
	execTpl           *template.Template // execute ci scripts template.
	cleanTpl          *template.Template // clean template. clean the building workspace.
	exitGroup         sync.WaitGroup     // wait group for Close builder. waiting all go routine to exit.

	db     *CIDB      // database
	github *GithubAPI // github api
}

// New builder instance.
// It will create the building directory for each go routine. The building dir can be configured in configuration file.
func newBuilder(jobChan chan int64, opts *Options, db *CIDB, github *GithubAPI) (builder *Builder, err error) {
	for i := 0; i < opts.Build.Concurrent; i++ {
		path := fmt.Sprintf("%s%c%d", opts.Build.Dir, os.PathSeparator, i)
		err = os.MkdirAll(path, 0755)
		if err != nil {
			return
		}
	}

	builder = &Builder{
		jobChan: jobChan,
		opt:     &opts.Build,
		db:      db,
		github:  github,
	}

	builder.bootstrapTpl, err = template.New("bootstrap").Parse(opts.Build.BootstrapTpl)
	if err != nil {
		return
	}
	builder.pushEventCloneTpl, err = template.New("pushEvent").Parse(opts.Build.PushEventCloneTpl)
	if err != nil {
		return
	}
	builder.execTpl, err = template.New("exec").Parse(opts.Build.ExecuteTpl)
	if err != nil {
		return
	}
	builder.cleanTpl, err = template.New("clean").Parse(opts.Build.CleanTpl)
	return
}

// The entry for each build goroutine.
// Param id is the go routine id, start from 0.
func (b *Builder) builderMain(id int) {
	path := fmt.Sprintf("%s%c%d", b.opt.Dir, os.PathSeparator, id)
	var bid int64
	for {
		bid = <-b.jobChan
		if bid == ExitFlag {
			return
		} else {
			b.build(bid, path)
		}
	}
	b.exitGroup.Done()
}

// Execute ci scripts for Build with id = bid, path as directory
func (b *Builder) build(bid int64, path string) {
	ev, err := b.db.GetPushEventByBuildId(bid)
	CheckNoErr(err)
	err = b.github.CreateStatus(ev.Head, GITHUB_PENDING)
	CheckNoErr(err)
	err = b.db.UpdateBuildStatus(bid, BUILD_RUNNING)
	CheckNoErr(err)

	// After running, all panic should be recovered.
	defer func() {
		if r := recover(); r != nil {
			// CI System Error, set github status & database to error
			err := b.github.CreateStatus(ev.Head, GITHUB_ERROR)
			CheckNoErr(err)
			err = b.db.UpdateBuildStatus(bid, BUILD_ERROR)
			CheckNoErr(err)
		}
	}()
	cmd, err := b.generatePushEventBuildCommand(ev)
	CheckNoErr(err)
	channels, err := b.ExecCommand(path, cmd)
	CheckNoErr(err)
	execCommand := func() bool {
		exit := false
		ok := false
		for !exit {
			select {
			case stdout := <-channels.Stdout:
				b.db.AppendBuildOutput(bid, stdout, syscall.Stdout)
				log.Println(stdout)
			case stderr := <-channels.Stderr:
				b.db.AppendBuildOutput(bid, stderr, syscall.Stderr)
				log.Println(stderr)
			case err = <-channels.Errors:
				if err != nil {
					b.db.AppendBuildOutput(bid, err.Error(), syscall.Stderr)
				} else {
					ok = true
					b.db.AppendBuildOutput(bid, "Exit 0", syscall.Stderr)
				}
				exit = true
				break
			}
		}
		return ok
	}
	b.db.AppendBuildOutput(bid, "Exec Build Commands", syscall.Stderr)
	ok := execCommand()

	if ok {
		err = b.db.UpdateBuildStatus(bid, BUILD_SUCCESS)
		CheckNoErr(err)
		err = b.github.CreateStatus(ev.Head, GITHUB_SUCCESS)
		CheckNoErr(err)
	} else {
		err = b.db.UpdateBuildStatus(bid, BUILD_FAILED)
		CheckNoErr(err)
		err = b.github.CreateStatus(ev.Head, GITHUB_FAILURE)
		CheckNoErr(err)
	}
	cmd, err = b.generateCleanCommand()
	CheckNoErr(err)
	channels, err = b.ExecCommand(path, cmd)
	CheckNoErr(err)
	b.db.AppendBuildOutput(bid, "Exec Clean Commands", syscall.Stderr)
	execCommand()
}

// Start all go routines
func (b *Builder) Start() {
	for i := 0; i < b.opt.Concurrent; i++ {
		b.exitGroup.Add(1)
		go b.builderMain(i)
	}
}

// Exit all go routines
func (b *Builder) Close() {
	for i := 0; i < b.opt.Concurrent; i++ {
		b.jobChan <- ExitFlag
	}
	b.exitGroup.Wait()
}

// Internal Object for generating build scripts.
type CommandBuilder struct {
	buffer bytes.Buffer
}

// Generate whole shell script for building a PushEvent.
// Returns a shell scripts, and error
func (b *Builder) generatePushEventBuildCommand(ev *PushEvent) (cmd []byte, err error) {
	cb, err := b.newCommandBuilder()
	if err != nil {
		return
	}
	err = b.pushEventCloneTpl.Execute(&cb.buffer, ev)
	if err != nil {
		return
	}
	cmd, err = b.toCommand(cb)
	return
}

// Generate clean script.
func (b *Builder) generateCleanCommand() (cmd []byte, err error) {
	buf := bytes.NewBuffer([]byte{})
	err = b.cleanTpl.Execute(buf, b.opt)
	cmd = buf.Bytes()
	return
}

// Add execute command to CommandBuilder then generate command.
func (b *Builder) toCommand(cb *CommandBuilder) (cmd []byte, err error) {
	err = b.execTpl.Execute(&cb.buffer, b.opt)
	cmd = cb.buffer.Bytes()
	return
}

// New Command Builder, also add bootstrap command to builder.
func (b *Builder) newCommandBuilder() (cb *CommandBuilder, err error) {
	cb = &CommandBuilder{}
	err = b.bootstrapTpl.Execute(&cb.buffer, b.opt)
	return
}

// Execute Command Channels.
type CommandExecChannels struct {
	Stdout chan string  // channel for stdout
	Stderr chan string  // channel for stderr
	Errors chan error   // channel for exit status, nil if exit 0
	Cmd    *exec.Cmd    // original command.
}

func reader2chan(r io.Reader, ch chan string, errs chan error) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		ch <- scanner.Text()
	}

	if err := scanner.Err(); err != nil {
		errs <- err
	}
}

func (b *Builder) ExecCommand(basepath string, cmd []byte) (channels *CommandExecChannels, err error) {
	path := fmt.Sprintf("%s%crun", basepath, os.PathSeparator)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0700)
	if err != nil {
		return
	}
	defer f.Close()
	_, err = f.Write(cmd)
	if err != nil {
		return
	}
	c := exec.Command(path)
	channels = &CommandExecChannels{
		Stdout: make(chan string),
		Stderr: make(chan string),
		Errors: make(chan error),
		Cmd:    c,
	}
	stdout, err := c.StdoutPipe()
	if err != nil {
		return
	}
	stderr, err := c.StderrPipe()
	if err != nil {
		return
	}

	err = c.Start()
	if err != nil {
		return
	}

	go reader2chan(stdout, channels.Stdout, channels.Errors)
	go reader2chan(stderr, channels.Stderr, channels.Errors)
	go func() {
		e := c.Wait()
		channels.Errors <- e
	}()
	return
}
