package service

import (
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/alessandrocuzzocrea/precious-time-tracker/internal/database"
)

type Service struct {
	db    *database.Queries
	rawDB *sql.DB
}

func New(db *database.Queries, rawDB *sql.DB) *Service {
	return &Service{
		db:    db,
		rawDB: rawDB,
	}
}

var tagRegex = regexp.MustCompile(`#([a-zA-Z0-9_]+)`)

func parseTags(description string) []string {
	matches := tagRegex.FindAllStringSubmatch(description, -1)
	var tags []string
	seen := make(map[string]bool)
	for _, match := range matches {
		if len(match) > 1 {
			tag := strings.ToLower(match[1])
			if !seen[tag] {
				tags = append(tags, tag)
				seen[tag] = true
			}
		}
	}
	return tags
}

func (s *Service) updateTags(ctx context.Context, qxt *database.Queries, entryID int64, tags []string) error {
	// First clear existing tags for this entry
	if err := qxt.DeleteTimeEntryTags(ctx, entryID); err != nil {
		return err
	}

	for _, tagName := range tags {
		// Create tag if not exists or get existing
		tag, err := qxt.CreateTag(ctx, tagName)
		if err != nil {
			return err
		}

		// Link tag to entry
		if err := qxt.CreateTimeEntryTag(ctx, database.CreateTimeEntryTagParams{
			TimeEntryID: entryID,
			TagID:       tag.ID,
		}); err != nil {
			return err
		}
	}

	// Clean up any orphaned tags
	if err := qxt.DeleteOrphanedTags(ctx); err != nil {
		return err
	}
	return nil
}

func (s *Service) ListTimeEntries(ctx context.Context) ([]database.ListTimeEntriesRow, error) {
	return s.db.ListTimeEntries(ctx)
}

func (s *Service) GetActiveTimeEntry(ctx context.Context) (database.GetActiveTimeEntryRow, error) {
	return s.db.GetActiveTimeEntry(ctx)
}

func (s *Service) GetTimeEntry(ctx context.Context, id int64) (database.GetTimeEntryRow, error) {
	return s.db.GetTimeEntry(ctx, id)
}

func (s *Service) ListTags(ctx context.Context) ([]database.Tag, error) {
	return s.db.ListTags(ctx)
}

func (s *Service) ListCategories(ctx context.Context) ([]database.Category, error) {
	return s.db.ListCategories(ctx)
}

func (s *Service) CreateCategory(ctx context.Context, name, color string) (database.Category, error) {
	return s.db.CreateCategory(ctx, database.CreateCategoryParams{
		Name:  name,
		Color: color,
	})
}

func (s *Service) UpdateCategory(ctx context.Context, id int64, name, color string) (database.Category, error) {
	return s.db.UpdateCategory(ctx, database.UpdateCategoryParams{
		ID:    id,
		Name:  name,
		Color: color,
	})
}

func (s *Service) DeleteCategory(ctx context.Context, id int64) error {
	return s.db.DeleteCategory(ctx, id)
}

func (s *Service) GetCategory(ctx context.Context, id int64) (database.Category, error) {
	return s.db.GetCategory(ctx, id)
}

