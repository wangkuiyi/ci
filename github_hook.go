package main

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"

	"github.com/bmatsuo/go-jsontree"
)

type Server struct {
	Port         int              // Port to listen on. Defaults to 80
	Path         string           // Path to receive on. Defaults to "/postreceive"
	Secret       string           // Option secret key for authenticating via HMAC
	IgnoreTags   bool             // If set to false, also execute command if tag is pushed
	Events       chan interface{} // Channel of events.
	EventHandler map[string]func(*jsontree.JsonTree) (interface{}, error)
}

// Create a new server with sensible defaults.
// By default the Port is set to 80 and the Path is set to `/postreceive`
func NewServer() *Server {
	return &Server{
		Port:         80,
		Path:         "/postreceive",
		IgnoreTags:   true,
		Events:       make(chan interface{}, 10), // buffered to 10 items
		EventHandler: make(map[string]func(*jsontree.JsonTree) (interface{}, error)),
	}
}

// Spin up the server and listen for github webhook push events. The events will be passed to Server.Events channel.
func (s *Server) ListenAndServe() error {
	return http.ListenAndServe(":"+strconv.Itoa(s.Port), s)
}

// Inside a go-routine, spin up the server and listen for github webhook push events. The events will be passed to Server.Events channel.
func (s *Server) GoListenAndServe() {
	go func() {
		err := s.ListenAndServe()
		if err != nil {
			panic(err)
		}
	}()
}

// Satisfies the http.Handler interface.
// Instead of calling Server.ListenAndServe you can integrate hookserve.Server inside your own http server.
// If you are using hookserve.Server in his way Server.Path should be set to match your mux pattern and Server.Port will be ignored.
func (s *Server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	defer req.Body.Close()

	fmt.Println(req.Method, req.URL.Path, s.Path)

	if req.Method != "POST" {
		http.Error(w, "405 Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if req.URL.Path != s.Path {
		http.Error(w, "404 Not found", http.StatusNotFound)
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

	// If we have a Secret set, we should check the MAC
	if s.Secret != "" {
		sig := req.Header.Get("X-Hub-Signature")

		if sig == "" {
			http.Error(w, "403 Forbidden - Missing X-Hub-Signature required for HMAC verification", http.StatusForbidden)
			return
		}

		mac := hmac.New(sha1.New, []byte(s.Secret))
		mac.Write(body)
		expectedMAC := mac.Sum(nil)
		expectedSig := "sha1=" + hex.EncodeToString(expectedMAC)
		if !hmac.Equal([]byte(expectedSig), []byte(sig)) {
			http.Error(w, "403 Forbidden - HMAC verification failed", http.StatusForbidden)
			return
		}
	}

	request := jsontree.New()
	err = request.UnmarshalJSON(body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	handler, ok := s.EventHandler[eventType]
	if !ok {
		http.Error(w, "Unknown Event Type "+eventType, http.StatusInternalServerError)
		return
	} else {
		ev, err := handler(request)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// We've built our Event - put it into the channel and we're done
		go func() {
			s.Events <- ev
		}()
	}

	w.Write([]byte(string("ok")))
}
