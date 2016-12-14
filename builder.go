// The utilities for running ci scripts.
package main

import (
	"bufio"
	"bytes"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"path"
	"strconv"
	"text/template"
	"time"

	"github.com/wangkuiyi/ci/db"
	"github.com/wangkuiyi/ci/github"
)

const (
	bootstrapTpl = `#!/bin/bash
echo "Setting Environments"
set -x
{{range $envKey, $envVal := .Env}}
export {{$envKey}}="{{$envVal}}"
{{end}}
set +x
set -e
`
	pushEventCloneTpl = `
set -x
cd {{.BuildPath}}
git clone --depth 1 {{.CloneURL}} repo
cd repo
git fetch origin {{.Ref}}
git checkout -qf {{.Head}}
`
	executeTpl = `
set +x
if [ -f {{.CIPath}} ]; then
	source {{.CIPath}}
else
	echo "{{.CIPath}} not found, it seems the ci script is not configured."
fi
`
	cleanTpl = `#!/bin/bash
rm -rf {{.BuildPath}}/*
`
)

// Builder will start multiple go routine to executing ci scripts for each builds.
// For each build, builder will generate an shell script for execution. Then just execute this shell script.
type Builder struct {
	jobChan     <-chan db.Build // channel for build id
	dir         string
	concurrency int
	ciPath      string
	env         map[string]string

	bootstrapTpl      *template.Template // the build bootstrap template, including setting environment, etc.
	pushEventCloneTpl *template.Template // git clone template for push event.
	execTpl           *template.Template // execute ci scripts template.
	cleanTpl          *template.Template // clean template. clean the building workspace.

	github *github.API // github api
}

// New builder instance.
// It will create the building directory for each go routine. The building dir can be configured in configuration file.
func newBuilder(jobChan <-chan db.Build, github *github.API, concurrency int, dir, ciPath string, env map[string]string) (builder *Builder, err error) {
	for i := 0; i < concurrency; i++ {
		path := path.Join(dir, strconv.Itoa(i))
		err = os.MkdirAll(path, 0755)
		if err != nil {
			return
		}
	}

	builder = &Builder{
		jobChan:     jobChan,
		dir:         dir,
		ciPath:      ciPath,
		env:         env,
		concurrency: concurrency,
		github:      github,
	}

	builder.bootstrapTpl, err = template.New("bootstrap").Parse(bootstrapTpl)
	if err != nil {
		return
	}
	builder.pushEventCloneTpl, err = template.New("pushEvent").Parse(pushEventCloneTpl)
	if err != nil {
		return
	}
	builder.execTpl, err = template.New("exec").Parse(executeTpl)
	if err != nil {
		return
	}
	builder.cleanTpl, err = template.New("clean").Parse(cleanTpl)
	return
}

// The entry for each build goroutine.
// Param id is the go routine id, start from 0.
func (b *Builder) builderMain(id int) {
	path := path.Join(b.dir, strconv.Itoa(id))
	for {
		build, ok := <-b.jobChan
		if !ok {
			break
		}
		err := b.build(build, path)
		if err != nil {
			build.SetStatus(db.BuildError)
			build.AppendOutput(db.OutputLine{T: db.Error, Str: err.Error(), Time: time.Now()})
			b.github.CreateStatus(build.CommitSHA, github.Failure)
			log.Println(err)
			continue
		}
	}
}

func run(b db.Build, cmd *exec.Cmd) error {
	o, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	e, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err = cmd.Start(); err != nil {
		return err
	}
	waitOut := make(chan struct{})
	waitErr := make(chan struct{})
	go func() {
		s := bufio.NewScanner(o)
		for s.Scan() {
			b.AppendOutput(db.OutputLine{T: db.Stdout, Str: s.Text(), Time: time.Now()})
		}
		close(waitOut)
	}()

	go func() {
		s := bufio.NewScanner(e)
		for s.Scan() {
			b.AppendOutput(db.OutputLine{T: db.Stderr, Str: s.Text(), Time: time.Now()})
		}
		close(waitErr)
	}()

	<-waitOut
	<-waitErr
	if err = cmd.Wait(); err != nil {
		return err
	}
	return nil
}

// Execute ci scripts for Build with id = bid, path as directory
func (b *Builder) build(build db.Build, path string) error {
	err := build.SetStatus(db.BuildRunning)
	if err != nil {
		return err
	}
	err = b.github.CreateStatus(build.CommitSHA, github.Pending)
	if err != nil {
		return err
	}

	var buffer bytes.Buffer
	err = b.bootstrapTpl.Execute(&buffer, struct{ Env map[string]string }{Env: b.env})
	if err != nil {
		return err
	}

	err = b.pushEventCloneTpl.Execute(&buffer, struct {
		CloneURL  string
		Ref       string
		Head      string
		BuildPath string
	}{CloneURL: build.CloneURL, Ref: build.Ref, Head: build.CommitSHA, BuildPath: path})
	if err != nil {
		return err
	}

	err = b.execTpl.Execute(&buffer, struct{ CIPath string }{CIPath: b.ciPath})
	if err != nil {
		return err
	}

	cmd, err := genCmd(path, buffer.Bytes())
	if err != nil {
		return err
	}

	var stdout, stderr bytes.Buffer
	err = build.AppendOutput(db.OutputLine{T: db.Info, Str: "Running build commands", Time: time.Now()})
	if err != nil {
		return err
	}
	buildErr := run(build, cmd)
	if buildErr != nil {
		err = build.AppendOutput(db.OutputLine{T: db.Error, Str: buildErr.Error(), Time: time.Now()})
		if err != nil {
			return err
		}
		err = build.SetStatus(db.BuildFailed)
		if err != nil {
			return err
		}
		err = b.github.CreateStatus(build.CommitSHA, github.Error)
		if err != nil {
			return err
		}
	} else {
		err = build.AppendOutput(db.OutputLine{T: db.Info, Str: "Exit 0", Time: time.Now()})
		if err != nil {
			return err
		}
		err = build.SetStatus(db.BuildSuccess)
		if err != nil {
			return err
		}
		err = b.github.CreateStatus(build.CommitSHA, github.Success)
		if err != nil {
			return err
		}
	}

	stdout, stderr = bytes.Buffer{}, bytes.Buffer{}
	var buf bytes.Buffer
	err = b.cleanTpl.Execute(&buf, struct {
		BuildPath string
	}{BuildPath: path})
	if err != nil {
		return err
	}

	cmd, err = genCmd(path, buf.Bytes())
	if err != nil {
		return err
	}

	err = build.AppendOutput(db.OutputLine{T: db.Info, Str: "Running clean commands", Time: time.Now()})
	if err != nil {
		return err
	}
	buildErr = run(build, cmd)
	err = build.AppendOutput(db.OutputLine{T: db.Stdout, Str: string(stdout.Bytes()), Time: time.Now()})
	if err != nil {
		return err
	}
	err = build.AppendOutput(db.OutputLine{T: db.Stderr, Str: string(stderr.Bytes()), Time: time.Now()})
	if err != nil {
		return err
	}

	if buildErr != nil {
		err = build.AppendOutput(db.OutputLine{T: db.Error, Str: buildErr.Error(), Time: time.Now()})
		if err != nil {
			return err
		}
	} else {
		err = build.AppendOutput(db.OutputLine{T: db.Info, Str: "Exit 0", Time: time.Now()})
		if err != nil {
			return err
		}
	}
	return nil
}

// Start all go routines
func (b *Builder) Start() {
	for i := 0; i < b.concurrency; i++ {
		go b.builderMain(i)
	}
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
