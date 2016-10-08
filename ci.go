package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	"github.com/topicai/candy"
)

// PushEvent represents the JSON payload from Github push
// Webhook. https://developer.github.com/v3/activity/events/types/#pushevent
type PushEvent struct {
	After string `json:"after,omitempty"`
	Repository
}

// Repository is the JSON schema used in Github PushEvent.
type Repository struct {
	URL string `json:"url,omitempty"`
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

	retrieve := makeRetriever(db)
	insert := makeInserter(db)

	http.HandleFunc("/ci/", // NOTE: /ci URL for Github Webhook.
		makeSafeHandler(func(w http.ResponseWriter, r *http.Request) {
			event := r.Header["X-Github-Event"]
			if r.Method == "POST" && len(event) > 0 && event[0] == "push" {
				var push PushEvent
				candy.Must(json.NewDecoder(r.Body).Decode(&push))
				ci(&push, insert)
			}
		}))
	http.HandleFunc("/status/", // NOTE: /status/{SHA} for retrieving status/details
		makeSafeHandler(func(w http.ResponseWriter, r *http.Request) {
			id := path.Base(r.URL.Path)
			status, detail := retrieve(id)
			fmt.Fprintf(w, "%s : %s\n\n%s", id, status, detail)
		}))
	candy.Must(http.ListenAndServe(*addr, nil))
}

func makeRetriever(db *sql.DB) func(id string) (status, detail string) {
	query, e := db.Prepare("SELECT status, detail FROM ci WHERE id = ?")
	candy.Must(e)
	return func(id string) (status, detail string) {
		candy.Must(query.QueryRow(id).Scan(&status, &detail))
		return
	}
}

func makeInserter(db *sql.DB) func(id, status, detail string) {
	stmtIn, e := db.Prepare(`INSERT INTO ci (id, status, detail) VALUES(?, ?, ?) ON DUPLICATE KEY UPDATE status=?, detail=?`)
	candy.Must(e)
	return func(id, status, detail string) {
		_, e := stmtIn.Exec(id, status, detail, status, detail)
		candy.Must(e)
	}
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

func ci(push *PushEvent, insert func(id, status, detail string)) {
	detail := ""
	defer func() {
		if e := recover(); e != nil {
			insert(push.After, "failed", fmt.Sprintf("%s\n%v", e, detail))
		}
	}()

	ws, e := ioutil.TempDir("", "")
	candy.Must(e)
	defer func() {
		candy.Must(os.RemoveAll(ws))
	}()

	repoURL, e := url.Parse(push.Repository.URL)
	candy.Must(e)
	repo := path.Base(repoURL.Path)

	candy.Must(os.Chdir(ws))
	cmd(nil, "git", "clone", push.Repository.URL)
	candy.Must(os.Chdir(path.Join(ws, repo)))

	cmd(nil, "git", "checkout", "-b", "ci", push.After)
	detail += cmd(nil, "bash", "-c", "./.ci.bash") // NOTE: entrypoinst must be named .ci.bash.

	insert(push.After, "success", detail)
}

func cmd(env map[string]string, name string, arg ...string) string {
	cmd := exec.Command(name, arg...)

	// Rewrite the value of existing keys.
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
		panic(fmt.Sprintf("%s %s\nFailed: %v\n%s", name, strings.Join(arg, " "), e, string(b)))
	}
	return string(b)
}
