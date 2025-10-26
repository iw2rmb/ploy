package artifacts_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/iw2rmb/ploy/internal/controlplane/artifacts"
	"github.com/iw2rmb/ploy/internal/controlplane/transfers"
	workflowartifacts "github.com/iw2rmb/ploy/internal/workflow/artifacts"
)

func TestReconcilerRetryFlow(t *testing.T) {
	etcd, client := startTestEtcd(t)
	t.Cleanup(func() {
		etcd.Close()
		client.Close()
	})

	store, err := artifacts.NewStore(client, artifacts.StoreOptions{})
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	fakeCluster := &integrationCluster{}
	manager := transfers.NewManager(transfers.Options{
		BaseDir:   t.TempDir(),
		Store:     store,
		Publisher: fakeCluster,
	})

	slot, err := manager.CreateUploadSlot(transfers.KindRepo, "job-retry", "", "node-1", 0)
	if err != nil {
		t.Fatalf("CreateUploadSlot: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(slot.RemotePath), 0o755); err != nil {
		t.Fatalf("prepare slot dir: %v", err)
	}
	if err := os.WriteFile(slot.RemotePath, []byte("payload"), 0o644); err != nil {
		t.Fatalf("write slot payload: %v", err)
	}
	digest := sha256.Sum256([]byte("payload"))
	digestValue := "sha256:" + hex.EncodeToString(digest[:])
	if _, err := manager.Commit(context.Background(), slot.ID, int64(len("payload")), digestValue); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	fakeCluster.artifactID = slot.ID

	fakeCluster.setSummary("pin_error")

	clk := &stubClock{current: time.Date(2025, 10, 26, 17, 0, 0, 0, time.UTC)}
	reconciler := artifacts.NewReconciler(artifacts.ReconcilerOptions{
		Store:      store,
		Cluster:    fakeCluster,
		RetryDelay: 10 * time.Millisecond,
		Clock:      clk.Now,
	})

	if err := reconciler.RunOnce(context.Background()); err != nil {
		t.Fatalf("first RunOnce: %v", err)
	}

	meta, err := store.Get(context.Background(), fakeCluster.artifactID)
	if err != nil {
		t.Fatalf("Get metadata: %v", err)
	}
	if meta.PinState != artifacts.PinStatePinning {
		t.Fatalf("expected pinning after retry, got %s", meta.PinState)
	}
	if meta.PinRetryCount != 1 {
		t.Fatalf("expected retry count 1, got %d", meta.PinRetryCount)
	}
	if !fakeCluster.pinCalled {
		t.Fatalf("expected pin to be invoked")
	}
	clk.Advance(20 * time.Millisecond)

	if err := reconciler.RunOnce(context.Background()); err != nil {
		t.Fatalf("second RunOnce: %v", err)
	}
	meta, err = store.Get(context.Background(), fakeCluster.artifactID)
	if err != nil {
		t.Fatalf("Get metadata: %v", err)
	}
	if meta.PinState != artifacts.PinStatePinned {
		t.Fatalf("expected pinned state, got %s (summary=%s, statusCalls=%d)", meta.PinState, fakeCluster.summary, fakeCluster.statusCalls)
	}
	if meta.PinReplicas != 1 {
		t.Fatalf("expected replica count 1, got %d", meta.PinReplicas)
	}
}

type integrationCluster struct {
	artifactID  string
	pinCalled   bool
	summary     string
	statusCalls int
}

func (c *integrationCluster) Add(ctx context.Context, req workflowartifacts.AddRequest) (workflowartifacts.AddResponse, error) {
	return workflowartifacts.AddResponse{CID: "bafy-integration", Digest: "sha256:payload", Name: req.Name, Size: int64(len(req.Payload))}, nil
}

func (c *integrationCluster) Fetch(ctx context.Context, cid string) (workflowartifacts.FetchResult, error) {
	return workflowartifacts.FetchResult{}, nil
}

func (c *integrationCluster) Status(ctx context.Context, cid string) (workflowartifacts.StatusResult, error) {
	c.statusCalls++
	peers := []workflowartifacts.StatusPeer{}
	if c.summary == "pinned" {
		peers = append(peers, workflowartifacts.StatusPeer{PeerID: "peer-1", Status: "pinned"})
	}
	return workflowartifacts.StatusResult{
		CID:                  cid,
		Summary:              c.summary,
		ReplicationFactorMin: 1,
		ReplicationFactorMax: 1,
		PinState:             c.summary,
		PinReplicas:          btoi(c.summary == "pinned"),
		Peers:                peers,
	}, nil
}

func (c *integrationCluster) Pin(ctx context.Context, cid string, opts workflowartifacts.PinOptions) error {
	c.pinCalled = true
	c.setSummary("pinned")
	return nil
}

func (c *integrationCluster) setSummary(state string) {
	c.summary = state
}

func btoi(cond bool) int {
	if cond {
		return 1
	}
	return 0
}

type stubClock struct {
	current time.Time
}

func (c *stubClock) Now() time.Time {
	return c.current
}

func (c *stubClock) Advance(d time.Duration) {
	c.current = c.current.Add(d)
}
