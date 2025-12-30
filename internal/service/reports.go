package service

import (
	"time"
)

// CalculateReportPeriod returns the start and end times for a given period relative to 'now'.
// end time is inclusive (e.g. 23:59:59).
func CalculateReportPeriod(period string, now time.Time) (time.Time, time.Time) {
	var start, end time.Time

	switch period {
	case "today":
		start = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		end = start.AddDate(0, 0, 1).Add(-time.Second)
	case "week":
		// Assume week starts on Monday
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		start = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -weekday+1)
		end = start.AddDate(0, 0, 7).Add(-time.Second)
	case "month":
		start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		end = start.AddDate(0, 1, 0).Add(-time.Second)
	case "year":
		start = time.Date(now.Year(), 1, 1, 0, 0, 0, 0, now.Location())
		end = start.AddDate(1, 0, 0).Add(-time.Second)
	default: // "all" or anything else
		start = time.Time{}
		end = now.AddDate(100, 0, 0) // Far future
	}

	return start, end
}
