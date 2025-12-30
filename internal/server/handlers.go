package server

import (
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/alessandrocuzzocrea/precious-time-tracker/internal/database"
	"github.com/alessandrocuzzocrea/precious-time-tracker/internal/service"
)

type editData struct {
	Entry      interface{} // Can be GetTimeEntryRow or database.TimeEntry
	Categories []database.Category
	Error      string
}

func (s *Server) routes() {
	s.Router.HandleFunc("GET /", s.handleIndex)
	s.Router.HandleFunc("POST /start", s.handleStartTimer)
	s.Router.HandleFunc("POST /stop", s.handleStopTimer)
	s.Router.HandleFunc("GET /entry/{id}", s.handleGetEntry)
	s.Router.HandleFunc("GET /entry/{id}/edit", s.handleEditEntry)
	s.Router.HandleFunc("GET /tags", s.handleListTags)
	s.Router.HandleFunc("GET /categories", s.handleListCategories)
	s.Router.HandleFunc("POST /categories", s.handleCreateCategory)
	s.Router.HandleFunc("POST /categories/{id}", s.handleUpdateCategory)
	s.Router.HandleFunc("DELETE /categories/{id}", s.handleDeleteCategory)
	s.Router.HandleFunc("GET /reports", s.handleReports)
	s.Router.HandleFunc("PUT /entry/{id}", s.handleUpdateEntry)
	s.Router.HandleFunc("PATCH /entry/active", s.handleUpdateActiveEntry)
	s.Router.HandleFunc("DELETE /entry/{id}", s.handleDeleteEntry)
	s.Router.HandleFunc("GET /data", s.handleDataPage)
	s.Router.HandleFunc("GET /export", s.handleExportCSV)
	s.Router.HandleFunc("POST /import", s.handleImportCSV)
	s.Router.HandleFunc("POST /import/preview", s.handlePreviewCSV)
	s.Router.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
}

func formatDuration(start time.Time, end sql.NullTime) string {
	if !end.Valid {
		return "Running"
	}
	d := end.Time.Sub(start)
	return d.Round(time.Second).String()
}

