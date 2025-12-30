package service

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/user/precious-time-tracker/internal/database"
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

func (s *Service) ListTimeEntries(ctx context.Context) ([]database.TimeEntry, error) {
	return s.db.ListTimeEntries(ctx)
}

func (s *Service) GetActiveTimeEntry(ctx context.Context) (database.TimeEntry, error) {
	return s.db.GetActiveTimeEntry(ctx)
}

func (s *Service) GetTimeEntry(ctx context.Context, id int64) (database.TimeEntry, error) {
	return s.db.GetTimeEntry(ctx, id)
}

func (s *Service) ListTags(ctx context.Context) ([]database.Tag, error) {
	return s.db.ListTags(ctx)
}

func (s *Service) StartTimer(ctx context.Context, description string) (*database.TimeEntry, error) {
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

	entry, err := qtx.CreateTimeEntry(ctx, database.CreateTimeEntryParams{
		Description: description,
		StartTime:   time.Now(),
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

	return &entry, nil
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

func (s *Service) UpdateTimeEntry(ctx context.Context, id int64, description string, start time.Time, end sql.NullTime) (*database.TimeEntry, error) {
	tx, err := s.rawDB.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	qtx := s.db.WithTx(tx)

	entry, err := qtx.UpdateTimeEntryFull(ctx, database.UpdateTimeEntryFullParams{
		Description: description,
		StartTime:   start,
		EndTime:     end,
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

	return &entry, nil
}

func (s *Service) DeleteTimeEntry(ctx context.Context, id int64) error {
	if err := s.db.DeleteTimeEntry(ctx, id); err != nil {
		return err
	}
	// Best effort cleanup
	_ = s.db.DeleteOrphanedTags(ctx)
	return nil
}
