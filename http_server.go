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

	"github.com/gorilla/context"
	"github.com/gorilla/mux"
	"github.com/urfave/negroni"
	"github.com/wangkuiyi/ci/webhook"
)

// HTTPServer for webhook, website.
type HTTPServer struct {
	addr string // http addr.

	router *mux.Router      // Router
	n      *negroni.Negroni // A http middleware framework

	db *CIDB // Database

	renderer *Renderer
	github   *GithubAPI
}

// Renderer is a http middleware for render template
type Renderer struct {
	tmpls map[string]*template.Template
	opts  *Options
}

func newRenderer(opts *Options) *Renderer {
	tmpls := make(map[string]*template.Template)
	files, err := ioutil.ReadDir(opts.HTTP.TemplateDir)
	if err != nil {
		panic(err)
	}
	for _, f := range files {
		if !f.IsDir() {
			name := f.Name()
			if strings.HasSuffix(name, ".gohtml") && !strings.HasSuffix(name, "base.gohtml") {
				tmpl := template.Must(template.ParseFiles(path.Join(opts.HTTP.TemplateDir, "base.gohtml"), path.Join(opts.HTTP.TemplateDir, name)))
				tmpls[name] = tmpl
			}
		}
	}

	return &Renderer{
		tmpls: tmpls,
		opts:  opts,
	}
}

func (renderer *Renderer) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	context.Set(r, "renderer", renderer)
	next(rw, r)
}

func (renderer *Renderer) render(rw http.ResponseWriter, name string, data map[string]interface{}) {
	insertIfNotExist := func(key string, val string) {
		_, ok := data[key]
		if !ok {
			data[key] = val
		}
	}

	insertIfNotExist("Owner", renderer.opts.Github.Owner)
	insertIfNotExist("RepoName", renderer.opts.Github.Name)
	insertIfNotExist("Description", renderer.opts.Github.Description)
	log.Println(data)
	tpl, ok := renderer.tmpls[fmt.Sprintf("%s.gohtml", name)]
	if ok {
		checkNoErr(tpl.Execute(rw, data))
	} else {
		log.Panic(fmt.Sprintln("cannot found template ", name))
	}
}

func newHTTPServer(opts *Options, db *CIDB, github *GithubAPI, eventQueue chan<- interface{}) *HTTPServer {
	serv := &HTTPServer{
		addr:     opts.HTTP.Addr,
		router:   mux.NewRouter(),
		n:        negroni.New(),
		db:       db,
		renderer: newRenderer(opts),
		github:   github,
	}
	hook := &webhook.Receiver{Ch: eventQueue}
	serv.n.Use(negroni.NewRecovery())
	serv.n.Use(negroni.NewLogger())
	serv.n.Use(serv.renderer)
	serv.router.HandleFunc(opts.HTTP.CIUri, hook.ServeHTTP)
	serv.router.HandleFunc("/", serv.homeHandler).Methods("Get").Name("home")
	serv.router.HandleFunc("/status/{sha:[0-9a-f]+}", serv.statusHandler).Methods("Get").Name("status")
	serv.router.HandleFunc("/builds/{buildID:[0-9]+}", serv.buildsHandler).Methods("Get").Name("builds")
	serv.router.HandleFunc("/build_output/", serv.buildOutputHandler).Methods("Get").Name("buildOutput")
	serv.n.UseHandler(serv.router)
	return serv
}

// ListenAndServe by using configuration.
func (httpServ *HTTPServer) ListenAndServe() error {
	return http.ListenAndServe(httpServ.addr, httpServ.n)
}

func (httpServ *HTTPServer) render(res http.ResponseWriter, req *http.Request, name string, data map[string]interface{}) {
	context.Get(req, "renderer").(*Renderer).render(res, name, data)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (httpServ *HTTPServer) homeHandler(res http.ResponseWriter, req *http.Request) {
	type BranchBuilds struct {
		Name     string
		Versions []VersionWithStatus
	}
	// view objects
	var vo struct {
		Branches []BranchBuilds
	}

	branches, err := httpServ.github.ListRemoteBranches()
	checkNoErr(err)

	vo.Branches = make([]BranchBuilds, len(branches))
	for i, b := range branches {
		vo.Branches[i].Name = b
		builds, err := httpServ.db.ListRecordPushByBranchName(b, 10, 0)
		vo.Branches[i].Versions = builds
		if err != nil {
			checkNoErr(err)
		}
	}

	dat := make(map[string]interface{})
	dat["Vo"] = vo

	httpServ.render(res, req, "index", dat)
}

func (httpServ *HTTPServer) statusHandler(res http.ResponseWriter, req *http.Request) {
	sha := path.Base(req.RequestURI)
	event, err := httpServ.db.GetPushEventByHead(sha)
	checkNoErr(err)
	ids, err := httpServ.db.GetBuildIDFromPushEventHead(sha)
	checkNoErr(err)
	httpServ.render(res, req, "status", map[string]interface{}{
		"Head": event.HeadCommit.ID,
		"Ref":  event.Ref,
		"Ids":  ids,
	})
}

func (httpServ *HTTPServer) buildsHandler(res http.ResponseWriter, req *http.Request) {
	bidStr := path.Base(req.RequestURI)
	bid, err := strconv.ParseInt(bidStr, 10, 64)
	checkNoErr(err)
	event, err := httpServ.db.GetPushEventByBuildID(bid)
	checkNoErr(err)
	httpServ.render(res, req, "builds", map[string]interface{}{
		"Head": event.HeadCommit.ID,
		"Ref":  event.Ref,
		"Id":   bid,
	})
}

func (httpServ *HTTPServer) buildOutputHandler(res http.ResponseWriter, req *http.Request) {
	queryInt := func(key string) int64 {
		tmpStr := req.URL.Query().Get(key)
		tmp, err := strconv.ParseInt(tmpStr, 10, 64)
		checkNoErr(err)
		return tmp
	}
	id := queryInt("id")
	offset := queryInt("offset")
	limit := queryInt("limit")
	output, err := httpServ.db.GetBuildOutputSince(id, offset, limit)
	checkNoErr(err)
	status, err := httpServ.db.GetBuildStatus(id)
	checkNoErr(err)
	dat, err := json.Marshal(struct {
		Status  string
		Outputs []CommandLineOutput
	}{
		Status:  string(status),
		Outputs: output,
	})
	checkNoErr(err)
	res.Header().Set("Content-Type", "application/json")
	_, err = res.Write(dat)
	checkNoErr(err)
}
