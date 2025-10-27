package transfers_test

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/server/v3/embed"

	"github.com/iw2rmb/ploy/internal/controlplane/transfers"
)

func TestSlotStorePersistsSlots(t *testing.T) {
	etcd, client := startSlotStoreEtcd(t)
	t.Cleanup(func() {
		etcd.Close()
		client.Close()
	})

	now := time.Date(2025, 10, 27, 9, 30, 0, 0, time.UTC)
	store := mustNewSlotStore(t, client, transfers.SlotStoreOptions{
		ClusterID: "cluster-alpha",
		TTL:       30 * time.Minute,
		Clock:     func() time.Time { return now },
	})
	t.Cleanup(func() { _ = store.Close() })

	slot := transfers.Slot{
		ID:         "slot-alpha",
		Kind:       transfers.KindRepo,
		JobID:      "job-123",
		Stage:      "plan",
		NodeID:     "node-a",
		RemotePath: "/slots/slot-alpha/payload",
		LocalPath:  filepath.Join("/var/lib/ploy/ssh-artifacts/slots", "slot-alpha", "payload"),
		MaxSize:    32 << 20,
		ExpiresAt:  now.Add(30 * time.Minute),
		State:      transfers.SlotPending,
	}

	record, err := store.CreateSlot(context.Background(), slot)
	if err != nil {
		t.Fatalf("CreateSlot: %v", err)
	}
	if record.Revision == 0 {
		t.Fatalf("expected revision to be recorded")
	}

	fetched, err := store.GetSlot(context.Background(), slot.ID)
	if err != nil {
		t.Fatalf("GetSlot: %v", err)
	}
	if fetched.Slot.JobID != slot.JobID {
		t.Fatalf("unexpected job id: %s", fetched.Slot.JobID)
	}
	if fetched.LeaseID == 0 {
		t.Fatalf("expected lease to be persisted")
	}

	updated, err := store.UpdateSlotState(context.Background(), slot.ID, record.Revision, transfers.SlotCommitted, "sha256:deadbeef")
	if err != nil {
		t.Fatalf("UpdateSlotState: %v", err)
	}
	if updated.Slot.State != transfers.SlotCommitted {
		t.Fatalf("expected committed state, got %s", updated.Slot.State)
	}
	if updated.Slot.Digest != "sha256:deadbeef" {
		t.Fatalf("expected digest propagated, got %s", updated.Slot.Digest)
	}
}

func TestSlotStoreRejectsStaleRevision(t *testing.T) {
	etcd, client := startSlotStoreEtcd(t)
	t.Cleanup(func() {
		etcd.Close()
		client.Close()
	})

	store := mustNewSlotStore(t, client, transfers.SlotStoreOptions{ClusterID: "cluster-beta"})
	t.Cleanup(func() { _ = store.Close() })

	slot := transfers.Slot{
		ID:         "slot-stale",
		Kind:       transfers.KindRepo,
		JobID:      "job-stale",
		NodeID:     "node-a",
		RemotePath: "/slots/slot-stale/payload",
		LocalPath:  filepath.Join("/var/lib/ploy/ssh-artifacts/slots", "slot-stale", "payload"),
		ExpiresAt:  time.Now().UTC().Add(10 * time.Minute),
		State:      transfers.SlotPending,
	}
	record, err := store.CreateSlot(context.Background(), slot)
	if err != nil {
		t.Fatalf("CreateSlot: %v", err)
	}

	if _, err := store.UpdateSlotState(context.Background(), slot.ID, record.Revision-1, transfers.SlotCommitted, "sha256:oops"); err == nil {
		t.Fatalf("expected revision conflict")
	}
}

