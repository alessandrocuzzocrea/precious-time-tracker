package main

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alessandrocuzzocrea/precious-time-tracker/internal/database"
	"github.com/alessandrocuzzocrea/precious-time-tracker/internal/server"
	"github.com/alessandrocuzzocrea/precious-time-tracker/internal/service"
	"github.com/alessandrocuzzocrea/precious-time-tracker/sql/schema"
	"github.com/pressly/goose/v3"

	_ "modernc.org/sqlite"
)

func getProjectRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(wd, "go.mod")); err == nil {
			return wd, nil
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			return "", os.ErrNotExist
		}
		wd = parent
	}
}

func newTestServer(t *testing.T) *server.Server {
	// Setup in-memory DB
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}

	// Create a temp file or just use the FS with goose
	goose.SetBaseFS(schema.FS)
	if err := goose.SetDialect("sqlite"); err != nil {
		t.Fatalf("failed to set dialect: %v", err)
	}
	// We need to disable logging or it will span stdout
	// goose.SetLogger(goose.NopLogger())

	if err := goose.Up(db, "."); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	dbQueries := database.New(db)
	svc := service.New(dbQueries, db)
	return server.NewServer(svc)
}

func TestHandleIndex(t *testing.T) {
	root, err := getProjectRoot()
	if err != nil {
		t.Fatalf("failed to find project root: %v", err)
	}
	// Temporarily change to root for templates
	oldWd, _ := os.Getwd()
	if err := os.Chdir(root); err != nil {
		t.Fatalf("failed to chdir to root: %v", err)
	}
	defer func() {
		if err := os.Chdir(oldWd); err != nil {
			t.Errorf("failed to restore wd: %v", err)
		}
	}()

	srv := newTestServer(t)
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Precious Time") {
		t.Errorf("expected body to contain title")
	}
}

func TestHandleStartTimer(t *testing.T) {
	// StartTimer handler issues redirect and DB writes, doesn't strictly need templates
	// effectively, but let's be safe and consistent.
	root, err := getProjectRoot()
	if err != nil {
		t.Fatalf("failed to find project root: %v", err)
	}
	oldWd, _ := os.Getwd()
	if err := os.Chdir(root); err != nil {
		t.Fatalf("failed to chdir to root: %v", err)
	}
	defer func() {
		if err := os.Chdir(oldWd); err != nil {
			t.Errorf("failed to restore wd: %v", err)
		}
	}()

	srv := newTestServer(t)

	form := url.Values{}
	form.Add("description", "Integration Test Task")
	req := httptest.NewRequest("POST", "/start", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusSeeOther {
		t.Errorf("expected redirect 303, got %d", resp.StatusCode)
	}

	// Verify DB
	ctx := context.Background()
	active, err := srv.Service.GetActiveTimeEntry(ctx)
	if err != nil {
		t.Fatalf("failed to get active entry: %v", err)
	}
	if active.Description != "Integration Test Task" {
		t.Errorf("expected description 'Integration Test Task', got %s", active.Description)
	}
}

func TestHandleEditAndUpdate(t *testing.T) {
	root, err := getProjectRoot()
	if err != nil {
		t.Fatalf("failed to find project root: %v", err)
	}
	oldWd, _ := os.Getwd()
	if err := os.Chdir(root); err != nil {
		t.Fatalf("failed to chdir to root: %v", err)
	}
	defer func() {
		if err := os.Chdir(oldWd); err != nil {
			t.Errorf("failed to restore wd: %v", err)
		}
	}()

	srv := newTestServer(t)

	// Create an entry
	ctx := context.Background()
	entry, err := srv.Service.StartTimer(ctx, "Old Description", nil)
	if err != nil {
		t.Fatalf("failed to create entry: %v", err)
	}
	// Stop it to make it a past entry? Or just edit running? Test edits existing.
	// Update works on ID.

	// 1. GET /entry/{id}/edit -> Should return the form
	req := httptest.NewRequest("GET", "/entry/"+fmt.Sprintf("%d", entry.ID)+"/edit", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Result().StatusCode)
	}
	if !strings.Contains(w.Body.String(), "input type=\"text\"") {
		t.Errorf("expected body to contain input field")
	}

	// 2. PUT /entry/{id} -> Should update description
	form := url.Values{}
	form.Add("description", "New Description")
	// Must provide start_time as form requires parsing it back, usually hidden input or preserved.
	// The handler expects start_time logic.
	// If I don't provide start_time in form, handler fails "Invalid start time format".
	// Test needs to simulate full form submission.
	form.Add("start_time", entry.StartTime.Format("2006-01-02T15:04:05"))

	req = httptest.NewRequest("PUT", "/entry/"+fmt.Sprintf("%d", entry.ID), strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Result().StatusCode)
	}
	if !strings.Contains(w.Body.String(), "New Description") {
		body := w.Body.String()
		if len(body) > 200 {
			body = body[:200] + "..."
		}
		t.Errorf("expected response to contain new description, got: %s", body)
	}

	// Verify DB
	updated, err := srv.Service.GetTimeEntry(ctx, entry.ID)
	if err != nil {
		t.Fatalf("failed to get entry: %v", err)
	}
	if updated.Description != "New Description" {
		t.Errorf("expected description 'New Description', got %s", updated.Description)
	}
}

