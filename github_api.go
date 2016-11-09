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
	_, _, err := gh.Repositories.Get(opts.Github.Owner, opts.Github.Name)
	checkNoErr(err)
	return &GithubAPI{
		opts:     &opts.Github,
		cli:      gh,
		httpOpts: &opts.HTTP,
	}
}

const (
	// GithubPending github check status pending
	GithubPending = "pending"
	// GithubSuccess github check status success
	GithubSuccess = "success"
	// GithubError github check status error
	GithubError = "error"
	// GithubFailure github check status failure
	GithubFailure = "failure"
)

// CreateStatus will a check status for version `sha`.
func (gh *GithubAPI) CreateStatus(sha string, status string) error {
	url := fmt.Sprintf("%s%s%s", gh.httpOpts.Hostname, gh.httpOpts.StatusURI, sha)
	_, _, err := gh.cli.Repositories.CreateStatus(gh.opts.Owner, gh.opts.Name, sha, &github.RepoStatus{
		TargetURL:   &url,
		State:       &status,
		Description: &gh.opts.Description,
	})
	return err
}

// ListRemoteBranches List all remote branches
func (gh *GithubAPI) ListRemoteBranches() ([]*string, error) {
	branches, _, err := gh.cli.Repositories.ListBranches(gh.opts.Owner, gh.opts.Name, nil)
	if err != nil {
		return nil, err
	}
	retv := make([]*string, len(branches))
	for i, b := range branches {
		retv[i] = b.Name
	}
	return retv, nil
}
