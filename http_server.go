// for Github Webhook, and CI website.
package main

import (
	"net/http"
	"os"

	"github.com/bmatsuo/go-jsontree"
	"github.com/reyoung/github_hook"

	"fmt"
	"html/template"

	"path/filepath"

	"strings"

	"log"

	"errors"

	"github.com/gorilla/context"
	"github.com/gorilla/mux"
	"github.com/urfave/negroni"
)

// PushEvent that ci server actually used.
type PushEvent struct {
	Ref      string
	Head     string
	CloneURL string
}

// BranchName Get branch name from PushEvent. Used for template
func (ev *PushEvent) BranchName() string {
	return ev.Ref[11:]
}

// HTTPServer for webhook, website.
type HTTPServer struct {
	EventQueue chan interface{}    // Channel for git hub web hook event
	server     *github_hook.Server // Github Webhook handler.
	addr       string              // http addr.

	router *mux.Router      // Router
	n      *negroni.Negroni // A http middleware framework

	db *CIDB // Database

	renderer *Renderer
}

// Renderer is a http middleware for render template
type Renderer struct {
	tmpls map[string]*template.Template
	opts  *Options
}

func newRenderer(opts *Options) *Renderer {
	tmpls := make(map[string]*template.Template)
	tmplDir := opts.HTTP.TemplateDir
	if strings.HasPrefix(opts.HTTP.TemplateDir, "./") {
		tmplDir = tmplDir[len("./"):]
	}
	checkNoErr(filepath.Walk(tmplDir, func(p string, info os.FileInfo, err error) error {
		if !info.IsDir() {
			if strings.HasSuffix(p, ".gohtml") && !strings.HasSuffix(p, "base.gohtml") {
				tmpl := template.Must(template.ParseFiles(fmt.Sprintf("%sbase.gohtml", opts.HTTP.TemplateDir), p))
				if strings.HasPrefix(p, tmplDir) {
					p = p[len(tmplDir):]
					log.Println("Loading template ", p)
					tmpls[p] = tmpl
				} else {
					return errors.New(fmt.Sprint("Error when loading template ", p))
				}
			}
		}
		return nil
	}))

	return &Renderer{
		tmpls: tmpls,
		opts:  opts,
	}
}

func (renderer *Renderer) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	context.Set(r, "renderer", renderer)
	next(rw, r)
}

func (renderer *Renderer) render(rw http.ResponseWriter, name string, data map[string]string) {
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

func newHTTPServer(opts *Options, db *CIDB) *HTTPServer {
	serv := &HTTPServer{
		EventQueue: nil,
		server:     github_hook.NewServer(),
		addr:       opts.HTTP.Addr,
		router:     mux.NewRouter(),
		n:          negroni.New(),
		db:         db,
		renderer:   newRenderer(opts),
	}
	serv.EventQueue = serv.server.Events
	serv.server.Path = opts.HTTP.CIUri
	serv.server.Secret = opts.HTTP.Secret
	serv.router.HandleFunc(serv.server.Path, serv.server.ServeHTTP)
	serv.server.EventHandler = make(map[string]func(*jsontree.JsonTree) (interface{}, error))
	serv.server.EventHandler["push"] = onPushEvent
	serv.n.Use(negroni.NewRecovery())
	serv.n.Use(negroni.NewLogger())
	serv.n.Use(serv.renderer)
	serv.router.HandleFunc("/", serv.homeHandler).Methods("Get").Name("home")
	//serv.router.HandleFunc(fmt.Sprintf("%s{sha:[0-9a-f]+}", opts.StatusUri), serv.statusHandler).Methods(
	//	"Get").Name("status")
	serv.n.UseHandler(serv.router)
	return serv
}

// ListenAndServe by using configuration.
func (httpServ *HTTPServer) ListenAndServe() error {
	return http.ListenAndServe(httpServ.addr, httpServ.n)
}

// onPushEvent Webhook. https://developer.github.com/v3/activity/events/types/#pushevent
func onPushEvent(request *jsontree.JsonTree) (ev interface{}, err error) {
	event := &PushEvent{}
	ev = event
	event.Ref, err = request.Get("ref").String()
	if err != nil {
		return
	}
	event.CloneURL, err = request.Get("repository").Get("clone_url").String()
	if err != nil {
		return
	}
	event.Head, err = request.Get("head_commit").Get("id").String()
	return
}

func (httpServ *HTTPServer) render(res http.ResponseWriter, req *http.Request, name string, data map[string]string) {
	context.Get(req, "renderer").(*Renderer).render(res, name, data)
}

func (httpServ *HTTPServer) homeHandler(res http.ResponseWriter, req *http.Request) {
	httpServ.render(res, req, "index", make(map[string]string))
}

//func (httpServ* HttpServer) statusHandler(res http.ResponseWriter, req *http.Request) {
//	sha := path.Base(req.URL)
//	template.ParseFiles()
//}
