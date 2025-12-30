package server

import (
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"time"

	"strconv"

	"github.com/user/precious-time-tracker/internal/database"
)

func (s *Server) routes() {
	s.Router.HandleFunc("GET /", s.handleIndex)
	s.Router.HandleFunc("POST /start", s.handleStartTimer)
	s.Router.HandleFunc("POST /stop", s.handleStopTimer)
	s.Router.HandleFunc("GET /entry/{id}", s.handleGetEntry)
	s.Router.HandleFunc("GET /entry/{id}/edit", s.handleEditEntry)
	s.Router.HandleFunc("PUT /entry/{id}", s.handleUpdateEntry)
	s.Router.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	// Parse templates
	// Using relative path execution from root
	// Parse templates
	// Using relative path execution from root
	tmpl, err := template.ParseFiles("templates/base.html", "templates/index.html", "templates/fragments.html")
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
		if _, err := s.DB.UpdateTimeEntry(r.Context(), database.UpdateTimeEntryParams{
			EndTime: sql.NullTime{Time: time.Now(), Valid: true},
			ID:      active.ID,
		}); err != nil {
			log.Printf("Failed to stop previous active timer (ID %d): %v", active.ID, err)
		}
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

func (s *Server) handleGetEntry(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	entry, err := s.DB.GetTimeEntry(r.Context(), id)
	if err != nil {
		http.Error(w, "Entry not found", http.StatusNotFound)
		return
	}

	tmpl, err := template.ParseFiles("templates/fragments.html")
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if err := tmpl.ExecuteTemplate(w, "entry-row", entry); err != nil {
		log.Printf("Template execution error: %v", err)
	}
}

func (s *Server) handleEditEntry(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	entry, err := s.DB.GetTimeEntry(r.Context(), id)
	if err != nil {
		http.Error(w, "Entry not found", http.StatusNotFound)
		return
	}

	tmpl, err := template.ParseFiles("templates/fragments.html")
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if err := tmpl.ExecuteTemplate(w, "edit-entry-row", entry); err != nil {
		log.Printf("Template execution error: %v", err)
	}
}

func (s *Server) handleUpdateEntry(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	description := r.FormValue("description")
	if description == "" {
		http.Error(w, "Description required", http.StatusBadRequest)
		return
	}

	// Helper for parsing flexible time formats
	parseTime := func(value string) (time.Time, error) {
		layouts := []string{
			"2006-01-02T15:04:05",
			"2006-01-02 15:04:05",
			"2006-01-02T15:04",
			"2006-01-02 15:04",
		}
		for _, layout := range layouts {
			if t, err := time.Parse(layout, value); err == nil {
				return t, nil
			}
		}
		return time.Time{}, fmt.Errorf("invalid format")
	}

	startTimeStr := r.FormValue("start_time")
	startTime, err := parseTime(startTimeStr)
	if err != nil {
		http.Error(w, "Invalid start time format (use YYYY-MM-DD HH:MM:SS): "+err.Error(), http.StatusBadRequest)
		return
	}

	endTimeStr := r.FormValue("end_time")
	var endTime sql.NullTime
	if endTimeStr != "" {
		et, err := parseTime(endTimeStr)
		if err != nil {
			http.Error(w, "Invalid end time format: "+err.Error(), http.StatusBadRequest)
			return
		}
		endTime = sql.NullTime{Time: et, Valid: true}
	}

	entry, err := s.DB.UpdateTimeEntryFull(r.Context(), database.UpdateTimeEntryFullParams{
		Description: description,
		StartTime:   startTime,
		EndTime:     endTime,
		ID:          id,
	})
	if err != nil {
		http.Error(w, "Failed to update: "+err.Error(), http.StatusInternalServerError)
		return
	}

	tmpl, err := template.ParseFiles("templates/fragments.html")
	if err != nil {
		http.Error(w, "Template error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if err := tmpl.ExecuteTemplate(w, "entry-row", entry); err != nil {
		log.Printf("Template execution error: %v", err)
	}
}
