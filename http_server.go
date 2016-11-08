// for Github Webhook, and CI website.
package main

import (
	"github.com/bmatsuo/go-jsontree"
	"github.com/reyoung/github_hook"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/urfave/negroni"
)

// PushEvent that ci server actually used.
type PushEvent struct {
	Ref      string
	Head     string
	CloneUrl string
}

// Get branch name from PushEvent. Used for template
func (ev *PushEvent) BranchName() string {
	return ev.Ref[11:]
}

// Http Server
type HttpServer struct {
	EventQueue chan interface{}	// Channel for git hub web hook event
	server     *github_hook.Server  // Github Webhook handler.
	addr       string  // http addr.

	router *mux.Router  // Router
	n *negroni.Negroni // A http middleware framework

	db *CIDB  // Database
}

func newHttpServer(opts *HttpOption, db *CIDB) *HttpServer {
	serv := &HttpServer{
		EventQueue: nil,
		server:     github_hook.NewServer(),
		addr:       opts.Addr,
		router:     mux.NewRouter(),
		n:          negroni.New(),
		db:         db,
	}
	serv.EventQueue = serv.server.Events
	serv.server.Path = opts.CIUri
	serv.server.Secret = opts.Secret
	serv.router.HandleFunc(serv.server.Path, serv.server.ServeHTTP)
	serv.server.EventHandler = make(map[string]func(*jsontree.JsonTree) (interface{}, error))
	serv.server.EventHandler["push"] = on_push_event

	serv.n.Use(negroni.NewRecovery())
	serv.n.Use(negroni.NewLogger())
	serv.router.HandleFunc("/", serv.homeHandler).Methods("Get").Name("home")
	//serv.router.HandleFunc(fmt.Sprintf("%s{sha:[0-9a-f]+}", opts.StatusUri), serv.statusHandler).Methods(
	//	"Get").Name("status")
	serv.n.UseHandler(serv.router)
	return serv
}

func (httpServ *HttpServer) ListenAndServe() error {
	return http.ListenAndServe(httpServ.addr, httpServ.n)
}

func (httpServ *HttpServer) GoListenAndServe() {
	go func() {
		CheckNoErr(httpServ.ListenAndServe())
	}()
}

// PushEvent represents the JSON payload from Github push
// Webhook. https://developer.github.com/v3/activity/events/types/#pushevent
func on_push_event(request *jsontree.JsonTree) (ev interface{}, err error) {
	event := &PushEvent{}
	ev = event
	event.Ref, err = request.Get("ref").String()
	if err != nil {
		return
	}
	event.CloneUrl, err = request.Get("repository").Get("clone_url").String()
	if err != nil {
		return
	}
	event.Head, err = request.Get("head_commit").Get("id").String()
	return
}

func (httpServ *HttpServer) homeHandler(http.ResponseWriter,
	*http.Request) {
}

//func (httpServ* HttpServer) statusHandler(res http.ResponseWriter, req *http.Request) {
//	sha := path.Base(req.URL)
//	template.ParseFiles()
//}