func TestSlotStoreCachesArtifactsAcrossRestarts(t *testing.T) {
	etcd, client := startSlotStoreEtcd(t)
	t.Cleanup(func() {
		etcd.Close()
		client.Close()
	})

	now := time.Date(2025, 10, 27, 10, 45, 0, 0, time.UTC)
	newStore := func() *transfers.SlotStore {
		return mustNewSlotStore(t, client, transfers.SlotStoreOptions{
			ClusterID: "cluster-cache",
			Clock:     func() time.Time { return now },
		})
	}

	store := newStore()
	slot := transfers.Slot{
		ID:         "slot-cache",
		Kind:       transfers.KindRepo,
		JobID:      "job-cache",
		NodeID:     "node-1",
		RemotePath: "/slots/slot-cache/payload",
		LocalPath:  filepath.Join("/var/lib/ploy/ssh-artifacts/slots", "slot-cache", "payload"),
		ExpiresAt:  now.Add(30 * time.Minute),
		State:      transfers.SlotPending,
	}
	if _, err := store.CreateSlot(context.Background(), slot); err != nil {
		t.Fatalf("CreateSlot: %v", err)
	}

	artifact := transfers.Artifact{
		ID:         slot.ID,
		Kind:       slot.Kind,
		JobID:      slot.JobID,
		Stage:      slot.Stage,
		NodeID:     slot.NodeID,
		RemotePath: slot.RemotePath,
		Size:       1024,
		Digest:     "sha256:cache",
		CID:        "bafy-cache",
		UpdatedAt:  now,
	}
	if err := store.RecordArtifact(context.Background(), artifact); err != nil {
		t.Fatalf("RecordArtifact: %v", err)
	}

	waitForCachedArtifact(t, store, slot.JobID, slot.ID)

	if err := store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	store = newStore()
	t.Cleanup(func() { _ = store.Close() })
	waitForCachedArtifact(t, store, slot.JobID, slot.ID)
}

func waitForCachedArtifact(t *testing.T, store *transfers.SlotStore, jobID, artifactID string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		cached := store.CachedArtifacts(jobID)
		for _, art := range cached {
			if art.ID == artifactID {
				return
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("artifact %s for job %s never cached", artifactID, jobID)
}

func mustNewSlotStore(t *testing.T, client *clientv3.Client, opts transfers.SlotStoreOptions) *transfers.SlotStore {
	t.Helper()
	store, err := transfers.NewSlotStore(client, opts)
	if err != nil {
		t.Fatalf("NewSlotStore: %v", err)
	}
	return store
}

func startSlotStoreEtcd(t *testing.T) (*embed.Etcd, *clientv3.Client) {
	t.Helper()
	cfg := embed.NewConfig()
	cfg.Dir = t.TempDir()
	clientURL := mustParseURL("http://127.0.0.1:0")
	peerURL := mustParseURL("http://127.0.0.1:0")
	cfg.ListenClientUrls = []url.URL{clientURL}
	cfg.ListenPeerUrls = []url.URL{peerURL}
	cfg.AdvertiseClientUrls = []url.URL{clientURL}
	cfg.AdvertisePeerUrls = []url.URL{peerURL}
	cfg.Name = "transfers-test"
	cfg.InitialCluster = fmt.Sprintf("%s=%s", cfg.Name, peerURL.String())
	cfg.ClusterState = embed.ClusterStateFlagNew
	cfg.InitialClusterToken = "transfers-test"
	cfg.LogLevel = "panic"

	e, err := embed.StartEtcd(cfg)
	if err != nil {
		t.Fatalf("start etcd: %v", err)
	}
	t.Cleanup(func() {
		select {
		case <-e.Server.StopNotify():
		case <-time.After(2 * time.Second):
		}
	})
	select {
	case <-e.Server.ReadyNotify():
	case <-time.After(10 * time.Second):
		e.Server.Stop()
		t.Fatalf("etcd start timeout")
	}

	client, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{e.Clients[0].Addr().String()},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		e.Close()
		t.Fatalf("client: %v", err)
	}
	return e, client
}

func mustParseURL(raw string) url.URL {
	parsed, err := url.Parse(raw)
	if err != nil {
		panic(err)
	}
	return *parsed
}
