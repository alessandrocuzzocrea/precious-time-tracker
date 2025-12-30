package main

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/user/precious-time-tracker/internal/database"
	"github.com/user/precious-time-tracker/internal/server"

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

	root, err := getProjectRoot()
	if err != nil {
		t.Fatalf("failed to find project root: %v", err)
	}

	// Load schema
	schemaPath := filepath.Join(root, "sql/schema/001_users_and_entries.sql")
	schemaBytes, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("failed to read schema from %s: %v", schemaPath, err)
	}
	if _, err := db.Exec(string(schemaBytes)); err != nil {
		t.Fatalf("failed to execute schema: %v", err)
	}

	dbQueries := database.New(db)
	return server.NewServer(dbQueries)
}

func TestHandleIndex(t *testing.T) {
	root, err := getProjectRoot()
	if err != nil {
		t.Fatalf("failed to find project root: %v", err)
	}
	// Temporarily change to root for templates
	oldWd, _ := os.Getwd()
	os.Chdir(root)
	defer os.Chdir(oldWd)

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
	os.Chdir(root)
	defer os.Chdir(oldWd)

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
