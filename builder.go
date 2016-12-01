// The utilities for running ci scripts.
package main

import (
	"bytes"
	"math/rand"
	"os"
	"os/exec"
	"path"
	"strconv"
	"sync"
	"text/template"

	"github.com/wangkuiyi/ci/webhook"
)

// Builder will start multiple go routine to executing ci scripts for each builds.
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
		path := path.Join(opts.Build.Dir, strconv.Itoa(i))
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
	path := path.Join(b.opt.Dir, strconv.Itoa(id))
	var bid int64
	var ok bool
	for {
		bid, ok = <-b.jobChan
		if !ok {
			break
		}
		b.build(bid, path)
	}
	b.exitGroup.Done()
}

// Execute ci scripts for Build with id = bid, path as directory
func (b *Builder) build(bid int64, path string) {
	ev, err := b.db.GetPushEventByBuildID(bid)
	checkNoErr(err)
	err = b.github.CreateStatus(ev.HeadCommit.ID, GithubPending)
	checkNoErr(err)
	err = b.db.UpdateBuildStatus(bid, BuildRunning)
	checkNoErr(err)

	// After running, all panic should be recovered.
	defer func() {
		if r := recover(); r != nil {
			// CI System Error, set github status & database to error
			err := b.github.CreateStatus(ev.HeadCommit.ID, GithubError)
			checkNoErr(err)
			err = b.db.UpdateBuildStatus(bid, BuildError)
			checkNoErr(err)
		}
	}()

	var buffer bytes.Buffer
	err = b.bootstrapTpl.Execute(&buffer, b.opt)
	checkNoErr(err)
	err = b.pushEventCloneTpl.Execute(&buffer, struct {
		webhook.PushEvent
		BuildPath string
	}{PushEvent: ev, BuildPath: path})
	checkNoErr(err)
	err = b.execTpl.Execute(&buffer, b.opt)
	checkNoErr(err)
	cmd, err := genCmd(path, buffer.Bytes())
	checkNoErr(err)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = b.db.AppendBuildOutput(bid, "Exec Build Commands", false)
	checkNoErr(err)
	buildErr := cmd.Run()
	err = b.db.AppendBuildOutput(bid, string(stdout.Bytes()), true)
	checkNoErr(err)
	err = b.db.AppendBuildOutput(bid, string(stderr.Bytes()), false)
	checkNoErr(err)
	if buildErr != nil {
		err = b.db.AppendBuildOutput(bid, buildErr.Error(), false)
		checkNoErr(err)
		err = b.db.UpdateBuildStatus(bid, BuildFailed)
		checkNoErr(err)
		err = b.github.CreateStatus(ev.HeadCommit.ID, GithubFailure)
		checkNoErr(err)
	} else {
		err = b.db.AppendBuildOutput(bid, "Exit 0", false)
		checkNoErr(err)
		err = b.db.UpdateBuildStatus(bid, BuildSuccess)
		checkNoErr(err)
		err = b.github.CreateStatus(ev.HeadCommit.ID, GithubSuccess)
		checkNoErr(err)
	}

	stdout, stderr = bytes.Buffer{}, bytes.Buffer{}
	var buf bytes.Buffer
	err = b.cleanTpl.Execute(&buf, struct {
		*BuildOption
		BuildPath string
	}{BuildOption: b.opt, BuildPath: path})
	checkNoErr(err)
	cmd, err = genCmd(path, buf.Bytes())
	checkNoErr(err)

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	b.db.AppendBuildOutput(bid, "Exec Clean Commands", false)
	checkNoErr(err)
	buildErr = cmd.Run()
	err = b.db.AppendBuildOutput(bid, string(stdout.Bytes()), true)
	checkNoErr(err)
	err = b.db.AppendBuildOutput(bid, string(stderr.Bytes()), false)
	checkNoErr(err)
	if buildErr != nil {
		err = b.db.AppendBuildOutput(bid, buildErr.Error(), false)
		checkNoErr(err)
	} else {
		err = b.db.AppendBuildOutput(bid, "Exit 0", false)
		checkNoErr(err)
	}
}

// Start all go routines
func (b *Builder) Start() {
	for i := 0; i < b.opt.Concurrent; i++ {
		b.exitGroup.Add(1)
		go b.builderMain(i)
	}
}

// Close will stop all go routines
func (b *Builder) Close() {
	close(b.jobChan)
	b.exitGroup.Wait()
}

func genCmd(basepath string, cmd []byte) (c *exec.Cmd, err error) {
	path := path.Join(basepath, strconv.Itoa(rand.Int()))
	// the build folder will be cleaned later
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0700)
	if err != nil {
		return
	}
	defer f.Close()
	_, err = f.Write(cmd)
	if err != nil {
		return
	}
	c = exec.Command(path)
	return
}
