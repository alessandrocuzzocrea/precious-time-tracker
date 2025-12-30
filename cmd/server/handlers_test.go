package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/user/precious-time-tracker/internal/database"
	"github.com/user/precious-time-tracker/internal/server"
	"github.com/user/precious-time-tracker/sql/schema"

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
	return server.NewServer(dbQueries, db)
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
	active, err := srv.DB.GetActiveTimeEntry(ctx)
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
	entry, err := srv.DB.CreateTimeEntry(ctx, database.CreateTimeEntryParams{
		Description: "Old Description",
		StartTime:   time.Now(),
	})
	if err != nil {
		t.Fatalf("failed to create entry: %v", err)
	}

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
	updated, err := srv.DB.GetTimeEntry(ctx, entry.ID)
	if err != nil {
		t.Fatalf("failed to get entry: %v", err)
	}
	if updated.Description != "New Description" {
		t.Errorf("expected description 'New Description', got %s", updated.Description)
	}
}
