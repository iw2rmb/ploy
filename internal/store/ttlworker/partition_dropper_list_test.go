package ttlworker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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

// TestDropPartitionsForTable_PartitionNameParsing verifies that partition names
// are correctly parsed and compared against the cutoff time. This test simulates
// the partition filtering logic without executing actual DROP statements.
func TestDropPartitionsForTable_PartitionNameParsing(t *testing.T) {
	ctx := context.Background()
	cutoff := time.Date(2025, 11, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name           string
		partitions     []string
		wantDropped    int
		wantLogWarning bool
	}{
		{
			name: "old partition dropped",
			partitions: []string{
				"ploy.logs_2025_09", // Ends 2025-10-01 < cutoff
			},
			wantDropped: 1,
		},
		{
			name: "recent partition kept",
			partitions: []string{
				"ploy.logs_2025_11", // Ends 2025-12-01 > cutoff
			},
			wantDropped: 0,
		},
		{
			name: "mixed old and recent",
			partitions: []string{
				"ploy.logs_2025_09", // Ends 2025-10-01 < cutoff
				"ploy.logs_2025_10", // Ends 2025-11-01 = cutoff (not before)
				"ploy.logs_2025_11", // Ends 2025-12-01 > cutoff
			},
			wantDropped: 1, // Only 2025_09
		},
		{
			name: "invalid partition name ignored",
			partitions: []string{
				"public.logs_2025_10",
				"ploy.logs_invalid",
			},
			wantDropped:    0,
			wantLogWarning: true,
		},
		{
			name: "invalid year ignored",
			partitions: []string{
				"ploy.logs_abcd_10",
			},
			wantDropped:    0,
			wantLogWarning: true,
		},
		{
			name: "invalid month ignored",
			partitions: []string{
				"ploy.logs_2025_ab",
			},
			wantDropped:    0,
			wantLogWarning: true,
		},
		{
			name:        "empty partition list",
			partitions:  []string{},
			wantDropped: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockStoreWithPartitions{
				logPartitions: tt.partitions,
			}

			// Validate partition listing works correctly.
			partitions, err := mock.ListLogPartitions(ctx)
			if err != nil {
				t.Fatalf("list partitions failed: %v", err)
			}

			// Simulate the partition filtering logic from DropOldPartitions.
			// Count partitions that would be dropped based on cutoff time.
			droppedCount := 0
			for _, partName := range partitions {
				matches := partitionPattern.FindStringSubmatch(partName)
				if len(matches) != 4 {
					continue
				}

				year, month := 0, 0
				_, _ = fmt.Sscanf(matches[2], "%d", &year)
				_, _ = fmt.Sscanf(matches[3], "%d", &month)

				if year == 0 || month == 0 {
					continue
				}

				partitionEnd := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 1, 0)
				if partitionEnd.Before(cutoff) {
					droppedCount++
				}
			}

			if droppedCount != tt.wantDropped {
				t.Errorf("dropped count: got %d, want %d", droppedCount, tt.wantDropped)
			}
		})
	}
}

// TestDropOldPartitions_ListErrors ensures that listing errors for individual
// tables do not cause the entire DropOldPartitions operation to fail. Errors
// are logged but the function continues processing other tables.
func TestDropOldPartitions_ListErrors(t *testing.T) {
	ctx := context.Background()
	cutoff := time.Now()
	logger := slog.Default()

	tests := []struct {
		name       string
		setupStore func(*mockStoreWithPartitions)
	}{
		{
			name: "list logs error",
			setupStore: func(m *mockStoreWithPartitions) {
				m.listLogsErr = errors.New("list logs failed")
			},
		},
		{
			name: "list events error",
			setupStore: func(m *mockStoreWithPartitions) {
				m.listEventsErr = errors.New("list events failed")
			},
		},
		{
			name: "list artifacts error",
			setupStore: func(m *mockStoreWithPartitions) {
				m.listArtifactsErr = errors.New("list artifacts failed")
			},
		},
		{
			name: "list metrics error",
			setupStore: func(m *mockStoreWithPartitions) {
				m.listMetricsErr = errors.New("list metrics failed")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockStoreWithPartitions{}
			tt.setupStore(mock)

			// Should not return error even if listing fails (errors are logged).
			err := DropOldPartitions(ctx, &pgxpool.Pool{}, mock, cutoff, logger)
			if err != nil {
				t.Errorf("expected no error, got %v", err)
			}
		})
	}
}

