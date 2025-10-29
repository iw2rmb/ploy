package hydration

import (
	"context"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/server/v3/embed"

	"github.com/iw2rmb/ploy/internal/controlplane/scheduler"
)

// TestIndexUpsertAndLookup ensures hydration snapshot entries are persisted and returned with ticket bindings.
func TestIndexUpsertAndLookup(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	e, client := newTestEtcd(t)
	defer e.Close()
	defer client.Close()

	clock := func() time.Time { return time.Date(2025, 10, 28, 20, 45, 0, 0, time.UTC) }
	index, err := NewIndex(client, IndexOptions{
		Prefix: "hydration/index/",
		Clock:  clock,
	})
	if err != nil {
		t.Fatalf("new index: %v", err)
	}

	record := SnapshotRecord{
		RepoURL:  "https://git.example.com/org/repo.git",
		Revision: "abc123",
		TicketID: "mod-123",
		Bundle: scheduler.BundleRecord{
			CID:       "bafy-snapshot",
			Digest:    "sha256:deadbeef",
			Size:      4096,
			TTL:       scheduler.HydrationSnapshotTTL,
			ExpiresAt: clock().Add(24 * time.Hour).UTC().Format(time.RFC3339Nano),
			Retained:  true,
		},
		Replication: ReplicationPolicy{
			Min: 2,
			Max: 3,
		},
		Sharing: SharingPolicy{
			Enabled: true,
		},
	}

	if _, err := index.UpsertSnapshot(ctx, record); err != nil {
		t.Fatalf("upsert snapshot: %v", err)
	}

	entry, ok, err := index.LookupSnapshot(ctx, LookupRequest{
		RepoURL:  record.RepoURL,
		Revision: record.Revision,
	})
	if err != nil {
		t.Fatalf("lookup snapshot: %v", err)
	}
	if !ok {
		t.Fatalf("expected snapshot entry to exist")
	}
	if entry.Bundle.CID != record.Bundle.CID {
		t.Fatalf("unexpected bundle cid %q", entry.Bundle.CID)
	}
	if entry.Replication.Min != record.Replication.Min || entry.Replication.Max != record.Replication.Max {
		t.Fatalf("unexpected replication policy: %#v", entry.Replication)
	}
	if !entry.Sharing.Enabled {
		t.Fatalf("expected sharing enabled")
	}
	if _, exists := entry.Tickets["mod-123"]; !exists {
		t.Fatalf("expected ticket binding recorded")
	}
	if entry.ExpiresAt.IsZero() {
		t.Fatalf("expected expiry timestamp recorded")
	}
}

// TestIndexListSnapshots ensures snapshot enumeration returns stored entries.
func TestIndexListSnapshots(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	e, client := newTestEtcd(t)
	defer e.Close()
	defer client.Close()

	clockTime := time.Date(2025, 10, 28, 21, 10, 0, 0, time.UTC)
	tick := 0
	clock := func() time.Time {
		defer func() { tick++ }()
		return clockTime.Add(time.Duration(tick) * time.Minute)
	}

	index, err := NewIndex(client, IndexOptions{Clock: clock})
	if err != nil {
		t.Fatalf("new index: %v", err)
	}

	records := []SnapshotRecord{
		{
			RepoURL:  "https://git.example.com/org/repo.git",
			Revision: "abc123",
			TicketID: "mod-a",
			Bundle: scheduler.BundleRecord{
				CID:       "cid-a",
				TTL:       scheduler.HydrationSnapshotTTL,
				ExpiresAt: clock().Add(24 * time.Hour).Format(time.RFC3339Nano),
			},
			Replication: ReplicationPolicy{Min: 1, Max: 3},
		},
		{
			RepoURL:  "https://git.example.com/org/repo.git",
			Revision: "def456",
			TicketID: "mod-b",
			Bundle: scheduler.BundleRecord{
				CID:       "cid-b",
				TTL:       scheduler.HydrationSnapshotTTL,
				ExpiresAt: clock().Add(24 * time.Hour).Format(time.RFC3339Nano),
			},
			Replication: ReplicationPolicy{Min: 1, Max: 2},
		},
	}

	for _, record := range records {
		if _, err := index.UpsertSnapshot(ctx, record); err != nil {
			t.Fatalf("upsert snapshot: %v", err)
		}
	}

	entries, err := index.ListSnapshots(ctx)
	if err != nil {
		t.Fatalf("list snapshots: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 snapshots, got %d", len(entries))
	}
	if !entries[0].UpdatedAt.Before(entries[1].UpdatedAt) {
		t.Fatalf("expected entries ordered by updated time")
	}
}

