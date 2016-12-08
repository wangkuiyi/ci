package main

import (
	"flag"
	"io/ioutil"
	"log"

	yaml "gopkg.in/yaml.v2"

	"github.com/wangkuiyi/ci/db"
	"github.com/wangkuiyi/ci/webhook"
)

func main() {
	path := flag.String("db", "", "path to db")
	fn := flag.String("config", "", "Configuration File")
	flag.Parse()

	if *fn == "" || *path == "" {
		flag.Usage()
		return
	}

	opts := newOptions()
	content, err := ioutil.ReadFile(*fn)
	if err != nil {
		panic(err)
	}
	err = yaml.Unmarshal(content, opts)
	if err != nil {
		panic(err)
	}
	github := newGithubAPI(opts)
	err = github.CheckRepo()
	if err != nil {
		panic(err)
	}

	d, err := db.Open(*path)
	if err != nil {
		panic(err)
	}

	buildChan := make(chan db.Build, 256)
	pending, err := d.PendingBuilds()
	if err != nil {
		panic(err)
	}

	go func() {
		for _, b := range pending {
			buildChan <- b
		}
	}()

	builder, err := newBuilder(buildChan, opts, github)
	builder.Start()

	eventQueue := make(chan interface{})
	serv := newHTTPServer(opts, d, github, eventQueue)
	go func() {
		log.Println(serv.ListenAndServe())
	}()

	for ev := range eventQueue {
		switch e := ev.(type) {
		case webhook.PushEvent:
			b, err := d.CreateBuild(db.Push, e.Repository.CloneURL, e.Ref, e.HeadCommit.ID)
			if err != nil {
				b.AppendOutput(db.OutputLine{T: db.Error, Str: err.Error()})
				log.Println(err)
				continue
			}
			b.SetStatus(db.BuildQueued)
			go func(b db.Build) {
				buildChan <- b
			}(b)
		case webhook.PullRequestEvent:
			// TODO(helin)
		}
	}
}
