//  handle the github API
package main

import (
	"fmt"
	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
)

type GithubAPI struct {
	opts     *GithubAPIOption
	httpOpts *HttpOption
	cli      *github.Client
}

func newGithubAPI(opts *Options) *GithubAPI {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: opts.Github.Token})
	tc := oauth2.NewClient(oauth2.NoContext, ts)
	gh := github.NewClient(tc)
	_, _, err := gh.Repositories.Get(opts.Github.Owner, opts.Github.Name)
	CheckNoErr(err)
	return &GithubAPI{
		opts:     &opts.Github,
		cli:      gh,
		httpOpts: &opts.HTTP,
	}
}

const (
	GITHUB_PENDING = "pending"
	GITHUB_SUCCESS = "success"
	GITHUB_ERROR   = "error"
	GITHUB_FAILURE = "failure"
)

// Create a check status for version `sha`.
func (gh *GithubAPI) CreateStatus(sha string, status string) error {
	url := fmt.Sprintf("%s%s%s", gh.httpOpts.Hostname, gh.httpOpts.StatusUri, sha)
	_, _, err := gh.cli.Repositories.CreateStatus(gh.opts.Owner, gh.opts.Name, sha, &github.RepoStatus{
		TargetURL:   &url,
		State:       &status,
		Description: &gh.opts.Description,
	})
	return err
}