// TestIndexUpdateAndDelete ensures replication updates and deletions persist correctly.
func TestIndexUpdateAndDelete(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	e, client := newTestEtcd(t)
	defer e.Close()
	defer client.Close()

	clock := func() time.Time { return time.Date(2025, 10, 28, 21, 40, 0, 0, time.UTC) }
	index, err := NewIndex(client, IndexOptions{Clock: clock})
	if err != nil {
		t.Fatalf("new index: %v", err)
	}

	record := SnapshotRecord{
		RepoURL:  "https://git.example.com/org/repo.git",
		Revision: "abc123",
		TicketID: "ticket-1",
		Bundle: scheduler.BundleRecord{
			CID:       "cid-1",
			TTL:       scheduler.HydrationSnapshotTTL,
			ExpiresAt: clock().Add(24 * time.Hour).Format(time.RFC3339Nano),
		},
		Replication: ReplicationPolicy{Min: 1, Max: 2},
	}
	entry, err := index.UpsertSnapshot(ctx, record)
	if err != nil {
		t.Fatalf("upsert snapshot: %v", err)
	}

	updated, err := index.UpdateReplication(ctx, entry.Fingerprint, ReplicationPolicy{Min: 2, Max: 3})
	if err != nil {
		t.Fatalf("update replication: %v", err)
	}
	if updated.Replication.Max != 3 {
		t.Fatalf("expected replication max 3, got %d", updated.Replication.Max)
	}

	if err := index.DeleteSnapshot(ctx, entry.Fingerprint); err != nil {
		t.Fatalf("delete snapshot: %v", err)
	}
	if _, ok, err := index.LookupSnapshot(ctx, LookupRequest{RepoURL: record.RepoURL, Revision: record.Revision}); err != nil || ok {
		t.Fatalf("expected snapshot removed, ok=%t err=%v", ok, err)
	}
	if _, ok, err := index.LookupTicket(ctx, record.TicketID); err != nil || ok {
		t.Fatalf("expected ticket binding removed, ok=%t err=%v", ok, err)
	}
}

// newTestEtcd spins up an embedded etcd server for hydration tests.
func newTestEtcd(t *testing.T) (*embed.Etcd, *clientv3.Client) {
	t.Helper()
	dir := t.TempDir()
	cfg := embed.NewConfig()
	cfg.Dir = dir

	clientURL := mustParseURL(t, "http://127.0.0.1:0")
	peerURL := mustParseURL(t, "http://127.0.0.1:0")
	cfg.ListenClientUrls = []url.URL{clientURL}
	cfg.ListenPeerUrls = []url.URL{peerURL}
	cfg.AdvertiseClientUrls = []url.URL{clientURL}
	cfg.AdvertisePeerUrls = []url.URL{peerURL}
	cfg.Name = "hydration-test"
	cfg.InitialCluster = cfg.Name + "=" + peerURL.String()
	cfg.ClusterState = embed.ClusterStateFlagNew
	cfg.Logger = "zap"
	cfg.LogOutputs = []string{filepath.Join(dir, "etcd.log")}

	e, err := embed.StartEtcd(cfg)
	if err != nil {
		t.Fatalf("start etcd: %v", err)
	}
	select {
	case <-e.Server.ReadyNotify():
	case <-time.After(15 * time.Second):
		t.Fatalf("timed out waiting for etcd ready")
	}

	client, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{e.Clients[0].Addr().String()},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		e.Close()
		t.Fatalf("new etcd client: %v", err)
	}
	return e, client
}

func mustParseURL(t *testing.T, raw string) url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	return *parsed
}
