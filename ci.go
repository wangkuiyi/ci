package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/topicai/candy"
)

// PushEvent is the JSON schema when Github calls the Webhook to notify about a push operation.
type PushEvent struct {
	After      string `json:"after,omitempty"`
	Repository struct {
		URL string `json:"url,omitempty"`
	}
}

func main() {
	addr := flag.String("addr", ":8080", "Listing address")
	dbuser := flag.String("user", "root", "MySQL username")
	dbpasswd := flag.String("passwd", "", "MySQL password")
	database := flag.String("database", "", "MySQL database")
	flag.Parse()

	db, e := sql.Open("mysql", fmt.Sprintf("%s:%s@/%s", *dbuser, *dbpasswd, *database))
	candy.Must(e)
	defer func() { candy.Must(db.Close()) }()

	query, e := db.Prepare("SELECT status, detail FROM ci WHERE id = ?")
	candy.Must(e)
	defer func() { candy.Must(query.Close()) }()

	http.HandleFunc("/ci", // NOTE: /ci URL for Github Webhook.
		makeSafeHandler(func(w http.ResponseWriter, r *http.Request) {
			event := r.Header["X-Github-Event"]
			if r.Method == "POST" && len(event) > 0 && event[0] == "push" {
				// For PushEvent: https://developer.github.com/v3/activity/events/types/#pushevent
				var push PushEvent
				candy.Must(json.NewDecoder(r.Body).Decode(&push))
				ci(db, &push)
			}
		}))
	http.HandleFunc("/status", // NOTE: /status/{SHA} for retrieving status/details
		makeSafeHandler(func(w http.ResponseWriter, r *http.Request) {
			id := path.Base(r.URL.Path)
			var status, detail string
			candy.Must(query.QueryRow(id).Scan(&status, &detail))
			fmt.Fprintf(w, "%s : %s\n\n%s", id, status, detail)
		}))
	candy.Must(http.ListenAndServe(*addr, nil))
}

func makeSafeHandler(fn http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if e := recover(); e != nil {
				http.Error(w, fmt.Sprint(e), http.StatusInternalServerError)
			}
		}()
		fn(w, r)
	}
}

func ci(db *sql.DB, push *PushEvent) {
	stmtIn, e := db.Prepare("INSERT ci SET id=?, status=?, detail=?")
	candy.Must(e)
	defer func() {
		candy.Must(stmtIn.Close())
	}()

	detail := ""
	defer func() {
		if e := recover(); e != nil {
			_, _ = stmtIn.Exec(push.After, "failed", fmt.Sprintf("%s\n%v", e, detail))
		}
	}()

	ws, e := ioutil.TempDir("", "")
	candy.Must(e)

	defer func() {
		candy.Must(os.RemoveAll(ws))
	}()

	repoURL, e := url.Parse(push.Repository.URL)
	candy.Must(e)

	pkg := path.Join(repoURL.Host, repoURL.Path)
	detail += cmd(nil, "go", "get", "-u", pkg) // NOTE：must be open source Go repo.

	repo := path.Base(repoURL.Path)
	candy.Must(os.Chdir(path.Join(ws, repo)))

	detail += cmd(nil, "git", "checkout", push.After)
	detail += cmd(nil, "bash", "-c", "./ci.bash") // NOTE: entrypoinst must be named .ci.bash.

	_, _ = stmtIn.Exec(push.After, "success", fmt.Sprintf("%s\n%v", e, detail))
}

func cmd(env map[string]string, name string, arg ...string) string {
	cmd := exec.Command(name, arg...)

	// Inherit environ from the parent process. Note that, instead
	// of appending env to cmd.Env, we rewrite the value of an
	// environment varaible in cmd.Env if it is in env.  This
	// prevents from cases like two GOPATH variables in cmd.Env.
	for _, en := range os.Environ() {
		kv := strings.Split(en, "=")
		if v := env[kv[0]]; v != "" {
			en = fmt.Sprintf("%s=%s", kv[0], v)
			delete(env, kv[0])
		}
		cmd.Env = append(cmd.Env, en)
	}

	for k, v := range env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	b, e := cmd.CombinedOutput()
	if e != nil {
		log.Panicf("%s %s\nFailed: %v\n%s", name, strings.Join(arg, " "), e, string(b))
	}
	return string(b)
}