func (s *Service) StartTimer(ctx context.Context, description string, categoryID *int64) (*database.GetTimeEntryRow, error) {
	if description == "" {
		description = "No description"
	}

	tx, err := s.rawDB.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	qtx := s.db.WithTx(tx)

	// Stop any currently active timer
	active, err := qtx.GetActiveTimeEntry(ctx)
	if err == nil {
		if _, err := qtx.UpdateTimeEntry(ctx, database.UpdateTimeEntryParams{
			EndTime: sql.NullTime{Time: time.Now(), Valid: true},
			ID:      active.ID,
		}); err != nil {
			log.Printf("Failed to stop previous active timer (ID %d): %v", active.ID, err)
		}
	}

	var catID sql.NullInt64
	if categoryID != nil {
		catID = sql.NullInt64{Int64: *categoryID, Valid: true}
	}

	entry, err := qtx.CreateTimeEntry(ctx, database.CreateTimeEntryParams{
		Description: description,
		StartTime:   time.Now(),
		CategoryID:  catID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create entry: %w", err)
	}

	tags := parseTags(description)
	if err := s.updateTags(ctx, qtx, entry.ID, tags); err != nil {
		return nil, fmt.Errorf("failed to update tags: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Fetch the full entry with category info
	fullEntry, err := s.db.GetTimeEntry(ctx, entry.ID)
	return &fullEntry, err
}

func (s *Service) StopTimer(ctx context.Context) error {
	active, err := s.db.GetActiveTimeEntry(ctx)
	if err != nil {
		return nil // Nothing to stop
	}

	_, err = s.db.UpdateTimeEntry(ctx, database.UpdateTimeEntryParams{
		EndTime: sql.NullTime{Time: time.Now(), Valid: true},
		ID:      active.ID,
	})
	return err
}

func (s *Service) UpdateTimeEntry(ctx context.Context, id int64, description string, start time.Time, end sql.NullTime, categoryID *int64) (*database.GetTimeEntryRow, error) {
	tx, err := s.rawDB.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	qtx := s.db.WithTx(tx)

	var catID sql.NullInt64
	if categoryID != nil {
		catID = sql.NullInt64{Int64: *categoryID, Valid: true}
	}

	entry, err := qtx.UpdateTimeEntryFull(ctx, database.UpdateTimeEntryFullParams{
		Description: description,
		StartTime:   start,
		EndTime:     end,
		CategoryID:  catID,
		ID:          id,
	})
	if err != nil {
		return nil, err
	}

	tags := parseTags(description)
	if err := s.updateTags(ctx, qtx, entry.ID, tags); err != nil {
		return nil, fmt.Errorf("failed to update tags: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	fullEntry, err := s.db.GetTimeEntry(ctx, id)
	return &fullEntry, err
}

func (s *Service) DeleteTimeEntry(ctx context.Context, id int64) error {
	if err := s.db.DeleteTimeEntry(ctx, id); err != nil {
		return err
	}
	// Best effort cleanup
	_ = s.db.DeleteOrphanedTags(ctx)
	return nil
}

type ReportFilter struct {
	StartDate      time.Time
	EndDate        time.Time
	CategoryFilter int64   // 0: All, -1: No Category, >0: Specific Category
	TagIDs         []int64 // AND filter
}

type CategoryBreakdown struct {
	CategoryID   int64
	CategoryName string
	Color        string
	TotalSeconds int64
	Percentage   float64
}

type ReportData struct {
	Entries           []database.ListTimeEntriesReportRow
	TotalSeconds      int64
	CategoryBreakdown []CategoryBreakdown
	Filter            ReportFilter
}

type CSVPreviewEntry struct {
	ID          int64
	Description string
	StartTime   time.Time
	EndTime     sql.NullTime
	Category    string
	Status      string // "New" or "Updated"
}

func (s *Service) GetReport(ctx context.Context, filter ReportFilter) (ReportData, error) {
	rows, err := s.db.ListTimeEntriesReport(ctx, database.ListTimeEntriesReportParams{
		StartTime:      filter.StartDate,
		StartTime_2:    filter.EndDate,
		CategoryFilter: filter.CategoryFilter,
	})
	if err != nil {
		return ReportData{}, err
	}

	var filteredRows []database.ListTimeEntriesReportRow
	categoryTotals := make(map[int64]*CategoryBreakdown)
	var totalSeconds int64

	// Initialize "No Category" breakdown
	noCategory := &CategoryBreakdown{
		CategoryID:   -1,
		CategoryName: "No Category",
		Color:        "#888888",
	}

	for _, row := range rows {
		// Filter by tags (AND logic)
		if len(filter.TagIDs) > 0 {
			entryTags, err := s.db.ListTagsForTimeEntry(ctx, row.ID)
			if err != nil {
				continue
			}
			tagMap := make(map[int64]bool)
			for _, t := range entryTags {
				tagMap[t.ID] = true
			}
			matchAll := true
			for _, id := range filter.TagIDs {
				if !tagMap[id] {
					matchAll = false
					break
				}
			}
			if !matchAll {
				continue
			}
		}

		duration := row.EndTime.Time.Sub(row.StartTime)
		seconds := int64(duration.Seconds())
		totalSeconds += seconds

		if row.CategoryID.Valid {
			catID := row.CategoryID.Int64
			if _, ok := categoryTotals[catID]; !ok {
				categoryTotals[catID] = &CategoryBreakdown{
					CategoryID:   catID,
					CategoryName: row.CategoryName.String,
					Color:        row.CategoryColor.String,
				}
			}
			categoryTotals[catID].TotalSeconds += seconds
		} else {
			noCategory.TotalSeconds += seconds
		}

		filteredRows = append(filteredRows, row)
	}

	var breakdown []CategoryBreakdown
	if totalSeconds > 0 {
		for _, b := range categoryTotals {
			b.Percentage = (float64(b.TotalSeconds) / float64(totalSeconds)) * 100
			breakdown = append(breakdown, *b)
		}
		if noCategory.TotalSeconds > 0 {
			noCategory.Percentage = (float64(noCategory.TotalSeconds) / float64(totalSeconds)) * 100
			breakdown = append(breakdown, *noCategory)
		}
	} else if noCategory.TotalSeconds > 0 || len(categoryTotals) > 0 {
		// This case shouldn't really happen if totalSeconds is 0, but for completeness
		for _, b := range categoryTotals {
			breakdown = append(breakdown, *b)
		}
		breakdown = append(breakdown, *noCategory)
	}

	return ReportData{
		Entries:           filteredRows,
		TotalSeconds:      totalSeconds,
		CategoryBreakdown: breakdown,
		Filter:            filter,
	}, nil
}

func (s *Service) ExportCSV(ctx context.Context, w io.Writer) error {
	entries, err := s.db.ListAllTimeEntries(ctx)
	if err != nil {
		return err
	}

	writer := csv.NewWriter(w)
	defer writer.Flush()

	// Header
	if err := writer.Write([]string{"id", "description", "start_time", "end_time", "category"}); err != nil {
		return err
	}

	for _, e := range entries {
		startTime := e.StartTime.Format(time.RFC3339)
		endTime := ""
		if e.EndTime.Valid {
			endTime = e.EndTime.Time.Format(time.RFC3339)
		}
		category := ""
		if e.CategoryName.Valid {
			category = e.CategoryName.String
		}

		if err := writer.Write([]string{
			strconv.FormatInt(e.ID, 10),
			e.Description,
			startTime,
			endTime,
			category,
		}); err != nil {
			return err
		}
	}

	return nil
}

func (s *Service) ImportCSV(ctx context.Context, r io.Reader) error {
	reader := csv.NewReader(r)
	records, err := reader.ReadAll()
	if err != nil {
		return err
	}

	if len(records) < 2 {
		return nil // Only header or empty
	}

	header := records[0]
	colMap := make(map[string]int)
	for i, h := range header {
		colMap[strings.ToLower(strings.TrimSpace(h))] = i
	}

	tx, err := s.rawDB.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	qtx := s.db.WithTx(tx)

	for _, record := range records[1:] {
		// Helper to get col value
		getVal := func(name string) string {
			if idx, ok := colMap[name]; ok && idx < len(record) {
				return strings.TrimSpace(record[idx])
			}
			return ""
		}

		idStr := getVal("id")
		description := getVal("description")
		startTimeStr := getVal("start_time")
		endTimeStr := getVal("end_time")
		categoryName := getVal("category")

		if description == "" && startTimeStr == "" {
			continue // Skip empty rows
		}

		startTime, err := parseFlexTime(startTimeStr)
		if err != nil {
			return fmt.Errorf("invalid start_time '%s': %w", startTimeStr, err)
		}

		var endTime sql.NullTime
		if endTimeStr != "" {
			et, err := parseFlexTime(endTimeStr)
			if err != nil {
				return fmt.Errorf("invalid end_time '%s': %w", endTimeStr, err)
			}
			endTime = sql.NullTime{Time: et, Valid: true}
		}

		var catID sql.NullInt64
		if categoryName != "" {
			cat, err := qtx.GetCategoryByName(ctx, categoryName)
			if err == sql.ErrNoRows {
				// Create category
				cat, err = qtx.CreateCategory(ctx, database.CreateCategoryParams{
					Name:  categoryName,
					Color: "#cccccc",
				})
				if err != nil {
					return fmt.Errorf("failed to create category '%s': %w", categoryName, err)
				}
			} else if err != nil {
				return err
			}
			catID = sql.NullInt64{Int64: cat.ID, Valid: true}
		}

		var entry database.TimeEntry
		id, _ := strconv.ParseInt(idStr, 10, 64)
		if id > 0 {
			entry, err = qtx.UpsertTimeEntry(ctx, database.UpsertTimeEntryParams{
				ID:          id,
				Description: description,
				StartTime:   startTime,
				EndTime:     endTime,
				CategoryID:  catID,
			})
		} else {
			entry, err = qtx.CreateTimeEntryFull(ctx, database.CreateTimeEntryFullParams{
				Description: description,
				StartTime:   startTime,
				EndTime:     endTime,
				CategoryID:  catID,
			})
		}

		if err != nil {
			return fmt.Errorf("failed to save entry: %w", err)
		}

		// Update tags
		tags := parseTags(description)
		if err := s.updateTags(ctx, qtx, entry.ID, tags); err != nil {
			return fmt.Errorf("failed to update tags for entry %d: %w", entry.ID, err)
		}
	}

	return tx.Commit()
}

func (s *Service) PreviewCSV(ctx context.Context, r io.Reader) ([]CSVPreviewEntry, error) {
	reader := csv.NewReader(r)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}

	if len(records) < 2 {
		return nil, nil
	}

	header := records[0]
	colMap := make(map[string]int)
	for i, h := range header {
		colMap[strings.ToLower(strings.TrimSpace(h))] = i
	}

	var preview []CSVPreviewEntry

	for _, record := range records[1:] {
		getVal := func(name string) string {
			if idx, ok := colMap[name]; ok && idx < len(record) {
				return strings.TrimSpace(record[idx])
			}
			return ""
		}

		idStr := getVal("id")
		description := getVal("description")
		startTimeStr := getVal("start_time")
		endTimeStr := getVal("end_time")
		categoryName := getVal("category")

		if description == "" && startTimeStr == "" {
			continue
		}

		startTime, err := parseFlexTime(startTimeStr)
		if err != nil {
			continue // Skip invalid rows for preview or handle error
		}

		var endTime sql.NullTime
		if endTimeStr != "" {
			et, err := parseFlexTime(endTimeStr)
			if err == nil {
				endTime = sql.NullTime{Time: et, Valid: true}
			}
		}

		id, _ := strconv.ParseInt(idStr, 10, 64)
		status := "New"
		if id > 0 {
			existing, err := s.db.GetTimeEntry(ctx, id)
			if err == nil {
				// Compare all fields to see if anything actually changed
				descMatch := existing.Description == description
				startMatch := existing.StartTime.Equal(startTime)
				endMatch := (existing.EndTime.Valid == endTime.Valid)
				if endMatch && existing.EndTime.Valid {
					endMatch = existing.EndTime.Time.Equal(endTime.Time)
				}
				catMatch := (existing.CategoryName.Valid && existing.CategoryName.String == categoryName) ||
					(!existing.CategoryName.Valid && categoryName == "")

				if descMatch && startMatch && endMatch && catMatch {
					continue // No changes, skip from preview
				}
				status = "Updated"
			}
		}

		preview = append(preview, CSVPreviewEntry{
			ID:          id,
			Description: description,
			StartTime:   startTime,
			EndTime:     endTime,
			Category:    categoryName,
			Status:      status,
		})
	}

	return preview, nil
}

func parseFlexTime(s string) (time.Time, error) {
	formats := []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04",
		"2006-01-02",
	}
	for _, f := range formats {
		t, err := time.Parse(f, s)
		if err == nil {
			return t, nil
		}
		// Try with local location
		t, err = time.ParseInLocation(f, s, time.Local)
		if err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported time format")
}
