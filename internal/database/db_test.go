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

func TestDeleteCascade(t *testing.T) {
	// Enable foreign keys! Important for CASCADE to work in SQLite
	// Without this, the test will correctly fail (orphans remain)
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer func() { _ = db.Close() }()

	if _, err := db.Exec("PRAGMA foreign_keys = ON;"); err != nil {
		t.Fatalf("failed to enable foreign keys: %v", err)
	}

	goose.SetBaseFS(schema.FS)
	if err := goose.SetDialect("sqlite"); err != nil {
		t.Fatalf("failed to set dialect: %v", err)
	}
	if err := goose.Up(db, "."); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	q := New(db)
	ctx := context.Background()

	// 1. Create Entry
	entry, err := q.CreateTimeEntry(ctx, CreateTimeEntryParams{
		Description: "Cascade Test",
		StartTime:   time.Now(),
	})
	if err != nil {
		t.Fatalf("CreateTimeEntry failed: %v", err)
	}

	// 2. Create Tag
	tag, err := q.CreateTag(ctx, "deletethis")
	if err != nil {
		t.Fatalf("CreateTag failed: %v", err)
	}

	// 3. Link them
	err = q.CreateTimeEntryTag(ctx, CreateTimeEntryTagParams{
		TimeEntryID: entry.ID,
		TagID:       tag.ID,
	})
	if err != nil {
		t.Fatalf("CreateTimeEntryTag failed: %v", err)
	}

	// Verify link exists
	tags, err := q.ListTagsForTimeEntry(ctx, entry.ID)
	if err != nil {
		t.Fatalf("ListTagsForTimeEntry failed: %v", err)
	}
	if len(tags) != 1 {
		t.Fatalf("Expected 1 tag, got %d", len(tags))
	}

	// 4. Delete Entry
	if err := q.DeleteTimeEntry(ctx, entry.ID); err != nil {
		t.Fatalf("DeleteTimeEntry failed: %v", err)
	}

	// 5. Verify Link is gone
	// We need a query to check directly OR check via ListTagsForTimeEntry (which might return empty if join fails, which is correct behavior for the app but we want to verify the row is GONE from time_entry_tags)
	// Since ListTagsForTimeEntry joins on time_entry_tags, if the link is gone, it returns 0.
	// But if the link is there but entry is gone... wait.
	// The Join is: SELECT t.* FROM tags t JOIN time_entry_tags tet ON t.id = tet.tag_id WHERE tet.time_entry_id = ?
	// If I query for the deleted entry ID, and the row in tet still exists, it will return the tag.
	tagsAfter, err := q.ListTagsForTimeEntry(ctx, entry.ID)
	if err != nil {
		t.Fatalf("ListTagsForTimeEntry after delete failed: %v", err)
	}
	if len(tagsAfter) != 0 {
		t.Errorf("Cascade failed! Expected 0 tags after entry delete, got %d. Foreign keys might not be enabled.", len(tagsAfter))
	}
}
