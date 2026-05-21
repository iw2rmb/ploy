package ttlworker

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestPartitionPattern validates the regex pattern that matches partition names.
// It ensures correct extraction of table name, year, and month, and rejects
// invalid formats such as wrong schema, out-of-range months, or malformed names.
func TestPartitionPattern(t *testing.T) {
	tests := []struct {
		name      string
		partition string
		wantMatch bool
		wantTable string
		wantYear  string
		wantMonth string
	}{
		{
			name:      "valid logs partition",
			partition: "ploy.logs_2025_10",
			wantMatch: true,
			wantTable: "logs",
			wantYear:  "2025",
			wantMonth: "10",
		},
		{
			name:      "valid events partition",
			partition: "ploy.events_2024_01",
			wantMatch: true,
			wantTable: "events",
			wantYear:  "2024",
			wantMonth: "01",
		},
		{
			name:      "valid artifact_bundles partition",
			partition: "ploy.artifact_bundles_2023_12",
			wantMatch: true,
			wantTable: "artifact_bundles",
			wantYear:  "2023",
			wantMonth: "12",
		},
		{
			name:      "valid node_metrics partition",
			partition: "ploy.node_metrics_2025_06",
			wantMatch: true,
			wantTable: "node_metrics",
			wantYear:  "2025",
			wantMonth: "06",
		},
		{
			name:      "month out of range 00",
			partition: "ploy.logs_2025_00",
			wantMatch: false,
		},
		{
			name:      "month out of range 13",
			partition: "ploy.logs_2025_13",
			wantMatch: false,
		},
		{
			name:      "invalid schema",
			partition: "public.logs_2025_10",
			wantMatch: false,
		},
		{
			name:      "invalid year format",
			partition: "ploy.logs_25_10",
			wantMatch: false,
		},
		{
			name:      "invalid month format",
			partition: "ploy.logs_2025_1",
			wantMatch: false,
		},
		{
			name:      "no underscores",
			partition: "ploy.logs",
			wantMatch: false,
		},
		{
			name:      "extra parts",
			partition: "ploy.logs_2025_10_extra",
			wantMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := partitionPattern.FindStringSubmatch(tt.partition)
			gotMatch := len(matches) == 4

			if gotMatch != tt.wantMatch {
				t.Errorf("partition %q: got match=%v, want match=%v", tt.partition, gotMatch, tt.wantMatch)
			}

			if tt.wantMatch && gotMatch {
				if matches[1] != tt.wantTable {
					t.Errorf("table: got %q, want %q", matches[1], tt.wantTable)
				}
				if matches[2] != tt.wantYear {
					t.Errorf("year: got %q, want %q", matches[2], tt.wantYear)
				}
				if matches[3] != tt.wantMonth {
					t.Errorf("month: got %q, want %q", matches[3], tt.wantMonth)
				}
			}
		})
	}
}

// TestDropOldPartitions_ListErrors ensures that listing errors for individual
// tables are aggregated and returned. The function continues processing other
// tables even when errors occur.
func TestDropOldPartitions_ListErrors(t *testing.T) {
	ctx := context.Background()
	cutoff := time.Now()
	logger := slog.Default()

	tests := []struct {
		name       string
		setupStore func(*mockStoreWithPartitions)
		wantErr    string
	}{
		{
			name: "list logs error",
			setupStore: func(m *mockStoreWithPartitions) {
				m.listLogsErr = errors.New("list logs failed")
			},
			wantErr: "list logs failed",
		},
		{
			name: "list events error",
			setupStore: func(m *mockStoreWithPartitions) {
				m.listEventsErr = errors.New("list events failed")
			},
			wantErr: "list events failed",
		},
		{
			name: "list artifacts error",
			setupStore: func(m *mockStoreWithPartitions) {
				m.listArtifactsErr = errors.New("list artifacts failed")
			},
			wantErr: "list artifacts failed",
		},
		{
			name: "list metrics error",
			setupStore: func(m *mockStoreWithPartitions) {
				m.listMetricsErr = errors.New("list metrics failed")
			},
			wantErr: "list metrics failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockStoreWithPartitions{}
			tt.setupStore(mock)

			// Should return error when listing fails (errors are aggregated).
			err := DropOldPartitions(ctx, &pgxpool.Pool{}, mock, cutoff, logger)
			if err == nil {
				t.Error("expected error, got nil")
			} else if !errors.Is(err, errors.Unwrap(err)) {
				// Just verify we got an error containing the expected message
				if !containsError(err, tt.wantErr) {
					t.Errorf("expected error containing %q, got %v", tt.wantErr, err)
				}
			}
		})
	}
}

// containsError checks if the error message contains the given substring.
func containsError(err error, substr string) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), substr)
}
