package main

import (
	"flag"
	"io/ioutil"
	"log"

	yaml "gopkg.in/yaml.v2"

	"fmt"

	"github.com/wangkuiyi/ci/db"
	"github.com/wangkuiyi/ci/github"
	"github.com/wangkuiyi/ci/webhook"
)

const (
	buildDir = "./build"
)

// settings that user need to define
type setting struct {
	// how many build scripts can be performed in parallel.
	// The builds are executed in different directory
	Concurrency int
	// The build environment can be anything. Such as OS=osx OS_VERSION=10.11
	Env map[string]string
	// repo settings
	Github struct {
		Description string // description for CI shown on github integration comment
		Secret      string // github webhook secret
		Token       string // github personal token.
		Owner       string // repository owner
		Name        string // repository name
		Filename    string // ci script filename
		Endpoint    string // ci server endpoint name (host:ip), build status on github will reference this endpoint
	}
}

func main() {
	path := flag.String("db", "/data/ci.db", "path to db")
	cfg := flag.String("config", "/data/ci.yaml", "configuration file")
	port := flag.Int("port", 8000, "ci server port")
	template := flag.String("template", "/templates", "ci server template directory")
	flag.Parse()

	setting := &setting{}
	content, err := ioutil.ReadFile(*cfg)
	if err != nil {
		panic(err)
	}
	err = yaml.Unmarshal(content, &setting)
	if err != nil {
		panic(err)
	}
	g := github.New(
		setting.Github.Endpoint,
		setting.Github.Description,
		setting.Github.Owner,
		setting.Github.Name,
		setting.Github.Token,
	)
	err = g.CheckRepo()
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

	builder, err := newBuilder(buildChan, g, setting.Concurrency, buildDir, setting.Github.Filename, setting.Env)
	builder.Start()

	eventQueue := make(chan interface{})
	serv := newHTTPServer(d, g, eventQueue, fmt.Sprintf(":%d", *port), *template, setting.Github.Owner, setting.Github.Name, setting.Github.Description)
	go func() {
		log.Println(serv.ListenAndServe())
	}()

	for ev := range eventQueue {
		switch e := ev.(type) {
		case webhook.PushEvent:
			b, err := d.CreateBuild(db.Push, e.Repository.CloneURL, e.Ref, e.HeadCommit.ID)
			if err != nil {
				log.Println(err, e)
				err = g.CreateStatus(e.HeadCommit.ID, github.Failure)
				if err != nil {
					log.Println(err)
				}
				continue
			}
			b.SetStatus(db.BuildQueued)
			go func(b db.Build) {
				buildChan <- b
			}(b)
		case webhook.PullRequestEvent:
			if e.Action != "opened" && e.Action != "synchronize" {
				continue
			}
			b, err := d.CreateBuild(db.PullRequest, e.PullRequest.Head.Repo.CloneURL, e.PullRequest.Head.Ref, e.PullRequest.Head.Sha)
			if err != nil {
				log.Println(err, e)
				err = g.CreateStatus(e.PullRequest.Head.Sha, github.Failure)
				if err != nil {
					log.Println(err)
				}
				continue
			}

			b.SetStatus(db.BuildQueued)
			go func(b db.Build) {
				buildChan <- b
			}(b)
		}
	}
}
