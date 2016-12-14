// for Github Webhook, and CI website.
package main

import (
	"io/ioutil"
	"net/http"

	"fmt"
	"html/template"

	"strings"

	"log"

	"path"

	"strconv"

	"encoding/json"

	"github.com/gorilla/mux"
	"github.com/urfave/negroni"
	"github.com/wangkuiyi/ci/db"
	"github.com/wangkuiyi/ci/github"
	"github.com/wangkuiyi/ci/webhook"
)

// HTTPServer for webhook, website.
type HTTPServer struct {
	addr string // http addr.

	router *mux.Router      // Router
	n      *negroni.Negroni // A http middleware framework

	db *db.DB // Database

	renderer *Renderer
	github   *github.API
}

// Renderer is a http middleware for render template
type Renderer struct {
	tmpls       map[string]*template.Template
	owner       string
	name        string
	description string
}

func newRenderer(dir, owner, name, description string) *Renderer {
	tmpls := make(map[string]*template.Template)
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		panic(err)
	}
	for _, f := range files {
		if !f.IsDir() {
			name := f.Name()
			if strings.HasSuffix(name, ".gohtml") && !strings.HasSuffix(name, "base.gohtml") {
				tmpl := template.Must(template.ParseFiles(path.Join(dir, "base.gohtml"), path.Join(dir, name)))
				tmpls[name] = tmpl
			}
		}
	}

	return &Renderer{
		tmpls:       tmpls,
		owner:       owner,
		name:        name,
		description: description,
	}
}

func (renderer *Renderer) render(rw http.ResponseWriter, name string, data map[string]interface{}) {
	insertIfNotExist := func(key string, val string) {
		_, ok := data[key]
		if !ok {
			data[key] = val
		}
	}

	insertIfNotExist("Owner", renderer.owner)
	insertIfNotExist("RepoName", renderer.name)
	insertIfNotExist("Description", renderer.description)
	tpl, ok := renderer.tmpls[fmt.Sprintf("%s.gohtml", name)]
	if ok {
		err := tpl.Execute(rw, data)
		if err != nil {
			log.Panic(err)
		}
	} else {
		log.Panic(fmt.Sprintln("cannot find template ", name))
	}
}

func newHTTPServer(db *db.DB, github *github.API, eventQueue chan<- interface{}, addr, dir, owner, name, description string) *HTTPServer {
	serv := &HTTPServer{
		addr:     addr,
		router:   mux.NewRouter(),
		n:        negroni.New(),
		db:       db,
		renderer: newRenderer(dir, owner, name, description),
		github:   github,
	}
	hook := &webhook.Receiver{Ch: eventQueue}
	serv.n.Use(negroni.NewRecovery())
	serv.n.Use(negroni.NewLogger())
	serv.router.HandleFunc("/ci/", hook.ServeHTTP)
	serv.router.HandleFunc("/", serv.homeHandler).Methods("Get").Name("home")
	serv.router.HandleFunc("/status/{sha:[0-9a-f]+}", serv.statusHandler).Methods("Get").Name("status")
	serv.router.HandleFunc("/builds/{buildID:[0-9]+}", serv.buildsHandler).Methods("Get").Name("builds")
	serv.router.HandleFunc("/build_output/", serv.buildOutputHandler).Methods("Get").Name("buildOutput")
	serv.n.UseHandler(serv.router)
	return serv
}

// ListenAndServe by using configuration.
func (h *HTTPServer) ListenAndServe() error {
	return http.ListenAndServe(h.addr, h.n)
}

func (h *HTTPServer) render(res http.ResponseWriter, req *http.Request, name string, data map[string]interface{}) {
	h.renderer.render(res, name, data)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// VersionWithStatus git commit with build status
type VersionWithStatus struct {
	Sha    string
	Status db.BuildStatus
}

func (h *HTTPServer) homeHandler(res http.ResponseWriter, req *http.Request) {
	type BranchBuilds struct {
		Name     string
		Versions []VersionWithStatus
	}
	// view objects
	var vo struct {
		Branches []BranchBuilds
	}

	refs, err := h.db.Refs(db.Push)
	if err != nil {
		// TODO(helin): better HTTP handler error handling than panic and recover
		log.Panic(err)
	}

	vo.Branches = make([]BranchBuilds, len(refs))
	for i, r := range refs {
		vo.Branches[i].Name = r
		builds, err := h.db.RefBuilds(db.Push, r, 0, 20)
		if err != nil {
			log.Panic(err)
		}
		vo.Branches[i].Versions = make([]VersionWithStatus, len(builds))
		for idx, b := range builds {
			stat, err := b.Status()
			if err != nil {
				log.Println(b, err)
				continue
			}
			vo.Branches[i].Versions[idx] = VersionWithStatus{Sha: b.CommitSHA, Status: stat}
		}
	}

	dat := make(map[string]interface{})
	dat["Vo"] = vo

	h.render(res, req, "index", dat)
}

func (h *HTTPServer) statusHandler(res http.ResponseWriter, req *http.Request) {
	sha := path.Base(req.RequestURI)
	bs, err := h.db.SHABuilds(sha)
	if err != nil {
		log.Panic(err)
	}

	var ids []string
	for _, b := range bs {
		ids = append(ids, strconv.FormatUint(b.ID, 10))
	}

	h.render(res, req, "status", map[string]interface{}{
		"Head": sha,
		"Ids":  ids,
	})
}

func (h *HTTPServer) buildsHandler(res http.ResponseWriter, req *http.Request) {
	bidStr := path.Base(req.RequestURI)
	bid, err := strconv.ParseUint(bidStr, 10, 64)
	if err != nil {
		log.Panic(err)
	}

	b, err := h.db.Build(bid)
	if err != nil {
		log.Panic(err)
	}

	h.render(res, req, "builds", map[string]interface{}{
		"Head": b.CommitSHA,
		"Ref":  b.Ref,
		"Id":   b.ID,
	})
}

func (h *HTTPServer) buildOutputHandler(res http.ResponseWriter, req *http.Request) {
	id, err := strconv.ParseUint(req.URL.Query().Get("id"), 10, 64)
	if err != nil {
		log.Panic(err)
	}

	start, err := strconv.Atoi(req.URL.Query().Get("start"))
	if err != nil {
		log.Panic(err)
	}

	end, err := strconv.Atoi(req.URL.Query().Get("end"))
	if err != nil {
		log.Panic(err)
	}

	b, err := h.db.Build(id)
	if err != nil {
		log.Panic(err)
	}

	output, err := b.Output(start, end)
	if err != nil {
		log.Panic(err)
	}

	stat, err := b.Status()
	if err != nil {
		log.Panic(err)
	}

	type line struct {
		Content string `json:"Content"`
		Channel int    `json:"Channel"`
	}

	var lines []line
	for _, l := range output {
		lines = append(lines, line{Content: l.Str, Channel: int(l.T)})
	}

	dat, err := json.Marshal(struct {
		Status  string
		Outputs []line
	}{
		Status:  string(stat),
		Outputs: lines,
	})
	if err != nil {
		log.Panic(err)
	}

	res.Header().Set("Content-Type", "application/json")
	_, err = res.Write(dat)
	if err != nil {
		log.Panic(err)
	}
}
