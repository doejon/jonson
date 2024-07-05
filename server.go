package jonson

import (
	"net/http"
)

// A handler will be handled by the server.
// You can decide which handler you feel like mounting, such as default http handlers,
// websocket handlers, an ajax rpc endpoint or exposing
// each rpc method as its own http endpoint.
type Handler interface {
	Handle(w http.ResponseWriter, req *http.Request) bool
}

// Server ...
type Server struct {
	handlers []Handler
}

// NewServer returns a new Server.
// The handlers provided to the server will be handled through iteration of provided handlers:
// the first handler returning "true" will stop the iteration through the handlers
// and the request will be seen as served.
func NewServer(handlers ...Handler) *Server {
	return &Server{
		handlers: handlers,
	}
}

// ServeHTTP implements the http.Handler interface
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	for _, v := range s.handlers {
		if v.Handle(w, r) {
			return
		}
	}
	w.WriteHeader(http.StatusNotFound)
}

// ListenAndServe will start listening on http on the given addr
func (s *Server) ListenAndServe(addr string) error {
	return http.ListenAndServe(addr, s)
}
