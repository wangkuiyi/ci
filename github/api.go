package github

import (
	"fmt"

	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
)

// API is used for ci system to access github api.
type API struct {
	cli         *github.Client
	endpoint    string
	owner       string
	name        string
	description string
}

// New creates a new github api
func New(endpoint, description, owner, name, token string) *API {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(oauth2.NoContext, ts)
	g := github.NewClient(tc)
	return &API{
		cli:         g,
		endpoint:    endpoint,
		owner:       owner,
		name:        name,
		description: description,
	}
}

// github build status
const (
	Pending = "pending"
	Success = "success"
	// GithubError means job did complete, but exited with non-zero status
	Error = "error"
	// GithubFailure means job failed to complete
	Failure = "failure"
)

// CheckRepo check if github repository is valid
func (g *API) CheckRepo() error {
	_, _, err := g.cli.Repositories.Get(g.owner, g.name)
	return err
}

// CreateStatus will a check status for version `sha`.
func (g *API) CreateStatus(sha string, status string) error {
	url := fmt.Sprintf("%s/status/%s", g.endpoint, sha)
	_, _, err := g.cli.Repositories.CreateStatus(g.owner, g.name, sha, &github.RepoStatus{
		TargetURL:   &url,
		State:       &status,
		Description: &g.description,
	})
	return err
}

// ListRemoteBranches List all remote branches
func (g *API) ListRemoteBranches() ([]string, error) {
	branches, _, err := g.cli.Repositories.ListBranches(g.owner, g.name, nil)
	if err != nil {
		return nil, err
	}
	retv := make([]string, len(branches))
	for i, b := range branches {
		retv[i] = *b.Name
	}
	return retv, nil
}
