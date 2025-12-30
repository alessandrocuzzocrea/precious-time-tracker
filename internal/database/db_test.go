package database

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/user/precious-time-tracker/sql/schema"
	_ "modernc.org/sqlite"
)

func TestQueries(t *testing.T) {
	// Open in-memory database
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Load and apply schema using Goose
	goose.SetBaseFS(schema.FS)
	if err := goose.SetDialect("sqlite"); err != nil {
		t.Fatalf("failed to set dialect: %v", err)
	}

	if err := goose.Up(db, "."); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	q := New(db)
	ctx := context.Background()

	// Test 1: CreateTimeEntry
	entry, err := q.CreateTimeEntry(ctx, CreateTimeEntryParams{
		Description: "Test Task",
		StartTime:   time.Now(),
	})
	if err != nil {
		t.Fatalf("CreateTimeEntry failed: %v", err)
	}
	if entry.Description != "Test Task" {
		t.Errorf("expected description 'Test Task', got %s", entry.Description)
	}
	if entry.EndTime.Valid {
		t.Errorf("expected end_time to be null, got %v", entry.EndTime)
	}

	// Test 2: GetActiveTimeEntry
	active, err := q.GetActiveTimeEntry(ctx)
	if err != nil {
		t.Fatalf("GetActiveTimeEntry failed: %v", err)
	}
	if active.ID != entry.ID {
		t.Errorf("expected active entry ID %d, got %d", entry.ID, active.ID)
	}

	// Test 3: UpdateTimeEntry (Stop timer)
	now := time.Now()
	stopped, err := q.UpdateTimeEntry(ctx, UpdateTimeEntryParams{
		EndTime: sql.NullTime{Time: now, Valid: true},
		ID:      entry.ID,
	})
	if err != nil {
		t.Fatalf("UpdateTimeEntry failed: %v", err)
	}
	if !stopped.EndTime.Valid {
		t.Error("expected end_time to be valid after update")
	}

	// Test 4: ListTimeEntries
	// Create another one to have list
	_, err = q.CreateTimeEntry(ctx, CreateTimeEntryParams{
		Description: "Task 2",
		StartTime:   time.Now(),
	})
	if err != nil {
		t.Fatalf("Second CreateTimeEntry failed: %v", err)
	}

	list, err := q.ListTimeEntries(ctx)
	if err != nil {
		t.Fatalf("ListTimeEntries failed: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2 entries, got %d", len(list))
	}
}
