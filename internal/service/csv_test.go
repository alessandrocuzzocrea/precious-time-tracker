package service

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestExportCSV(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	// 1. Create some data
	cat, _ := svc.CreateCategory(ctx, "Work", "#ff0000")
	now := time.Now().Truncate(time.Second) // Truncate for CSV roundtrip comparison
	start := now.Add(-1 * time.Hour)
	end := now

	entry, err := svc.StartTimer(ctx, "Test Entry", &cat.ID)
	if err != nil {
		t.Fatalf("failed to create entry: %v", err)
	}
	_, err = svc.UpdateTimeEntry(ctx, entry.ID, "Test Entry", start, sql.NullTime{Time: end, Valid: true}, &cat.ID)
	if err != nil {
		t.Fatalf("failed to update entry: %v", err)
	}

	// 2. Export
	var buf bytes.Buffer
	if err := svc.ExportCSV(ctx, &buf); err != nil {
		t.Fatalf("ExportCSV failed: %v", err)
	}

	// 3. Verify
	reader := csv.NewReader(&buf)
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("failed to read CSV output: %v", err)
	}

	// Expect Header + 1 Row
	if len(records) != 2 {
		t.Errorf("expected 2 records, got %d", len(records))
	}
	header := records[0]
	if header[0] != "id" || header[4] != "category" {
		t.Errorf("unexpected header: %v", header)
	}

	row := records[1]
	if row[1] != "Test Entry" {
		t.Errorf("expected description 'Test Entry', got %s", row[1])
	}
	// Verify times in RFC3339
	if row[2] != start.Format(time.RFC3339) {
		t.Errorf("expected start %s, got %s", start.Format(time.RFC3339), row[2])
	}
}

func TestPreviewCSV(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	// 1. Create existing entry
	entry, _ := svc.StartTimer(ctx, "Existing", nil)
	if err := svc.StopTimer(ctx); err != nil {
		t.Fatalf("StopTimer failed: %v", err)
	} // creates valid end time
	// Refetch to get the updated EndTime
	updated, _ := svc.GetTimeEntry(ctx, entry.ID)
	entry = &updated

	csvContent := `id,description,start_time,end_time,category
,New Entry,2025-01-01T10:00:00Z,,
` + // Row 1: New
		strings.TrimSpace(
			// Row 2: Update (Change Description)
			getCSVRow(t, entry.ID, "Changed Description", entry.StartTime, entry.EndTime.Time, ""),
		) + `
` + // Row 3: No Change (Should be skipped)
		strings.TrimSpace(
			getCSVRow(t, entry.ID, "Existing", entry.StartTime, entry.EndTime.Time, ""),
		)

	// 2. Preview
	preview, err := svc.PreviewCSV(ctx, strings.NewReader(csvContent))
	if err != nil {
		t.Fatalf("PreviewCSV failed: %v", err)
	}

	// 3. Verify
	// 3. Verify
	// Should contain: 1 New, 1 Updated. The "No Change" row should be filtered out.
	if len(preview) != 2 {
		for i, p := range preview {
			t.Logf("Item %d: Status=%s ID=%d Desc=%s (Changed: Desc=%v Start=%v End=%v Cat=%v)",
				i, p.Status, p.ID, p.Description, p.DescriptionChanged, p.StartTimeChanged, p.EndTimeChanged, p.CategoryChanged)
			if p.Status == "Updated" {
				// Debug time comparison
				existing, _ := svc.GetTimeEntry(ctx, p.ID)
				t.Logf("  DB Start: %v (%d) | CSV Start: %v (%d)", existing.StartTime, existing.StartTime.Unix(), p.StartTime, p.StartTime.Unix())
				t.Logf("  DB End:   %v | CSV End:   %v", existing.EndTime.Time, p.EndTime.Time)
			}
		}
		t.Fatalf("expected 2 preview items, got %d", len(preview))
	}

	if preview[0].Status != "New" {
		t.Errorf("expected first item to be New, got %s", preview[0].Status)
	}
	if preview[0].Description != "New Entry" {
		t.Errorf("expected first item description 'New Entry', got %s", preview[0].Description)
	}

	if preview[1].Status != "Updated" {
		t.Errorf("expected second item to be Updated, got %s", preview[1].Status)
	}
	if !preview[1].DescriptionChanged {
		t.Error("expected DescriptionChanged to be true")
	}
}

func TestImportCSV(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	cat, _ := svc.CreateCategory(ctx, "ExistingCat", "#000000")
	entry, _ := svc.StartTimer(ctx, "Old Msg", &cat.ID)
	if err := svc.StopTimer(ctx); err != nil {
		t.Fatalf("StopTimer failed: %v", err)
	} // Ensure valid end time

	// Update the fetched entry to match DB state for precise time formatting
	entryFromDB, _ := svc.GetTimeEntry(ctx, entry.ID)

	// CSV contains:
	// 1. New Entry with New Category
	// 2. Update Existing Entry (Description + Time)
	// We use Truncate(time.Second) to match the RFC3339 precision used in CSV export/import
	newStart := entryFromDB.StartTime.Add(time.Hour).Truncate(time.Second)
	newEnd := entryFromDB.EndTime.Time.Add(time.Hour).Truncate(time.Second)

	csvContent := `id,description,start_time,end_time,category
,Brand New,2025-01-01T12:00:00Z,2025-01-01T13:00:00Z,NewCat
` + strings.TrimSpace(
		getCSVRow(t, entryFromDB.ID, "Updated Msg", newStart, newEnd, "ExistingCat"),
	)

	// 2. Import
	if err := svc.ImportCSV(ctx, strings.NewReader(csvContent)); err != nil {
		t.Fatalf("ImportCSV failed: %v", err)
	}

	// 3. Verify
	entries, _ := svc.ListTimeEntries(ctx)
	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}

	// Check New Entry
	newEntry := entries[0] // Sorted by start time DESC usually, so let's find by description
	if newEntry.Description != "Brand New" {
		newEntry = entries[1]
	}
	if newEntry.Description != "Brand New" {
		t.Error("New entry not found")
	}
	if newEntry.CategoryName.String != "NewCat" {
		t.Errorf("expected category NewCat, got %s", newEntry.CategoryName.String)
	}

	// Check Updated Entry
	updatedEntry, _ := svc.GetTimeEntry(ctx, entry.ID)
	if updatedEntry.Description != "Updated Msg" {
		t.Errorf("expected description 'Updated Msg', got %s", updatedEntry.Description)
	}
	if !updatedEntry.StartTime.Equal(newStart) {
		t.Errorf("StartTime was not updated. Expected %v, got %v", newStart, updatedEntry.StartTime)
	}
}

// Helper to construct a CSV row string
func getCSVRow(t *testing.T, id int64, desc string, start, end time.Time, cat string) string {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	err := w.Write([]string{
		fmt.Sprintf("%d", id),
		desc,
		start.Format(time.RFC3339),
		end.Format(time.RFC3339),
		cat,
	})
	if err != nil {
		t.Fatalf("failed to write csv row: %v", err)
	}
	w.Flush()
	return strings.TrimSpace(buf.String())
}
