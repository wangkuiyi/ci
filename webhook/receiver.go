package webhook

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
)

// PushEvent is a webhook push event
type PushEvent struct {
	Ref        string `json:"ref"`
	HeadCommit struct {
		ID string `json:"id"`
	} `json:"head_commit"`
	Repository struct {
		CloneURL string `json:"clone_url"`
	} `json:"repository"`
}

// PullRequestEvent is a webhook pull request event
type PullRequestEvent struct {
	Action     string `json:"action"`
	PullRequst struct {
		ID int `json:"id"`
	} `json:"pull_request"`
	Repo struct {
		CloneURL string `json:"clone_url"`
	} `json:"repo"`
	Head struct {
		Sha string `json:"sha"`
	} `json:"head"`
}

// Receiver receives webhook events
type Receiver struct {
	Ch chan<- interface{}
}

func (r *Receiver) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	defer req.Body.Close()

	if req.Method != "POST" {
		http.Error(w, "405 Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	eventType := req.Header.Get("X-GitHub-Event")
	if eventType == "" {
		http.Error(w, "400 Bad Request - Missing X-GitHub-Event Header", http.StatusBadRequest)
		return
	}

	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	switch eventType {
	case "push":
		e := PushEvent{}
		err = json.Unmarshal(body, &e)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		r.Ch <- e
	case "pull_request":
		e := PullRequestEvent{}
		err = json.Unmarshal(body, &e)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		r.Ch <- e
	}
}
