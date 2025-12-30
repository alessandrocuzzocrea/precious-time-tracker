package server

import (
	"database/sql"
	"html/template"
	"log"
	"net/http"
	"time"

	"github.com/user/precious-time-tracker/internal/database"
)

func (s *Server) routes() {
	s.Router.HandleFunc("GET /", s.handleIndex)
	s.Router.HandleFunc("POST /start", s.handleStartTimer)
	s.Router.HandleFunc("POST /stop", s.handleStopTimer)
	s.Router.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	// Parse templates
	// Using relative path execution from root
	tmpl, err := template.ParseFiles("templates/base.html", "templates/index.html")
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	entries, err := s.DB.ListTimeEntries(r.Context())
	if err != nil {
		log.Printf("Error listing entries: %v", err)
		// Don't fail completely, just show empty
		entries = []database.TimeEntry{}
	}

	active, err := s.DB.GetActiveTimeEntry(r.Context())
	if err != nil && err != sql.ErrNoRows {
		log.Printf("Error getting active entry: %v", err)
	}

	data := map[string]interface{}{
		"Entries": entries,
		"Active":  nil,
	}
	if err == nil {
		data["Active"] = active
	}

	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("Template execution error: %v", err)
	}
}

func (s *Server) handleStartTimer(w http.ResponseWriter, r *http.Request) {
	description := r.FormValue("description")
	if description == "" {
		description = "No description"
	}

	// Stop any currently active timer first? Or just forbid?
	// For simplicity, let's stop any active one.
	active, err := s.DB.GetActiveTimeEntry(r.Context())
	if err == nil {
		s.DB.UpdateTimeEntry(r.Context(), database.UpdateTimeEntryParams{
			EndTime: sql.NullTime{Time: time.Now(), Valid: true},
			ID:      active.ID,
		})
	}

	_, err = s.DB.CreateTimeEntry(r.Context(), database.CreateTimeEntryParams{
		Description: description,
		StartTime:   time.Now(),
	})
	if err != nil {
		http.Error(w, "Failed to start timer", http.StatusInternalServerError)
		return
	}

	// Redirect or return partial?
	// For HTMX, ideally we return the updated list or just redirect to home
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleStopTimer(w http.ResponseWriter, r *http.Request) {
	active, err := s.DB.GetActiveTimeEntry(r.Context())
	if err != nil {
		// Nothing to stop
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	_, err = s.DB.UpdateTimeEntry(r.Context(), database.UpdateTimeEntryParams{
		EndTime: sql.NullTime{Time: time.Now(), Valid: true},
		ID:      active.ID,
	})
	if err != nil {
		http.Error(w, "Failed to stop timer", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}