func TestHandleUpdateActiveEntry(t *testing.T) {
	srv := newTestServer(t)
	ctx := context.Background()

	// 1. Create a category
	cat, err := srv.Service.CreateCategory(ctx, "Test Cat", "#ff0000")
	if err != nil {
		t.Fatalf("failed to create category: %v", err)
	}

	// 2. Start a timer
	_, err = srv.Service.StartTimer(ctx, "Initial Description", nil)
	if err != nil {
		t.Fatalf("failed to start timer: %v", err)
	}

	// 3. Update it via PATCH /entry/active
	form := url.Values{}
	form.Add("description", "Updated Live Description")
	form.Add("category_id", fmt.Sprintf("%d", cat.ID))

	req := httptest.NewRequest("PATCH", "/entry/active", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Result().StatusCode)
	}

	// 4. Verify DB
	active, err := srv.Service.GetActiveTimeEntry(ctx)
	if err != nil {
		t.Fatalf("failed to get active entry: %v", err)
	}
	if active.Description != "Updated Live Description" {
		t.Errorf("expected description 'Updated Live Description', got %s", active.Description)
	}
	if !active.CategoryID.Valid || active.CategoryID.Int64 != cat.ID {
		t.Errorf("expected category ID %d, got %v", cat.ID, active.CategoryID)
	}
}

func TestHandleLists(t *testing.T) {
	root, _ := getProjectRoot()
	oldWd, _ := os.Getwd()
	os.Chdir(root)
	defer os.Chdir(oldWd)

	srv := newTestServer(t)

	// List Tags
	req := httptest.NewRequest("GET", "/tags", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("GET /tags expected 200, got %d", w.Result().StatusCode)
	}

	// List Categories
	req = httptest.NewRequest("GET", "/categories", nil)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("GET /categories expected 200, got %d", w.Result().StatusCode)
	}
}

func TestHandleReports(t *testing.T) {
	root, _ := getProjectRoot()
	oldWd, _ := os.Getwd()
	os.Chdir(root)
	defer os.Chdir(oldWd)

	srv := newTestServer(t)

	// Test various periods
	periods := []string{"today", "week", "month", "year", "all"}
	for _, p := range periods {
		req := httptest.NewRequest("GET", "/reports?period="+p, nil)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)
		if w.Result().StatusCode != http.StatusOK {
			t.Errorf("GET /reports?period=%s expected 200, got %d", p, w.Result().StatusCode)
		}
	}
}

func TestHandleDataPageAndExport(t *testing.T) {
	root, _ := getProjectRoot()
	oldWd, _ := os.Getwd()
	os.Chdir(root)
	defer os.Chdir(oldWd)

	srv := newTestServer(t)

	// Data Page
	req := httptest.NewRequest("GET", "/data", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("GET /data expected 200, got %d", w.Result().StatusCode)
	}

	// Export CSV
	req = httptest.NewRequest("GET", "/export", nil)
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("GET /export expected 200, got %d", w.Result().StatusCode)
	}
	if w.Header().Get("Content-Type") != "text/csv" {
		t.Errorf("expected Content-Type text/csv, got %s", w.Header().Get("Content-Type"))
	}
}

func TestHandleImportPreview(t *testing.T) {
	root, _ := getProjectRoot()
	oldWd, _ := os.Getwd()
	os.Chdir(root)
	defer os.Chdir(oldWd)

	srv := newTestServer(t)

	// Prepare multipart form with a CSV file
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	fw, _ := w.CreateFormFile("csv_file", "test.csv")
	fw.Write([]byte("id,description,start_time,end_time,category\n,Test Item,2024-01-01T10:00:00Z,,Work"))
	w.Close()

	req := httptest.NewRequest("POST", "/import/preview", &b)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Result().StatusCode != http.StatusOK {
		t.Errorf("POST /import/preview expected 200, got %d", rec.Result().StatusCode)
	}
}
