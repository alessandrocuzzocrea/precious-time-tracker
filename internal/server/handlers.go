package server

import (
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"time"

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

func formatDuration(start time.Time, end sql.NullTime) string {
	if !end.Valid {
		// Calculate current duration relative to now?
		// Or just return "-" or "Running"
		// "Running" is clearer.
		return "Running"
	}
	d := end.Time.Sub(start)
	return d.Round(time.Second).String()
}

func (s *Server) render(w http.ResponseWriter, tmplName string, data interface{}, files ...string) {
	funcs := template.FuncMap{
		"duration": formatDuration,
	}

	// Always include fragments
	allFiles := append([]string{"templates/fragments.html"}, files...)

	// Deduplicate if needed, but ParseFiles handles it? No, duplicates might error or override.
	// Actually, just passing "templates/base.html" and "templates/index.html" is fine.
	// I'll make the helper take the specific files needed for that view.

	t, err := template.New("").Funcs(funcs).ParseFiles(allFiles...)
	if err != nil {
		http.Error(w, "Template parse error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// If tmplName is empty, just Execute (for full pages usually base or index) - wait,
	// Execute executes the first one or specific?
	// If I use "base.html" which defines "content", I execute "base.html".
	// If I use "entry-row", I execute "entry-row".

	if tmplName == "" {
		// Guess defaults? No, be explicit.
		// For index.html, it defines "content" but we usually execute the root or "base.html"?
		// My base.html executes "content".
		// Index.html defines "content".
		// I should execute "base.html" (which is just the file name usually).
		// Wait, ParseFiles returns a template where the name is the *first filename*.
		if err := t.ExecuteTemplate(w, "base.html", data); err != nil {
			log.Printf("Template execution error: %v", err)
		}
	} else {
		if err := t.ExecuteTemplate(w, tmplName, data); err != nil {
			log.Printf("Template execution error: %v", err)
		}
	}
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	entries, err := s.DB.ListTimeEntries(r.Context())
	if err != nil {
		log.Printf("Error listing entries: %v", err)
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

	s.render(w, "", data, "templates/base.html", "templates/index.html")
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

	s.render(w, "entry-row", entry)
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

	s.render(w, "edit-entry-row", entry)
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

	s.render(w, "entry-row", entry)
}
