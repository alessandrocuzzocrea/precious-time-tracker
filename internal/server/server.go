package server

import (
	"net/http"

	"github.com/user/precious-time-tracker/internal/service"
)

type Server struct {
	Service *service.Service
	Router  *http.ServeMux
}

func NewServer(svc *service.Service) *Server {
	s := &Server{
		Service: svc,
		Router:  http.NewServeMux(),
	}
	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.Router.ServeHTTP(w, r)
}
