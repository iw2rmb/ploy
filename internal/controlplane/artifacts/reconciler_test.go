package artifacts_test

import (
	"context"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/controlplane/artifacts"
	workflowartifacts "github.com/iw2rmb/ploy/internal/workflow/artifacts"
)

func TestReconcilerMarksArtifactsPinned(t *testing.T) {
	t.Parallel()

	etcd, client := startTestEtcd(t)
	t.Cleanup(func() {
		etcd.Close()
		client.Close()
	})

	store, err := artifacts.NewStore(client, artifacts.StoreOptions{Clock: fixedClock(time.Date(2025, 10, 26, 15, 0, 0, 0, time.UTC))})
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	ctx := context.Background()
	if _, err := store.Create(ctx, artifacts.Metadata{
		ID:     "artifact-pin",
		CID:    "bafy-pin",
		Digest: "sha256:pin",
		Size:   64,
		JobID:  "job-pin",
		Kind:   "logs",
	}); err != nil {
		t.Fatalf("seed artifact: %v", err)
	}

	fakeCluster := &fakeClusterClient{
		status: workflowartifacts.StatusResult{
			CID:     "bafy-pin",
			Summary: "pinned",
			Peers: []workflowartifacts.StatusPeer{
				{PeerID: "peer-a", Status: "pinned"},
			},
		},
	}
	metricsStub := newStubPinMetrics()
	rec := artifacts.NewReconciler(artifacts.ReconcilerOptions{
		Store:      store,
		Cluster:    fakeCluster,
		Interval:   time.Second,
		BatchSize:  10,
		Metrics:    metricsStub,
		Clock:      fixedClock(time.Date(2025, 10, 26, 15, 0, 0, 0, time.UTC)),
		RetryDelay: 30 * time.Second,
	})

	if err := rec.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	updated, err := store.Get(ctx, "artifact-pin")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if updated.PinState != artifacts.PinStatePinned {
		t.Fatalf("expected pinned state, got %s", updated.PinState)
	}
	if updated.PinReplicas != 1 {
		t.Fatalf("expected replicas=1, got %d", updated.PinReplicas)
	}
	if updated.PinRetryCount != 0 {
		t.Fatalf("expected retry count 0, got %d", updated.PinRetryCount)
	}
	if metricsStub.stateCounts[string(artifacts.PinStatePinned)] != 1 {
		t.Fatalf("expected metrics to record pinned state")
	}
}

func TestReconcilerRetriesFailedPins(t *testing.T) {
	t.Parallel()

	etcd, client := startTestEtcd(t)
	t.Cleanup(func() {
		etcd.Close()
		client.Close()
	})

	clock := fixedClock(time.Date(2025, 10, 26, 15, 30, 0, 0, time.UTC))
	store, err := artifacts.NewStore(client, artifacts.StoreOptions{Clock: clock})
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	ctx := context.Background()
	base := clock()
	if _, err := store.Create(ctx, artifacts.Metadata{
		ID:               "artifact-retry",
		CID:              "bafy-retry",
		Digest:           "sha256:retry",
		Size:             32,
		JobID:            "job-retry",
		Kind:             "logs",
		PinState:         artifacts.PinStateFailed,
		PinRetryCount:    1,
		PinNextAttemptAt: base.Add(-time.Minute),
	}); err != nil {
		t.Fatalf("seed artifact: %v", err)
	}

	fakeCluster := &fakeClusterClient{
		status: workflowartifacts.StatusResult{
			CID:     "bafy-retry",
			Summary: "pin_error",
		},
	}
	metricsStub := newStubPinMetrics()
	rec := artifacts.NewReconciler(artifacts.ReconcilerOptions{
		Store:      store,
		Cluster:    fakeCluster,
		Interval:   time.Second,
		BatchSize:  10,
		Metrics:    metricsStub,
		Clock:      clock,
		RetryDelay: time.Minute,
	})

	if err := rec.RunOnce(ctx); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	updated, err := store.Get(ctx, "artifact-retry")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if updated.PinState != artifacts.PinStatePinning {
		t.Fatalf("expected state pinning, got %s", updated.PinState)
	}
	if updated.PinRetryCount != 2 {
		t.Fatalf("expected retry count 2, got %d", updated.PinRetryCount)
	}
	if updated.PinNextAttemptAt.Sub(base.Add(time.Minute)) != 0 {
		t.Fatalf("expected next attempt scheduled, got %s", updated.PinNextAttemptAt)
	}
	if !fakeCluster.pinCalled {
		t.Fatalf("expected cluster pin to be invoked")
	}
	if metricsStub.retryCount != 1 {
		t.Fatalf("expected retry metric increment")
	}
}

type fakeClusterClient struct {
	status    workflowartifacts.StatusResult
	statusErr error
	pinCalled bool
}

func (f *fakeClusterClient) Status(ctx context.Context, cid string) (workflowartifacts.StatusResult, error) {
	if f.statusErr != nil {
		return workflowartifacts.StatusResult{}, f.statusErr
	}
	return f.status, nil
}

func (f *fakeClusterClient) Pin(ctx context.Context, cid string, opts workflowartifacts.PinOptions) error {
	f.pinCalled = true
	return nil
}

type stubPinMetrics struct {
	stateCounts map[string]int
	retryCount  int
}

func newStubPinMetrics() *stubPinMetrics {
	return &stubPinMetrics{
		stateCounts: make(map[string]int),
	}
}

func (m *stubPinMetrics) UpdateState(counts map[string]int) {
	for k, v := range counts {
		m.stateCounts[k] = v
	}
}

func (m *stubPinMetrics) ObserveRetry(kind string) {
	_ = kind
	m.retryCount++
}
