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

type editData struct {
	Entry database.TimeEntry
	Error string
}

func (s *Server) routes() {
	s.Router.HandleFunc("GET /", s.handleIndex)
	s.Router.HandleFunc("POST /start", s.handleStartTimer)
	s.Router.HandleFunc("POST /stop", s.handleStopTimer)
	s.Router.HandleFunc("GET /entry/{id}", s.handleGetEntry)
	s.Router.HandleFunc("GET /entry/{id}/edit", s.handleEditEntry)
	s.Router.HandleFunc("GET /tags", s.handleListTags)
	s.Router.HandleFunc("PUT /entry/{id}", s.handleUpdateEntry)
	s.Router.HandleFunc("DELETE /entry/{id}", s.handleDeleteEntry)
	s.Router.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
}

func formatDuration(start time.Time, end sql.NullTime) string {
	if !end.Valid {
		return "Running"
	}
	d := end.Time.Sub(start)
	return d.Round(time.Second).String()
}

func (s *Server) render(w http.ResponseWriter, r *http.Request, tmplName string, data interface{}, files ...string) {
	funcs := template.FuncMap{
		"duration": formatDuration,
	}

	allFiles := append([]string{"templates/fragments.html"}, files...)

	t, err := template.New("").Funcs(funcs).ParseFiles(allFiles...)
	if err != nil {
		http.Error(w, "Template parse error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Prepare data for rendering
	var finalData interface{}
	if tmplName == "" {
		// Full page render: ensure Active entry is available for the sticky bar
		active, err := s.Service.GetActiveTimeEntry(r.Context())
		if err != nil && err != sql.ErrNoRows {
			log.Printf("Error getting active entry for render: %v", err)
		}

		m := make(map[string]interface{})
		if data != nil {
			if existingMap, ok := data.(map[string]interface{}); ok {
				m = existingMap
			} else {
				m["PageData"] = data
			}
		}

		if err == nil {
			m["Active"] = active
		} else {
			m["Active"] = nil
		}
		finalData = m
	} else {
		finalData = data
	}

	if tmplName == "" {
		if err := t.ExecuteTemplate(w, "base.html", finalData); err != nil {
			log.Printf("Template execution error: %v", err)
		}
	} else {
		if err := t.ExecuteTemplate(w, tmplName, finalData); err != nil {
			log.Printf("Template execution error: %v", err)
		}
	}
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	entries, err := s.Service.ListTimeEntries(r.Context())
	if err != nil {
		log.Printf("Error listing entries: %v", err)
		entries = []database.TimeEntry{}
	}

	data := map[string]interface{}{
		"Entries": entries,
	}
	// Active will be filled by render if tmplName is ""

	s.render(w, r, "", data, "templates/base.html", "templates/index.html")
}

func (s *Server) handleStartTimer(w http.ResponseWriter, r *http.Request) {
	description := r.FormValue("description")

	_, err := s.Service.StartTimer(r.Context(), description)
	if err != nil {
		http.Error(w, "Failed to start timer: "+err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleStopTimer(w http.ResponseWriter, r *http.Request) {
	if err := s.Service.StopTimer(r.Context()); err != nil {
		http.Error(w, "Failed to stop timer: "+err.Error(), http.StatusInternalServerError)
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

	entry, err := s.Service.GetTimeEntry(r.Context(), id)
	if err != nil {
		http.Error(w, "Entry not found", http.StatusNotFound)
		return
	}

	s.render(w, r, "entry-row", entry)
}

func (s *Server) handleEditEntry(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	entry, err := s.Service.GetTimeEntry(r.Context(), id)
	if err != nil {
		http.Error(w, "Entry not found", http.StatusNotFound)
		return
	}

	s.render(w, r, "edit-entry-row", editData{Entry: entry})
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

	// Fetch original entry to use as fallback/template
	originalEntry, err := s.Service.GetTimeEntry(r.Context(), id)
	if err != nil {
		http.Error(w, "Entry not found", http.StatusNotFound)
		return
	}

	startTimeStr := r.FormValue("start_time")
	startTime, err := parseTime(startTimeStr)
	if err != nil {
		s.render(w, r, "edit-entry-row", editData{Entry: originalEntry, Error: "Invalid start time format"})
		return
	}

	endTimeStr := r.FormValue("end_time")
	var endTime sql.NullTime
	if endTimeStr != "" {
		et, err := parseTime(endTimeStr)
		if err != nil {
			s.render(w, r, "edit-entry-row", editData{Entry: originalEntry, Error: "Invalid end time format"})
			return
		}
		if !et.After(startTime) {
			unsavedEntry := originalEntry
			unsavedEntry.Description = description
			unsavedEntry.StartTime = startTime
			unsavedEntry.EndTime = sql.NullTime{Time: et, Valid: true}
			s.render(w, r, "edit-entry-row", editData{Entry: unsavedEntry, Error: "End time must be after start time"})
			return
		}
		endTime = sql.NullTime{Time: et, Valid: true}
	}

	entry, err := s.Service.UpdateTimeEntry(r.Context(), id, description, startTime, endTime)
	if err != nil {
		s.render(w, r, "edit-entry-row", editData{Entry: originalEntry, Error: "Failed to update: " + err.Error()})
		return
	}

	s.render(w, r, "entry-row", entry)
}

func (s *Server) handleDeleteEntry(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	if err := s.Service.DeleteTimeEntry(r.Context(), id); err != nil {
		http.Error(w, "Failed to delete entry", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleListTags(w http.ResponseWriter, r *http.Request) {
	tags, err := s.Service.ListTags(r.Context())
	if err != nil {
		log.Printf("Error listing tags: %v", err)
		http.Error(w, "Failed to list tags", http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"Tags": tags,
	}

	s.render(w, r, "", data, "templates/base.html", "templates/tags.html")
}
