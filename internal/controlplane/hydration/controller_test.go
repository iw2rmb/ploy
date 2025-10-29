package hydration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/iw2rmb/ploy/internal/controlplane/scheduler"
	"github.com/iw2rmb/ploy/internal/metrics"
)

type stubCluster struct {
	pins  []pinRequest
	unpin []string
	err   error
}

type pinRequest struct {
	CID string
	Min int
	Max int
}

func (s *stubCluster) Pin(ctx context.Context, cid string, opts PinOptions) error {
	if s.err != nil {
		return s.err
	}
	s.pins = append(s.pins, pinRequest{CID: cid, Min: opts.ReplicationFactorMin, Max: opts.ReplicationFactorMax})
	return nil
}

func (s *stubCluster) Unpin(ctx context.Context, cid string) error {
	if s.err != nil {
		return s.err
	}
	s.unpin = append(s.unpin, cid)
	return nil
}

// TestControllerApplyReplicationDecision ensures replication overrides persist and pin updates trigger.
func TestControllerApplyReplicationDecision(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	e, client := newTestEtcd(t)
	defer e.Close()
	defer client.Close()

	clock := func() time.Time { return time.Date(2025, 10, 28, 21, 50, 0, 0, time.UTC) }
	index, err := NewIndex(client, IndexOptions{Clock: clock})
	if err != nil {
		t.Fatalf("new index: %v", err)
	}
	record := SnapshotRecord{
		RepoURL:     "https://git.example.com/org/repo.git",
		Revision:    "abc123",
		TicketID:    "ticket-1",
		Bundle:      scheduler.BundleRecord{CID: "cid-1", TTL: scheduler.HydrationSnapshotTTL},
		Replication: ReplicationPolicy{Min: 1, Max: 5},
	}
	entry, err := index.UpsertSnapshot(ctx, record)
	if err != nil {
		t.Fatalf("upsert snapshot: %v", err)
	}

	cluster := &stubCluster{}
	metricRecorder, err := metrics.NewHydrationMetrics(prometheus.NewRegistry())
	if err != nil {
		t.Fatalf("new hydration metrics: %v", err)
	}

	controller, err := NewController(client, ControllerOptions{Index: index, Cluster: cluster, Clock: clock, Metrics: metricRecorder})
	if err != nil {
		t.Fatalf("new controller: %v", err)
	}
	t.Cleanup(controller.Close)

	decision := PolicyDecision{
		PolicyID:          "policy",
		Fingerprint:       entry.Fingerprint,
		Action:            PolicyActionReduceReplication,
		Level:             PolicyDecisionLevelEnforce,
		TargetReplication: 3,
		Snapshot:          entry,
	}
	if err := controller.applyReplicationDecision(ctx, decision); err != nil {
		t.Fatalf("apply decision: %v", err)
	}

	updated, ok, err := index.LookupSnapshot(ctx, LookupRequest{RepoURL: record.RepoURL, Revision: record.Revision})
	if err != nil || !ok {
		t.Fatalf("expected snapshot present, err=%v ok=%t", err, ok)
	}
	if updated.Replication.Max != 3 {
		t.Fatalf("expected replication max 3, got %d", updated.Replication.Max)
	}
	if len(cluster.pins) != 1 || cluster.pins[0].Max != 3 {
		t.Fatalf("expected cluster pin update, got %+v", cluster.pins)
	}
}

// TestControllerApplyEvictionDecision ensures eviction decisions unpin and delete snapshots.
func TestControllerApplyEvictionDecision(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	e, client := newTestEtcd(t)
	defer e.Close()
	defer client.Close()

	index, err := NewIndex(client, IndexOptions{})
	if err != nil {
		t.Fatalf("new index: %v", err)
	}
	record := SnapshotRecord{
		RepoURL:  "https://git.example.com/org/repo.git",
		Revision: "def456",
		TicketID: "ticket-2",
		Bundle:   scheduler.BundleRecord{CID: "cid-2", TTL: scheduler.HydrationSnapshotTTL},
	}
	entry, err := index.UpsertSnapshot(ctx, record)
	if err != nil {
		t.Fatalf("upsert snapshot: %v", err)
	}

	cluster := &stubCluster{}
	metricRecorder, err := metrics.NewHydrationMetrics(prometheus.NewRegistry())
	if err != nil {
		t.Fatalf("new hydration metrics: %v", err)
	}

	controller, err := NewController(client, ControllerOptions{Index: index, Cluster: cluster, Metrics: metricRecorder})
	if err != nil {
		t.Fatalf("new controller: %v", err)
	}
	t.Cleanup(controller.Close)

	decision := PolicyDecision{
		PolicyID:    "policy",
		Fingerprint: entry.Fingerprint,
		Action:      PolicyActionEvictSnapshot,
		Level:       PolicyDecisionLevelEnforce,
		Snapshot:    entry,
	}
	if err := controller.applyEvictionDecision(ctx, decision); err != nil {
		t.Fatalf("apply eviction: %v", err)
	}

	if len(cluster.unpin) != 1 || cluster.unpin[0] != entry.Bundle.CID {
		t.Fatalf("expected unpin call, got %+v", cluster.unpin)
	}
	if _, ok, err := index.LookupSnapshot(ctx, LookupRequest{RepoURL: record.RepoURL, Revision: record.Revision}); err != nil || ok {
		t.Fatalf("expected snapshot removed, ok=%t err=%v", ok, err)
	}
}

