package main

import (
	"log"

	"github.com/wangkuiyi/ci/webhook"
)

func main() {
	opts := ParseArgs()
	github := newGithubAPI(opts)
	err := github.CheckRepo()
	checkNoErr(err)

	db, err := openCIDB(opts.Database.User, opts.Database.Password, opts.Database.DatabaseName)
	checkNoErr(err)

	buildChan := make(chan int64, 256)
	go func() { checkNoErr(db.RecoverFromPreviousDown(buildChan)) }()

	builder, err := newBuilder(buildChan, opts, db, github)
	builder.Start()
	defer builder.Close()

	eventQueue := make(chan interface{})
	serv := newHTTPServer(opts, db, github, eventQueue)
	go func() {
		checkNoErr(serv.ListenAndServe())
	}()

	for ev := range eventQueue {
		switch e := ev.(type) {
		case webhook.PushEvent:
			bid, err := db.AddPushEvent(e)
			checkNoErr(err)
			buildChan <- bid
		case webhook.PullRequestEvent:
			// TODO(helin)
		}
	}
}

func checkNoErr(err error) {
	if err != nil {
		log.Panic(err.Error())
	}
}