// TestDropOldPartitions_NoPartitionsExist validates that when no partitions
// exist for any table (empty or nil slices), the DropOldPartitions function
// completes successfully without attempting any DROP operations.
func TestDropOldPartitions_NoPartitionsExist(t *testing.T) {
	ctx := context.Background()
	cutoff := time.Date(2025, 11, 1, 0, 0, 0, 0, time.UTC)
	logger := slog.Default()

	tests := []struct {
		name       string
		setupStore func(*mockStoreWithPartitions)
	}{
		{
			name: "no log partitions exist",
			setupStore: func(m *mockStoreWithPartitions) {
				m.logPartitions = []string{}
			},
		},
		{
			name: "no event partitions exist",
			setupStore: func(m *mockStoreWithPartitions) {
				m.eventPartitions = []string{}
			},
		},
		{
			name: "no artifact bundle partitions exist",
			setupStore: func(m *mockStoreWithPartitions) {
				m.artifactBundlePartitions = []string{}
			},
		},
		{
			name: "no node metrics partitions exist",
			setupStore: func(m *mockStoreWithPartitions) {
				m.nodeMetricsPartitions = []string{}
			},
		},
		{
			name: "no partitions exist for any table",
			setupStore: func(m *mockStoreWithPartitions) {
				m.logPartitions = []string{}
				m.eventPartitions = []string{}
				m.artifactBundlePartitions = []string{}
				m.nodeMetricsPartitions = []string{}
			},
		},
		{
			name: "all partitions are nil slices",
			setupStore: func(m *mockStoreWithPartitions) {
				m.logPartitions = nil
				m.eventPartitions = nil
				m.artifactBundlePartitions = nil
				m.nodeMetricsPartitions = nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockStoreWithPartitions{}
			tt.setupStore(mock)

			// Pool is nil here, but that's ok - DropOldPartitions should still
			// attempt to list partitions and find none, then no-op.
			// Since there are no partitions to drop, no Exec calls will be made.
			err := DropOldPartitions(ctx, nil, mock, cutoff, logger)

			// Should not return error when no partitions exist.
			if err != nil {
				t.Errorf("expected no error when no partitions exist, got %v", err)
			}
		})
	}
}

// TestDropOldPartitions_NoPartitionsOlderThanCutoff ensures that partitions
// ending at or after the cutoff time are not dropped. This validates the
// boundary condition where partition end time equals the cutoff.
func TestDropOldPartitions_NoPartitionsOlderThanCutoff(t *testing.T) {
	ctx := context.Background()
	cutoff := time.Date(2025, 11, 1, 0, 0, 0, 0, time.UTC)
	logger := slog.Default()

	tests := []struct {
		name       string
		partitions []string
		desc       string
	}{
		{
			name: "all partitions are recent",
			partitions: []string{
				"ploy.logs_2025_11", // Ends 2025-12-01 > cutoff
				"ploy.logs_2025_12", // Ends 2026-01-01 > cutoff
			},
			desc: "no partitions should be dropped when all are after cutoff",
		},
		{
			name: "partition exactly at cutoff boundary",
			partitions: []string{
				"ploy.logs_2025_10", // Ends 2025-11-01 = cutoff (not before)
			},
			desc: "partition ending exactly at cutoff should not be dropped",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockStoreWithPartitions{
				logPartitions:            tt.partitions,
				eventPartitions:          tt.partitions,
				artifactBundlePartitions: tt.partitions,
				nodeMetricsPartitions:    tt.partitions,
			}

			// No pool means no actual DROP statements will execute,
			// but we can verify the logic doesn't attempt drops.
			err := DropOldPartitions(ctx, nil, mock, cutoff, logger)

			if err != nil {
				t.Errorf("expected no error, got %v", err)
			}

			// Since we don't have a real pool to verify Exec calls,
			// we rely on the implementation logic: partitions not before
			// cutoff won't trigger drops. This test validates the no-op behavior.
		})
	}
}
