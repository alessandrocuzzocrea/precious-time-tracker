package server

import (
	"net/http"

	"github.com/user/precious-time-tracker/internal/database"
)

type Server struct {
	DB     *database.Queries
	Router *http.ServeMux
}

func NewServer(db *database.Queries) *Server {
	s := &Server{
		DB:     db,
		Router: http.NewServeMux(),
	}
	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.Router.ServeHTTP(w, r)
}
