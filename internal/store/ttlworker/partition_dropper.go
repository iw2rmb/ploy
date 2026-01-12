// Package ttlworker provides a background worker for purging expired data from the database.
package ttlworker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iw2rmb/ploy/internal/store"
)

// partitionTable describes a partitioned table and how to list its partitions.
type partitionTable struct {
	name   string
	listFn func(context.Context) ([]string, error)
}

// partitionPattern matches partition names like "ploy.logs_2025_10" and extracts year and month.
// partitionPattern enforces month range 01..12 to avoid mis-parsing invalid names
// like 00 or 13, which Go's time.Date would otherwise normalize to adjacent months.
var partitionPattern = regexp.MustCompile(`^ploy\.(\w+)_(\d{4})_(0[1-9]|1[0-2])$`)

// DropOldPartitions drops entire monthly partitions older than the cutoff date.
// This is much more efficient than row-by-row deletion for large partitions.
// Stray rows in the parent table (if any) are handled by the fallback DELETE queries.
// Returns an aggregated error if any table operations fail.
func DropOldPartitions(ctx context.Context, pool *pgxpool.Pool, st store.Store, cutoff time.Time, logger *slog.Logger) error {
	if pool == nil || st == nil {
		return nil
	}

	// Define partitioned tables and their list functions.
	tables := []partitionTable{
		{"logs", st.ListLogPartitions},
		{"events", st.ListEventPartitions},
		{"artifact_bundles", st.ListArtifactBundlePartitions},
		{"node_metrics", st.ListNodeMetricsPartitions},
	}

	var errs []error
	for _, table := range tables {
		if err := dropPartitionsForTable(ctx, pool, table.name, cutoff, logger, table.listFn); err != nil {
			logger.Error("partition-dropper: failed to drop old partitions", "table", table.name, "err", err)
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// dropPartitionsForTable drops partitions for a specific table that are older than cutoff.
func dropPartitionsForTable(
	ctx context.Context,
	pool *pgxpool.Pool,
	tableName string,
	cutoff time.Time,
	logger *slog.Logger,
	listFn func(context.Context) ([]string, error),
) error {
	// List all partitions for this table.
	partitions, err := listFn(ctx)
	if err != nil {
		return fmt.Errorf("list partitions for %s: %w", tableName, err)
	}

	droppedCount := 0
	for _, partName := range partitions {
		// Parse partition name to extract year and month.
		matches := partitionPattern.FindStringSubmatch(partName)
		if len(matches) != 4 {
			// Not a recognized partition pattern; skip.
			logger.Warn("partition-dropper: unrecognized partition name", "table", tableName, "partition", partName)
			continue
		}

		// matches[1] = table name, matches[2] = year, matches[3] = month
		year, err := strconv.Atoi(matches[2])
		if err != nil {
			logger.Warn("partition-dropper: invalid year in partition", "table", tableName, "partition", partName, "err", err)
			continue
		}
		month, err := strconv.Atoi(matches[3])
		if err != nil {
			logger.Warn("partition-dropper: invalid month in partition", "table", tableName, "partition", partName, "err", err)
			continue
		}

		// Calculate the end of the partition month (start of next month).
		partitionEnd := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 1, 0)

		// If the partition end is before the cutoff, the entire partition is expired.
		if partitionEnd.Before(cutoff) {
			// Drop the partition.
			dropSQL := fmt.Sprintf("DROP TABLE IF EXISTS %s CASCADE", partName)
			if _, err := pool.Exec(ctx, dropSQL); err != nil {
				logger.Error("partition-dropper: failed to drop partition", "table", tableName, "partition", partName, "err", err)
				continue
			}
			logger.Info("partition-dropper: dropped partition", "table", tableName, "partition", partName)
			droppedCount++
		}
	}

	if droppedCount > 0 {
		logger.Info("partition-dropper: dropped partitions", "table", tableName, "count", droppedCount)
	}

	return nil
}
