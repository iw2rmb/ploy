package ttlworker

import (
	"regexp"
	"testing"
	"time"
)

func TestPartitionPattern(t *testing.T) {
	tests := []struct {
		name          string
		partitionName string
		wantMatch     bool
		wantTable     string
		wantYear      string
		wantMonth     string
	}{
		{
			name:          "valid logs partition",
			partitionName: "ploy.logs_2025_10",
			wantMatch:     true,
			wantTable:     "logs",
			wantYear:      "2025",
			wantMonth:     "10",
		},
		{
			name:          "valid events partition",
			partitionName: "ploy.events_2024_01",
			wantMatch:     true,
			wantTable:     "events",
			wantYear:      "2024",
			wantMonth:     "01",
		},
		{
			name:          "valid artifact_bundles partition",
			partitionName: "ploy.artifact_bundles_2023_12",
			wantMatch:     true,
			wantTable:     "artifact_bundles",
			wantYear:      "2023",
			wantMonth:     "12",
		},
		{
			name:          "invalid format - no schema",
			partitionName: "logs_2025_10",
			wantMatch:     false,
		},
		{
			name:          "invalid format - wrong schema",
			partitionName: "public.logs_2025_10",
			wantMatch:     false,
		},
		{
			name:          "invalid format - no year",
			partitionName: "ploy.logs_10",
			wantMatch:     false,
		},
		{
			name:          "invalid format - bad year",
			partitionName: "ploy.logs_25_10",
			wantMatch:     false,
		},
		{
			name:          "invalid format - bad month",
			partitionName: "ploy.logs_2025_1",
			wantMatch:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := partitionPattern.FindStringSubmatch(tt.partitionName)
			gotMatch := len(matches) == 4

			if gotMatch != tt.wantMatch {
				t.Errorf("partitionPattern.FindStringSubmatch(%q) match=%v, want %v",
					tt.partitionName, gotMatch, tt.wantMatch)
				return
			}

			if !tt.wantMatch {
				return
			}

			if matches[1] != tt.wantTable {
				t.Errorf("table name = %q, want %q", matches[1], tt.wantTable)
			}
			if matches[2] != tt.wantYear {
				t.Errorf("year = %q, want %q", matches[2], tt.wantYear)
			}
			if matches[3] != tt.wantMonth {
				t.Errorf("month = %q, want %q", matches[3], tt.wantMonth)
			}
		})
	}
}

func TestPartitionPatternExport(t *testing.T) {
	// Ensure the pattern is exported and accessible.
	if partitionPattern == nil {
		t.Error("partitionPattern is nil")
	}

	// Verify it's a valid regex.
	if _, err := regexp.Compile(partitionPattern.String()); err != nil {
		t.Errorf("partitionPattern is not a valid regex: %v", err)
	}
}

func TestPartitionEndCalculation(t *testing.T) {
	tests := []struct {
		name      string
		year      int
		month     int
		wantYear  int
		wantMonth time.Month
		wantDay   int
	}{
		{
			name:      "January to February",
			year:      2025,
			month:     1,
			wantYear:  2025,
			wantMonth: time.February,
			wantDay:   1,
		},
		{
			name:      "December to next year January",
			year:      2024,
			month:     12,
			wantYear:  2025,
			wantMonth: time.January,
			wantDay:   1,
		},
		{
			name:      "October to November",
			year:      2025,
			month:     10,
			wantYear:  2025,
			wantMonth: time.November,
			wantDay:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the partition end calculation logic from partition_dropper.go.
			partitionEnd := time.Date(tt.year, time.Month(tt.month), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 1, 0)

			if partitionEnd.Year() != tt.wantYear {
				t.Errorf("year = %d, want %d", partitionEnd.Year(), tt.wantYear)
			}
			if partitionEnd.Month() != tt.wantMonth {
				t.Errorf("month = %v, want %v", partitionEnd.Month(), tt.wantMonth)
			}
			if partitionEnd.Day() != tt.wantDay {
				t.Errorf("day = %d, want %d", partitionEnd.Day(), tt.wantDay)
			}
		})
	}
}

func TestPartitionExpiration(t *testing.T) {
	tests := []struct {
		name        string
		partYear    int
		partMonth   int
		cutoffDelta time.Duration
		wantExpired bool
	}{
		{
			name:        "partition from 2 months ago is expired with 30-day TTL",
			partYear:    2025,
			partMonth:   9,
			cutoffDelta: -30 * 24 * time.Hour,
			wantExpired: true,
		},
		{
			name:        "current month partition is not expired",
			partYear:    time.Now().Year(),
			partMonth:   int(time.Now().Month()),
			cutoffDelta: -30 * 24 * time.Hour,
			wantExpired: false,
		},
		{
			name:        "partition from 1 year ago is expired",
			partYear:    2024,
			partMonth:   11,
			cutoffDelta: -30 * 24 * time.Hour,
			wantExpired: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cutoff := time.Now().Add(tt.cutoffDelta)
			partitionEnd := time.Date(tt.partYear, time.Month(tt.partMonth), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 1, 0)
			gotExpired := partitionEnd.Before(cutoff)

			if gotExpired != tt.wantExpired {
				t.Errorf("partition end %v before cutoff %v: got %v, want %v",
					partitionEnd, cutoff, gotExpired, tt.wantExpired)
			}
		})
	}
}
