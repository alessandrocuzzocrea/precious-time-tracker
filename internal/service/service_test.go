package service

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/alessandrocuzzocrea/precious-time-tracker/internal/database"
	"github.com/alessandrocuzzocrea/precious-time-tracker/sql/schema"
	"github.com/pressly/goose/v3"

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

	// Stop it so it appears in ListTimeEntries
	err = svc.StopTimer(ctx)
	if err != nil {
		t.Fatalf("StopTimer failed: %v", err)
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

func TestGetReport(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	var err error

	cat1, _ := svc.CreateCategory(ctx, "Work", "#ff0000")
	cat2, _ := svc.CreateCategory(ctx, "Personal", "#00ff00")

	now := time.Now()
	// Entry 1: Work, today, with tag1
	e1, _ := svc.StartTimer(ctx, "Work #tag1", &cat1.ID)
	_, err = svc.UpdateTimeEntry(ctx, e1.ID, e1.Description, now.Add(-2*time.Hour), sql.NullTime{Time: now.Add(-1 * time.Hour), Valid: true}, &cat1.ID)
	if err != nil {
		t.Fatalf("failed to update e1: %v", err)
	}

	// Entry 2: Personal, today, with tag1 and tag2
	e2, _ := svc.StartTimer(ctx, "Personal #tag1 #tag2", &cat2.ID)
	_, err = svc.UpdateTimeEntry(ctx, e2.ID, e2.Description, now.Add(-30*time.Minute), sql.NullTime{Time: now, Valid: true}, &cat2.ID)
	if err != nil {
		t.Fatalf("failed to update e2: %v", err)
	}

	// Entry 3: No category, today, with tag2
	e3, _ := svc.StartTimer(ctx, "Uncategorized #tag2", nil)
	_, err = svc.UpdateTimeEntry(ctx, e3.ID, e3.Description, now.Add(-15*time.Minute), sql.NullTime{Time: now.Add(-5 * time.Minute), Valid: true}, nil)
	if err != nil {
		t.Fatalf("failed to update e3: %v", err)
	}

	// Entry 4: Yesterday (different period)
	yesterday := now.AddDate(0, 0, -1)
	e4, _ := svc.StartTimer(ctx, "Yesterday", &cat1.ID)
	_, err = svc.UpdateTimeEntry(ctx, e4.ID, e4.Description, yesterday, sql.NullTime{Time: yesterday.Add(time.Hour), Valid: true}, &cat1.ID)
	if err != nil {
		t.Fatalf("failed to update e4: %v", err)
	}

	startToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	endToday := startToday.AddDate(0, 0, 1).Add(-time.Second)

	// 1. All Categories, Today, No Tags
	report, err := svc.GetReport(ctx, ReportFilter{
		StartDate:      startToday,
		EndDate:        endToday,
		CategoryFilter: 0,
	})
	if err != nil {
		t.Fatalf("GetReport failed: %v", err)
	}
	if len(report.Entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(report.Entries))
	}
	expectedTotal := int64((1 * time.Hour).Seconds() + (30 * time.Minute).Seconds() + (10 * time.Minute).Seconds())
	if report.TotalSeconds != expectedTotal {
		t.Errorf("expected total %ds, got %ds", expectedTotal, report.TotalSeconds)
	}

	// 2. Filter by Category 1 (Work)
	report, _ = svc.GetReport(ctx, ReportFilter{
		StartDate:      startToday,
		EndDate:        endToday,
		CategoryFilter: cat1.ID,
	})
	if len(report.Entries) != 1 || report.Entries[0].ID != e1.ID {
		t.Errorf("expected entry e1, got %v", report.Entries)
	}

	// 3. Filter by "No Category"
	report, _ = svc.GetReport(ctx, ReportFilter{
		StartDate:      startToday,
		EndDate:        endToday,
		CategoryFilter: -1,
	})
	if len(report.Entries) != 1 || report.Entries[0].ID != e3.ID {
		t.Errorf("expected entry e3, got %v", report.Entries)
	}

	// 4. Filter by Multiple Tags (AND)
	tags, _ := svc.ListTags(ctx)
	var tag1ID, tag2ID int64
	for _, tg := range tags {
		if tg.Name == "tag1" {
			tag1ID = tg.ID
		}
		if tg.Name == "tag2" {
			tag2ID = tg.ID
		}
	}

	report, _ = svc.GetReport(ctx, ReportFilter{
		StartDate:      startToday,
		EndDate:        endToday,
		CategoryFilter: 0,
		TagIDs:         []int64{tag1ID, tag2ID},
	})
	if len(report.Entries) != 1 || report.Entries[0].ID != e2.ID {
		t.Errorf("expected entry e2, got %v", report.Entries)
	}

	// 5. Verify breakdown
	foundNoCategory := false
	for _, b := range report.CategoryBreakdown {
		if b.CategoryID == -1 {
			foundNoCategory = true
		}
	}
	// Note: In this specific filter (tag1 AND tag2), e3 is NOT present, so foundNoCategory remains false.
	// We check it here to avoid ineffassign before re-assigning it below.
	if len(report.Entries) == 1 && foundNoCategory {
		t.Errorf("No Category should not be in breakdown for this specific filter")
	}

	// In the tags filter above, e2 is the only one, so breakdown should have Personal (100%)
	// Let's check a report without tag filter for breakdown
	report, _ = svc.GetReport(ctx, ReportFilter{
		StartDate:      startToday,
		EndDate:        endToday,
		CategoryFilter: 0,
	})
	foundNoCategory = false
	for _, b := range report.CategoryBreakdown {
		if b.CategoryID == -1 {
			foundNoCategory = true
			if b.TotalSeconds != 600 { // 10 minutes
				t.Errorf("expected 600s for No Category, got %d", b.TotalSeconds)
			}
		}
	}
	if !foundNoCategory {
		t.Errorf("No Category not found in breakdown")
	}
}
