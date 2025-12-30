package server

import (
	"database/sql"
	"net/http"

	"github.com/user/precious-time-tracker/internal/database"
)

type Server struct {
	DB     *database.Queries
	RawDB  *sql.DB
	Router *http.ServeMux
}

func NewServer(db *database.Queries, rawDB *sql.DB) *Server {
	s := &Server{
		DB:     db,
		RawDB:  rawDB,
		Router: http.NewServeMux(),
	}
	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.Router.ServeHTTP(w, r)
}
