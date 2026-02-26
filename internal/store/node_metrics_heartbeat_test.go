package store

import (
	"context"
	"net/netip"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/domain/types"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestUpdateNodeHeartbeat_AppendsNodeMetricsHistory(t *testing.T) {
	ctx, db := openStoreForCancelBulkTests(t)

	nodeID := types.NewNodeKey()
	node, err := db.CreateNode(ctx, CreateNodeParams{
		ID:          types.NodeID(nodeID),
		Name:        "metrics-history-" + nodeID,
		IpAddress:   netip.AddrFrom4([4]byte{nodeID[0], nodeID[1], nodeID[2], nodeID[3]}),
		Concurrency: 1,
	})
	if err != nil {
		t.Fatalf("CreateNode() failed: %v", err)
	}

	first := UpdateNodeHeartbeatParams{
		ID:             node.ID,
		LastHeartbeat:  pgtype.Timestamptz{Time: time.Now().UTC().Add(-2 * time.Second), Valid: true},
		CpuTotalMillis: 4000,
		CpuFreeMillis:  2500,
		MemTotalBytes:  8 * 1024 * 1024 * 1024,
		MemFreeBytes:   5 * 1024 * 1024 * 1024,
		DiskTotalBytes: 200 * 1024 * 1024 * 1024,
		DiskFreeBytes:  120 * 1024 * 1024 * 1024,
		Version:        "ployd-node/test-v1",
	}
	if err := db.UpdateNodeHeartbeat(ctx, first); err != nil {
		t.Fatalf("UpdateNodeHeartbeat(first) failed: %v", err)
	}

	if got := countNodeMetricsRows(t, ctx, db, node.ID); got != 1 {
		t.Fatalf("node_metrics rows after first heartbeat = %d, want 1", got)
	}
	assertLatestNodeMetric(t, ctx, db, node.ID, first)

	second := UpdateNodeHeartbeatParams{
		ID:             node.ID,
		LastHeartbeat:  pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		CpuTotalMillis: 4000,
		CpuFreeMillis:  1400,
		MemTotalBytes:  8 * 1024 * 1024 * 1024,
		MemFreeBytes:   2 * 1024 * 1024 * 1024,
		DiskTotalBytes: 200 * 1024 * 1024 * 1024,
		DiskFreeBytes:  80 * 1024 * 1024 * 1024,
		Version:        "ployd-node/test-v2",
	}
	if err := db.UpdateNodeHeartbeat(ctx, second); err != nil {
		t.Fatalf("UpdateNodeHeartbeat(second) failed: %v", err)
	}

	if got := countNodeMetricsRows(t, ctx, db, node.ID); got != 2 {
		t.Fatalf("node_metrics rows after second heartbeat = %d, want 2", got)
	}
	assertLatestNodeMetric(t, ctx, db, node.ID, second)
}

func countNodeMetricsRows(t *testing.T, ctx context.Context, db Store, nodeID types.NodeID) int {
	t.Helper()

	var count int
	if err := db.Pool().QueryRow(ctx, `SELECT COUNT(*) FROM ploy.node_metrics WHERE node_id = $1`, nodeID).Scan(&count); err != nil {
		t.Fatalf("count node_metrics rows failed: %v", err)
	}
	return count
}

func assertLatestNodeMetric(t *testing.T, ctx context.Context, db Store, nodeID types.NodeID, want UpdateNodeHeartbeatParams) {
	t.Helper()

	var (
		cpuTotal  int32
		cpuFree   int32
		memTotal  int64
		memFree   int64
		diskTotal int64
		diskFree  int64
	)
	err := db.Pool().QueryRow(
		ctx,
		`SELECT cpu_total_millis, cpu_free_millis, mem_total_bytes, mem_free_bytes, disk_total_bytes, disk_free_bytes
FROM ploy.node_metrics
WHERE node_id = $1
ORDER BY id DESC
LIMIT 1`,
		nodeID,
	).Scan(&cpuTotal, &cpuFree, &memTotal, &memFree, &diskTotal, &diskFree)
	if err != nil {
		t.Fatalf("read latest node_metrics row failed: %v", err)
	}

	if cpuTotal != want.CpuTotalMillis {
		t.Fatalf("latest cpu_total_millis = %d, want %d", cpuTotal, want.CpuTotalMillis)
	}
	if cpuFree != want.CpuFreeMillis {
		t.Fatalf("latest cpu_free_millis = %d, want %d", cpuFree, want.CpuFreeMillis)
	}
	if memTotal != want.MemTotalBytes {
		t.Fatalf("latest mem_total_bytes = %d, want %d", memTotal, want.MemTotalBytes)
	}
	if memFree != want.MemFreeBytes {
		t.Fatalf("latest mem_free_bytes = %d, want %d", memFree, want.MemFreeBytes)
	}
	if diskTotal != want.DiskTotalBytes {
		t.Fatalf("latest disk_total_bytes = %d, want %d", diskTotal, want.DiskTotalBytes)
	}
	if diskFree != want.DiskFreeBytes {
		t.Fatalf("latest disk_free_bytes = %d, want %d", diskFree, want.DiskFreeBytes)
	}
}
