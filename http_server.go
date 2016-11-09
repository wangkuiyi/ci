// for Github Webhook, and CI website.
package main

import (
	"net/http"

	"github.com/bmatsuo/go-jsontree"
	"github.com/reyoung/github_hook"

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
}

func newHTTPServer(opts *HTTPOption, db *CIDB) *HTTPServer {
	serv := &HTTPServer{
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
	serv.server.EventHandler["push"] = onPushEvent

	serv.n.Use(negroni.NewRecovery())
	serv.n.Use(negroni.NewLogger())
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

func (httpServ *HTTPServer) homeHandler(http.ResponseWriter,
	*http.Request) {
}

//func (httpServ* HttpServer) statusHandler(res http.ResponseWriter, req *http.Request) {
//	sha := path.Base(req.URL)
//	template.ParseFiles()
//}
