package main

import (
	"log"
	"testing"

	sqlmock "gopkg.in/DATA-DOG/go-sqlmock.v1"

	"github.com/stretchr/testify/assert"
	"github.com/wangkuiyi/ci/webhook"
)

var opts *Options

func TestAddPushEvent(t *testing.T) {
	ev := webhook.PushEvent{}
	ev.HeadCommit.ID = "929924c96f51e2614efcba584fe9cf3a8cb4ec04"
	ev.Ref = "refs/heads/refine_ci_server"
	ev.Repository.CloneURL = "https://github.com/reyoung/ci.git"

	opts.Build.Env["os"] = "osx"

	db, mock, err := sqlmock.New()
	if err != nil {
		panic(err)
	}
	mock.ExpectPrepare("select new_push_event")
	rows := sqlmock.NewRows([]string{"buildId"}).AddRow(0)
	mock.ExpectQuery("select new_push_event").WillReturnRows(rows)
	rows = sqlmock.NewRows([]string{"pe.head", "pe.ref", "pe.clone_url"}).AddRow(ev.Repository.CloneURL, ev.Ref, ev.HeadCommit.ID)
	mock.ExpectQuery("SELECT pe.head, pe.ref, pe.clone_url FROM PushEvents ").WillReturnRows(rows)

	DB := &CIDB{db}
	checkNoErr(err)
	defer DB.Close()
	bid, err := DB.AddPushEvent(ev)
	assert.NoError(t, err)
	getEv, err := DB.GetPushEventByBuildID(bid)
	assert.NoError(t, err)
	assert.Equal(t, ev, getEv)
	DB.AppendBuildOutput(bid, "", true)
	DB.AppendBuildOutput(bid, "", false)
	DB.AppendBuildOutput(bid, "", true)
	DB.AppendBuildOutput(bid, "", false)
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
