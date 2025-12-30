package service

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/user/precious-time-tracker/internal/database"
	"github.com/user/precious-time-tracker/sql/schema"

	_ "modernc.org/sqlite"
)

func newTestService(t *testing.T) *Service {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}

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

	dbQueries := database.New(db)
	return New(dbQueries, db)
}

func TestStartAndStopTimer(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	// 1. Start timer
	entry, err := svc.StartTimer(ctx, "Test Task #tag1", nil)
	if err != nil {
		t.Fatalf("StartTimer failed: %v", err)
	}
	if entry.Description != "Test Task #tag1" {
		t.Errorf("expected description 'Test Task #tag1', got %s", entry.Description)
	}
	if entry.EndTime.Valid {
		t.Errorf("expected EndTime to be invalid for running timer")
	}

	// Verify tags
	tags, err := svc.ListTags(ctx)
	if err != nil {
		t.Fatalf("ListTags failed: %v", err)
	}
	if len(tags) != 1 || tags[0].Name != "tag1" {
		t.Errorf("expected 1 tag 'tag1', got %v", tags)
	}

	// 2. Start another timer (should stop the first one)
	entry2, err := svc.StartTimer(ctx, "Second Task", nil)
	if err != nil {
		t.Fatalf("StartTimer 2 failed: %v", err)
	}

	// Verify first one is stopped
	oldEntry, err := svc.GetTimeEntry(ctx, entry.ID)
	if err != nil {
		t.Fatalf("GetTimeEntry failed: %v", err)
	}
	if !oldEntry.EndTime.Valid {
		t.Errorf("expected first entry to be stopped")
	}

	// 3. Stop running timer
	err = svc.StopTimer(ctx)
	if err != nil {
		t.Fatalf("StopTimer failed: %v", err)
	}

	stoppedEntry, err := svc.GetTimeEntry(ctx, entry2.ID)
	if err != nil {
		t.Fatalf("GetTimeEntry 2 failed: %v", err)
	}
	if !stoppedEntry.EndTime.Valid {
		t.Errorf("expected second entry to be stopped")
	}
}

func TestUpdateTimeEntry(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	entry, _ := svc.StartTimer(ctx, "Initial #old", nil)

	newStartTime := entry.StartTime.Add(-1 * time.Hour)
	newEndTime := sql.NullTime{Time: entry.StartTime.Add(1 * time.Hour), Valid: true}

	updated, err := svc.UpdateTimeEntry(ctx, entry.ID, "Updated #new", newStartTime, newEndTime, nil)
	if err != nil {
		t.Fatalf("UpdateTimeEntry failed: %v", err)
	}

	if updated.Description != "Updated #new" {
		t.Errorf("expected description 'Updated #new', got %s", updated.Description)
	}
	if !updated.StartTime.Equal(newStartTime) {
		t.Errorf("expected start time %v, got %v", newStartTime, updated.StartTime)
	}

	// Verify tags updated
	tags, _ := svc.ListTags(ctx)
	foundOld := false
	foundNew := false
	for _, tag := range tags {
		if tag.Name == "old" {
			foundOld = true
		}
		if tag.Name == "new" {
			foundNew = true
		}
	}
	if foundOld {
		t.Errorf("expected tag 'old' to be removed")
	}
	if !foundNew {
		t.Errorf("expected tag 'new' to be present")
	}
}

func TestDeleteTimeEntry(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	entry, _ := svc.StartTimer(ctx, "To Delete #tag", nil)

	err := svc.DeleteTimeEntry(ctx, entry.ID)
	if err != nil {
		t.Fatalf("DeleteTimeEntry failed: %v", err)
	}

	_, err = svc.GetTimeEntry(ctx, entry.ID)
	if err == nil {
		t.Errorf("expected entry to be deleted")
	}

	// Verify tag is cleaned up if orphaned
	tags, _ := svc.ListTags(ctx)
	if len(tags) != 0 {
		t.Errorf("expected tags to be cleaned up, got %v", tags)
	}
}

func TestCategoryCRUD(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	// Create
	cat, err := svc.CreateCategory(ctx, "Work", "#ff0000")
	if err != nil {
		t.Fatalf("CreateCategory failed: %v", err)
	}
	if cat.Name != "Work" || cat.Color != "#ff0000" {
		t.Errorf("expected Work/#ff0000, got %s/%s", cat.Name, cat.Color)
	}

	// List
	cats, err := svc.ListCategories(ctx)
	if err != nil {
		t.Fatalf("ListCategories failed: %v", err)
	}
	if len(cats) != 1 || cats[0].Name != "Work" {
		t.Errorf("expected 1 category 'Work', got %v", cats)
	}

	// Update
	updated, err := svc.UpdateCategory(ctx, cat.ID, "Personal", "#00ff00")
	if err != nil {
		t.Fatalf("UpdateCategory failed: %v", err)
	}
	if updated.Name != "Personal" || updated.Color != "#00ff00" {
		t.Errorf("expected Personal/#00ff00, got %s/%s", updated.Name, updated.Color)
	}

	// Delete
	err = svc.DeleteCategory(ctx, cat.ID)
	if err != nil {
		t.Fatalf("DeleteCategory failed: %v", err)
	}
	cats, _ = svc.ListCategories(ctx)
	if len(cats) != 0 {
		t.Errorf("expected 0 categories, got %v", cats)
	}
}

func TestTimeEntryWithCategory(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	cat, _ := svc.CreateCategory(ctx, "Work", "#ff0000")

	// Start with category
	entry, err := svc.StartTimer(ctx, "Working hard", &cat.ID)
	if err != nil {
		t.Fatalf("StartTimer with category failed: %v", err)
	}
	if !entry.CategoryID.Valid || entry.CategoryID.Int64 != cat.ID {
		t.Errorf("expected category ID %d, got %v", cat.ID, entry.CategoryID)
	}

	// Check List
	entries, _ := svc.ListTimeEntries(ctx)
	if len(entries) == 0 || entries[0].CategoryName.String != "Work" {
		t.Errorf("expected category name 'Work' in list, got %v", entries[0].CategoryName)
	}

	// Update category
	cat2, _ := svc.CreateCategory(ctx, "Personal", "#00ff00")
	updated, err := svc.UpdateTimeEntry(ctx, entry.ID, entry.Description, entry.StartTime, entry.EndTime, &cat2.ID)
	if err != nil {
		t.Fatalf("UpdateTimeEntry with category failed: %v", err)
	}
	if updated.CategoryID.Int64 != cat2.ID {
		t.Errorf("expected category ID %d, got %v", cat2.ID, updated.CategoryID)
	}
}

func TestParseTags(t *testing.T) {
	tests := []struct {
		desc     string
		input    string
		expected []string
	}{
		{"no tags", "hello world", nil},
		{"one tag", "hello #world", []string{"world"}},
		{"multiple tags", "#a #b #c", []string{"a", "b", "c"}},
		{"case insensitive", "#Tag #tag", []string{"tag"}},
		{"special characters", "#tag_123 #not-a-tag", []string{"tag_123", "not"}},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			got := parseTags(tt.input)
			if len(got) != len(tt.expected) {
				t.Errorf("expected %v, got %v", tt.expected, got)
				return
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("expected %v, got %v", tt.expected, got)
				}
			}
		})
	}
}
