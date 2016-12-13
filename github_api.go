//handle the github API
package main

import (
	"fmt"

	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
)

// GithubAPI is used for ci system to access github api.
type GithubAPI struct {
	opts     *GithubAPIOption
	httpOpts *HTTPOption
	cli      *github.Client
}

func newGithubAPI(opts *Options) *GithubAPI {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: opts.Github.Token})
	tc := oauth2.NewClient(oauth2.NoContext, ts)
	gh := github.NewClient(tc)
	return &GithubAPI{
		opts:     &opts.Github,
		cli:      gh,
		httpOpts: &opts.HTTP,
	}
}

// github build status
const (
	GithubPending = "pending"
	GithubSuccess = "success"
	// GithubError means job did complete, but exited with non-zero status
	GithubError = "error"
	// GithubFailure means job failed to complete
	GithubFailure = "failure"
)

// CheckRepo check if github repository is valid
func (gh *GithubAPI) CheckRepo() error {
	_, _, err := gh.cli.Repositories.Get(gh.opts.Owner, gh.opts.Name)
	return err
}

// CreateStatus will a check status for version `sha`.
func (gh *GithubAPI) CreateStatus(sha string, status string) error {
	url := fmt.Sprintf("%s/status/%s", gh.httpOpts.Hostname, sha)
	_, _, err := gh.cli.Repositories.CreateStatus(gh.opts.Owner, gh.opts.Name, sha, &github.RepoStatus{
		TargetURL:   &url,
		State:       &status,
		Description: &gh.opts.Description,
	})
	return err
}

// ListRemoteBranches List all remote branches
func (gh *GithubAPI) ListRemoteBranches() ([]string, error) {
	branches, _, err := gh.cli.Repositories.ListBranches(gh.opts.Owner, gh.opts.Name, nil)
	if err != nil {
		return nil, err
	}
	retv := make([]string, len(branches))
	for i, b := range branches {
		retv[i] = *b.Name
	}
	return retv, nil
}
