package ttlworker

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// mockStoreWithPartitions implements the Store interface with partition listing.
// This mock is shared across partition dropper tests to simulate store behavior
// for both listing partitions and handling various error conditions.
type mockStoreWithPartitions struct {
	mockStore
	logPartitions            []string
	eventPartitions          []string
	artifactBundlePartitions []string
	nodeMetricsPartitions    []string
	listLogsErr              error
	listEventsErr            error
	listArtifactsErr         error
	listMetricsErr           error
	pool                     *pgxpool.Pool
}

func (m *mockStoreWithPartitions) ListLogPartitions(ctx context.Context) ([]string, error) {
	return m.logPartitions, m.listLogsErr
}

func (m *mockStoreWithPartitions) ListEventPartitions(ctx context.Context) ([]string, error) {
	return m.eventPartitions, m.listEventsErr
}

func (m *mockStoreWithPartitions) ListArtifactBundlePartitions(ctx context.Context) ([]string, error) {
	return m.artifactBundlePartitions, m.listArtifactsErr
}

func (m *mockStoreWithPartitions) ListNodeMetricsPartitions(ctx context.Context) ([]string, error) {
	return m.nodeMetricsPartitions, m.listMetricsErr
}

func (m *mockStoreWithPartitions) Pool() *pgxpool.Pool {
	return m.pool
}
