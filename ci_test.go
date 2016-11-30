package main

import (
	"log"
	"syscall"
	"testing"

	sqlmock "gopkg.in/DATA-DOG/go-sqlmock.v1"

	"github.com/stretchr/testify/assert"
)

var opts *Options

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

	opts.Build.Env["os"] = "osx"

	db, mock, err := sqlmock.New()
	if err != nil {
		panic(err)
	}
	mock.ExpectPrepare("select new_push_event")
	rows := sqlmock.NewRows([]string{"buildId"}).AddRow(0)
	mock.ExpectQuery("select new_push_event").WillReturnRows(rows)
	rows = sqlmock.NewRows([]string{"pe.head", "pe.ref", "pe.clone_url"}).AddRow(ev.CloneURL, ev.Ref, ev.Head)
	mock.ExpectQuery("SELECT pe.head, pe.ref, pe.clone_url FROM PushEvents ").WillReturnRows(rows)

	DB := &CIDB{db}
	checkNoErr(err)
	defer DB.Close()
	jobChan := make(chan int64)
	builder, err := newBuilder(jobChan, opts, DB, nil)
	checkNoErr(err)

	bid, err := DB.AddPushEvent(ev)
	assert.NoError(t, err)
	getEv, err := DB.GetPushEventByBuildID(bid)
	assert.NoError(t, err)
	assert.Equal(t, ev, getEv)
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

	assert.NoError(t, DB.removePushEvent(ev, bid))
}

func TestMain(m *testing.M) {
	opts = ParseArgs()
}