// TestControllerHandlesCompletion ensures controller indexes hydration snapshots and pins them.
func TestControllerHandlesCompletion(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	e, client := newTestEtcd(t)
	defer e.Close()
	defer client.Close()

	cluster := &stubCluster{}
	index, err := NewIndex(client, IndexOptions{
		Prefix: "hydration/index/",
		Clock: func() time.Time {
			return time.Date(2025, 10, 28, 21, 30, 0, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatalf("new index: %v", err)
	}

	metricRecorder, err := metrics.NewHydrationMetrics(prometheus.NewRegistry())
	if err != nil {
		t.Fatalf("new hydration metrics: %v", err)
	}

	controller, err := NewController(client, ControllerOptions{
		Index:         index,
		Cluster:       cluster,
		DefaultPolicy: ReplicationPolicy{Min: 2, Max: 3},
		Metrics:       metricRecorder,
	})
	if err != nil {
		t.Fatalf("new controller: %v", err)
	}
	t.Cleanup(controller.Close)

	completion := CompletionEvent{
		TicketID: "mod-123",
		StageID:  "mods-plan",
		RepoURL:  "https://git.example.com/org/repo.git",
		Revision: "deadbeef",
		Bundle: scheduler.BundleRecord{
			CID:       "bafy-snapshot",
			Digest:    "sha256:abc",
			Size:      4096,
			TTL:       scheduler.HydrationSnapshotTTL,
			ExpiresAt: time.Date(2025, 10, 29, 21, 30, 0, 0, time.UTC).Format(time.RFC3339Nano),
			Retained:  true,
		},
	}

	if err := controller.HandleCompletion(ctx, completion); err != nil {
		t.Fatalf("handle completion: %v", err)
	}

	if len(cluster.pins) != 1 || cluster.pins[0].CID != completion.Bundle.CID {
		t.Fatalf("expected cluster pin for hydration bundle")
	}
	if cluster.pins[0].Min != 2 || cluster.pins[0].Max != 3 {
		t.Fatalf("unexpected replication factors: %#v", cluster.pins[0])
	}

	entry, ok, err := index.LookupSnapshot(ctx, LookupRequest{
		RepoURL:  completion.RepoURL,
		Revision: completion.Revision,
	})
	if err != nil {
		t.Fatalf("lookup snapshot: %v", err)
	}
	if !ok {
		t.Fatalf("expected entry indexed")
	}
	if _, exists := entry.Tickets["mod-123"]; !exists {
		t.Fatalf("expected ticket recorded")
	}
}

// TestControllerHandleCompletionIgnoresClusterFailure verifies failures bubble up when pinning fails.
func TestControllerHandleCompletionIgnoresClusterFailure(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	e, client := newTestEtcd(t)
	defer e.Close()
	defer client.Close()

	cluster := &stubCluster{err: errors.New("pin failed")}
	index, err := NewIndex(client, IndexOptions{
		Prefix: "hydration/index/",
		Clock:  time.Now,
	})
	if err != nil {
		t.Fatalf("new index: %v", err)
	}
	metricRecorder, err := metrics.NewHydrationMetrics(prometheus.NewRegistry())
	if err != nil {
		t.Fatalf("new hydration metrics: %v", err)
	}
	controller, err := NewController(client, ControllerOptions{
		Index:   index,
		Cluster: cluster,
		Metrics: metricRecorder,
	})
	if err != nil {
		t.Fatalf("new controller: %v", err)
	}
	t.Cleanup(controller.Close)

	err = controller.HandleCompletion(ctx, CompletionEvent{
		TicketID: "mod-456",
		StageID:  "mods-plan",
		RepoURL:  "https://git.example.com/org/repo.git",
		Revision: "cafebabe",
		Bundle: scheduler.BundleRecord{
			CID:       "bafy-zzz",
			Digest:    "sha256:zzz",
			Size:      111,
			TTL:       scheduler.HydrationSnapshotTTL,
			ExpiresAt: time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339Nano),
		},
	})
	if err == nil {
		t.Fatalf("expected failure when cluster pin fails")
	}
}
