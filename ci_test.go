package main

import (
	"log"
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
)

var DB *CIDB
var jobChan chan int64
var builder *Builder

const BuildCommand = `#!/bin/bash
echo "Setting Environments"
set -x

export os="osx"

set +x
set -e

set -x
git clone --branch=refine_ci_server --depth=50 https://github.com/reyoung/ci.git repo
cd repo
git checkout -qf 929924c96f51e2614efcba584fe9cf3a8cb4ec04

set +x
if [ -f ./ci.sh ]; then
	./ci.sh
else
	echo "./ci.sh not found, it seems the ci script is not configured."
fi
`

func TestAddPushEvent(t *testing.T) {
	ev := PushEvent{
		CloneURL: "https://github.com/reyoung/ci.git",
		Ref:      "refs/heads/refine_ci_server",
		Head:     "929924c96f51e2614efcba584fe9cf3a8cb4ec04",
	}
	bid, err := DB.AddPushEvent(&ev)
	assert.NoError(t, err)
	getEv, err := DB.GetPushEventByBuildID(bid)
	assert.NoError(t, err)
	assert.Equal(t, ev, *getEv)
	cmd, err := builder.generatePushEventBuildCommand(&ev)
	assert.NoError(t, err)
	assert.Equal(t, string(cmd[:]), BuildCommand)
	chans, err := builder.execCommand(".", cmd)
	assert.NoError(t, err)
	execCommand := func() {
		exit := false
		for !exit {
			select {
			case stdout := <-chans.Stdout:
				DB.AppendBuildOutput(bid, stdout, syscall.Stdout)
			case stderr := <-chans.Stderr:
				DB.AppendBuildOutput(bid, stderr, syscall.Stderr)
			case err = <-chans.Errors:
				assert.NoError(t, err)
				exit = true
				break
			}
		}
	}
	execCommand()
	cmd, err = builder.generateCleanCommand()
	assert.NoError(t, err)
	chans, err = builder.execCommand(".", cmd)
	assert.NoError(t, err)
	execCommand()

	outputs, err := DB.GetBuildOutput(bid)
	assert.NoError(t, err)

	for _, output := range outputs {
		log.Println(output.Content)
	}

	assert.NoError(t, DB.removePushEvent(&ev, bid))
}

func TestMain(m *testing.M) {
	opts := ParseArgs()
	opts.Build.Env["os"] = "osx"
	var err error
	DB, err = openCIDB(opts.Database.User, opts.Database.Password, opts.Database.DatabaseName)
	checkNoErr(err)
	defer DB.Close()
	jobChan = make(chan int64)
	builder, err = newBuilder(jobChan, opts, DB, nil)
	checkNoErr(err)
	os.Exit(m.Run())
}
