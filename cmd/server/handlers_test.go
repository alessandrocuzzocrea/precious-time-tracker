package main

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/user/precious-time-tracker/internal/database"
	"github.com/user/precious-time-tracker/internal/server"

	_ "github.com/mattn/go-sqlite3"
)

func newTestServer(t *testing.T) *server.Server {
	// Setup in-memory DB
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}

	// Load schema
	// Path is relative to cmd/server
	// cmd/server -> ../../sql/schema
	schemaBytes, err := os.ReadFile("../../sql/schema/001_users_and_entries.sql")
	if err != nil {
		t.Fatalf("failed to read schema: %v", err)
	}
	if _, err := db.Exec(string(schemaBytes)); err != nil {
		t.Fatalf("failed to execute schema: %v", err)
	}

	dbQueries := database.New(db)
	return server.NewServer(dbQueries)
}

func TestHandleIndex(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	// Need to fix template path for tests since they run in cmd/server
	// But the handler uses "templates/base.html" which is relative to CWD.
	// If we run test from cmd/server, CWD is cmd/server.
	// But the templates are in ../../templates.
	// This is a common issue.
	// We can change the CWD in the test or use absolute paths in the handler.
	// For this test, let's just cheat and skip template rendering check or
	// assume we run tests from root.
	// Actually, the handler uses `template.ParseFiles("templates/...")`.
	// If we run `go test ./cmd/server`, we are in `cmd/server`.
	// So `templates/` won't be found.
	// We should probably change directory to root for the test.
	os.Chdir("../../")

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
	// os.Chdir("../../") // Already changed in previous test if run sequentially?
	// But to be safe lets ensure we are at root.
	// Note: changing CWD in tests is risky if parallel.
	// For simplicity, we assume sequential or just do it once.
	// Better: use setup function.

	// We need to be in root for template parsing in other handlers if they used it,
	// but StartTimer issues a redirect, so it might not need templates?
	// StartTimer uses DB.

	wd, _ := os.Getwd()
	if !strings.HasSuffix(wd, "precious-time-tracker") {
		os.Chdir("../../")
	}

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