func formatDurationSeconds(seconds int64) string {
	d := time.Duration(seconds) * time.Second
	if d < time.Hour {
		return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
}

func (s *Server) render(w http.ResponseWriter, r *http.Request, tmplName string, data interface{}, files ...string) {
	funcs := template.FuncMap{
		"duration":         formatDuration,
		"duration_seconds": formatDurationSeconds,
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
		entries = []database.ListTimeEntriesRow{}
	}
	categories, err := s.Service.ListCategories(r.Context())
	if err != nil {
		log.Printf("Error listing categories: %v", err)
		categories = []database.Category{}
	}

	data := map[string]interface{}{
		"Entries":    entries,
		"Categories": categories,
	}
	// Active will be filled by render if tmplName is ""

	s.render(w, r, "", data, "templates/base.html", "templates/index.html")
}

func (s *Server) handleStartTimer(w http.ResponseWriter, r *http.Request) {
	description := r.FormValue("description")
	catIDStr := r.FormValue("category_id")
	var catID *int64
	if catIDStr != "" {
		if id, err := strconv.ParseInt(catIDStr, 10, 64); err == nil {
			catID = &id
		}
	}

	_, err := s.Service.StartTimer(r.Context(), description, catID)
	if err != nil {
		http.Error(w, "Failed to start timer: "+err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleUpdateActiveEntry(w http.ResponseWriter, r *http.Request) {
	active, err := s.Service.GetActiveTimeEntry(r.Context())
	if err != nil {
		log.Printf("Error getting active entry: %v", err)
		http.Error(w, "No active entry", http.StatusNotFound)
		return
	}

	description := r.FormValue("description")
	categoryIDStr := r.FormValue("category_id")

	var categoryID *int64
	if categoryIDStr != "" {
		id, err := strconv.ParseInt(categoryIDStr, 10, 64)
		if err == nil {
			categoryID = &id
		}
	}

	_, err = s.Service.UpdateTimeEntry(r.Context(), active.ID, description, active.StartTime, active.EndTime, categoryID)
	if err != nil {
		log.Printf("Error updating active entry: %v", err)
		http.Error(w, "Failed to update", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
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

	categories, _ := s.Service.ListCategories(r.Context())

	s.render(w, r, "edit-entry-row", editData{Entry: entry, Categories: categories})
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

	catIDStr := r.FormValue("category_id")
	var catID *int64
	if catIDStr != "" {
		if cid, err := strconv.ParseInt(catIDStr, 10, 64); err == nil {
			catID = &cid
		}
	}

	entry, err := s.Service.UpdateTimeEntry(r.Context(), id, description, startTime, endTime, catID)
	if err != nil {
		categories, _ := s.Service.ListCategories(r.Context())
		s.render(w, r, "edit-entry-row", editData{Entry: originalEntry, Categories: categories, Error: "Failed to update: " + err.Error()})
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

func (s *Server) handleListCategories(w http.ResponseWriter, r *http.Request) {
	categories, err := s.Service.ListCategories(r.Context())
	if err != nil {
		log.Printf("Error listing categories: %v", err)
		http.Error(w, "Failed to list categories", http.StatusInternalServerError)
		return
	}

	data := map[string]interface{}{
		"Categories": categories,
	}

	s.render(w, r, "", data, "templates/base.html", "templates/categories.html")
}

func (s *Server) handleCreateCategory(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	color := r.FormValue("color")
	if color == "" {
		color = "#cccccc"
	}

	_, err := s.Service.CreateCategory(r.Context(), name, color)
	if err != nil {
		http.Error(w, "Failed to create category: "+err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/categories", http.StatusSeeOther)
}

func (s *Server) handleUpdateCategory(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	name := r.FormValue("name")
	color := r.FormValue("color")

	_, err = s.Service.UpdateCategory(r.Context(), id, name, color)
	if err != nil {
		http.Error(w, "Failed to update category: "+err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/categories", http.StatusSeeOther)
}

func (s *Server) handleReports(w http.ResponseWriter, r *http.Request) {
	period := r.URL.Query().Get("period")
	if period == "" {
		period = "today"
	}

	now := time.Now()
	var start, end time.Time

	switch period {
	case "today":
		start = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		end = start.AddDate(0, 0, 1).Add(-time.Second)
	case "week":
		// Assume week starts on Monday
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		start = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -weekday+1)
		end = start.AddDate(0, 0, 7).Add(-time.Second)
	case "month":
		start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		end = start.AddDate(0, 1, 0).Add(-time.Second)
	case "year":
		start = time.Date(now.Year(), 1, 1, 0, 0, 0, 0, now.Location())
		end = start.AddDate(1, 0, 0).Add(-time.Second)
	default: // "all" or anything else
		start = time.Time{}
		end = now.AddDate(100, 0, 0) // Far future
	}

	catFilterStr := r.URL.Query().Get("category_id")
	var catFilter int64
	if catFilterStr != "" {
		catFilter, _ = strconv.ParseInt(catFilterStr, 10, 64)
	}

	tagIDsStr := r.URL.Query()["tag_ids"]
	var tagIDs []int64
	for _, idStr := range tagIDsStr {
		if id, err := strconv.ParseInt(idStr, 10, 64); err == nil {
			tagIDs = append(tagIDs, id)
		}
	}

	report, err := s.Service.GetReport(r.Context(), service.ReportFilter{
		StartDate:      start,
		EndDate:        end,
		CategoryFilter: catFilter,
		TagIDs:         tagIDs,
	})
	if err != nil {
		log.Printf("Error getting report: %v", err)
		http.Error(w, "Failed to get report", http.StatusInternalServerError)
		return
	}

	categories, _ := s.Service.ListCategories(r.Context())
	tags, _ := s.Service.ListTags(r.Context())

	data := map[string]interface{}{
		"Report":           report,
		"Categories":       categories,
		"Tags":             tags,
		"Period":           period,
		"SelectedCategory": catFilter,
		"SelectedTags":     tagIDs,
	}

	if r.Header.Get("HX-Request") == "true" {
		s.render(w, r, "report-content", data, "templates/reports.html")
	} else {
		s.render(w, r, "", data, "templates/base.html", "templates/reports.html")
	}
}

func (s *Server) handleDeleteCategory(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	if err := s.Service.DeleteCategory(r.Context(), id); err != nil {
		http.Error(w, "Failed to delete category", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleDataPage(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{
		"Success": r.URL.Query().Get("success") == "1",
	}
	s.render(w, r, "", data, "templates/base.html", "templates/data.html")
}

func (s *Server) handleExportCSV(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment;filename=time-entries.csv")
	if err := s.Service.ExportCSV(r.Context(), w); err != nil {
		log.Printf("Export error: %v", err)
		// Can't really send error after headers, but we can try
	}
}

func (s *Server) handleImportCSV(w http.ResponseWriter, r *http.Request) {
	file, _, err := r.FormFile("csv_file")
	if err != nil {
		http.Error(w, "Failed to get file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	if err := s.Service.ImportCSV(r.Context(), file); err != nil {
		log.Printf("Import error: %v", err)
		http.Error(w, "Import failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/data?success=1", http.StatusSeeOther)
}

func (s *Server) handlePreviewCSV(w http.ResponseWriter, r *http.Request) {
	file, _, err := r.FormFile("csv_file")
	if err != nil {
		http.Error(w, "Failed to get file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	preview, err := s.Service.PreviewCSV(r.Context(), file)
	if err != nil {
		log.Printf("Preview error: %v", err)
		http.Error(w, "Preview failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.render(w, r, "csv-preview", preview)
}
