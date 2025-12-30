package service

import (
	"testing"
	"time"
)

func TestCalculateReportPeriod(t *testing.T) {
	// Fixed "now": Monday, Jan 15, 2024 (Leap year)
	now := time.Date(2024, time.January, 15, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name          string
		period        string
		expectedStart string
		expectedEnd   string
	}{
		{
			"Today",
			"today",
			"2024-01-15T00:00:00Z",
			"2024-01-15T23:59:59Z",
		},
		{
			"This Week (Monday start)",
			"week",
			"2024-01-15T00:00:00Z",
			"2024-01-21T23:59:59Z",
		},
		{
			"This Month",
			"month",
			"2024-01-01T00:00:00Z",
			"2024-01-31T23:59:59Z",
		},
		{
			"This Year",
			"year",
			"2024-01-01T00:00:00Z",
			"2024-12-31T23:59:59Z",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end := CalculateReportPeriod(tt.period, now)
			if start.Format(time.RFC3339) != tt.expectedStart {
				t.Errorf("expected start %s, got %s", tt.expectedStart, start.Format(time.RFC3339))
			}
			if end.Format(time.RFC3339) != tt.expectedEnd {
				t.Errorf("expected end %s, got %s", tt.expectedEnd, end.Format(time.RFC3339))
			}
		})
	}
}

func TestWeekBoundarySunday(t *testing.T) {
	// Sunday Jan 14, 2024
	now := time.Date(2024, time.January, 14, 12, 0, 0, 0, time.UTC)
	// Should be part of week Jan 8 - Jan 14

	start, end := CalculateReportPeriod("week", now)
	expectedStart := "2024-01-08T00:00:00Z"
	expectedEnd := "2024-01-14T23:59:59Z"

	if start.Format(time.RFC3339) != expectedStart {
		t.Errorf("expected start %s, got %s", expectedStart, start.Format(time.RFC3339))
	}
	if end.Format(time.RFC3339) != expectedEnd {
		t.Errorf("expected end %s, got %s", expectedEnd, end.Format(time.RFC3339))
	}
}
