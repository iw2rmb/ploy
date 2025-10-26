package artifacts_test

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/server/v3/embed"

	"github.com/iw2rmb/ploy/internal/controlplane/artifacts"
)

func TestStoreCreateGetAndList(t *testing.T) {
	t.Parallel()

	etcd, client := startTestEtcd(t)
	t.Cleanup(func() {
		etcd.Close()
		client.Close()
	})

	clock := fixedClock(time.Date(2025, 10, 26, 12, 0, 0, 0, time.UTC))
	store, err := artifacts.NewStore(client, artifacts.StoreOptions{Clock: clock})
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	ctx := context.Background()
	input := artifacts.Metadata{
		ID:     "artifact-alpha",
		CID:    "bafyalpha",
		Digest: "sha256:alpha",
		Size:   1024,
		JobID:  "job-1",
		Stage:  "plan",
		Kind:   "repo",
		NodeID: "node-a",
		Name:   "plan.tar.zst",
		TTL:    "24h",
	}

	created, err := store.Create(ctx, input)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
		t.Fatalf("expected timestamps to be set: %#v", created)
	}
	if created.CreatedAt != clock() {
		t.Fatalf("expected created_at to use clock, got %s", created.CreatedAt)
	}
	if created.ExpiresAt.Sub(created.CreatedAt) != 24*time.Hour {
		t.Fatalf("expected expires_at 24h ahead, got %s", created.ExpiresAt)
	}

	fetched, err := store.Get(ctx, input.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if fetched.CID != input.CID || fetched.Digest != input.Digest {
		t.Fatalf("unexpected fetched metadata: %#v", fetched)
	}
	if fetched.TTL != input.TTL {
		t.Fatalf("expected ttl %q, got %q", input.TTL, fetched.TTL)
	}

	// Second artifact for another job/stage.
	_, err = store.Create(ctx, artifacts.Metadata{
		ID:     "artifact-beta",
		CID:    "bafybeta",
		Digest: "sha256:beta",
		Size:   2048,
		JobID:  "job-2",
		Stage:  "verify",
		Kind:   "report",
		NodeID: "node-b",
	})
	if err != nil {
		t.Fatalf("Create second: %v", err)
	}

	list, err := store.List(ctx, artifacts.ListOptions{JobID: "job-1", Limit: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list.Artifacts) != 1 || list.Artifacts[0].ID != "artifact-alpha" {
		t.Fatalf("unexpected list contents: %#v", list.Artifacts)
	}

	stageList, err := store.List(ctx, artifacts.ListOptions{JobID: "job-2", Stage: "verify", Limit: 10})
	if err != nil {
		t.Fatalf("List stage: %v", err)
	}
	if len(stageList.Artifacts) != 1 || stageList.Artifacts[0].ID != "artifact-beta" {
		t.Fatalf("unexpected stage list: %#v", stageList.Artifacts)
	}
}

func TestStoreCursorAndDelete(t *testing.T) {
	t.Parallel()

	etcd, client := startTestEtcd(t)
	t.Cleanup(func() {
		etcd.Close()
		client.Close()
	})

	store, err := artifacts.NewStore(client, artifacts.StoreOptions{Clock: fixedClock(time.Date(2025, 10, 26, 13, 0, 0, 0, time.UTC))})
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		id := fmt.Sprintf("artifact-%d", i)
		if _, err := store.Create(ctx, artifacts.Metadata{
			ID:     id,
			CID:    fmt.Sprintf("bafy%d", i),
			Digest: fmt.Sprintf("sha256:%d", i),
			Size:   int64(100 + i),
			JobID:  "job-cursor",
			Stage:  "results",
		}); err != nil {
			t.Fatalf("Create %s: %v", id, err)
		}
	}

	firstPage, err := store.List(ctx, artifacts.ListOptions{JobID: "job-cursor", Limit: 2})
	if err != nil {
		t.Fatalf("List first page: %v", err)
	}
	if len(firstPage.Artifacts) != 2 {
		t.Fatalf("expected 2 artifacts, got %d", len(firstPage.Artifacts))
	}
	if firstPage.NextCursor == "" {
		t.Fatalf("expected next cursor")
	}

	secondPage, err := store.List(ctx, artifacts.ListOptions{JobID: "job-cursor", Limit: 2, Cursor: firstPage.NextCursor})
	if err != nil {
		t.Fatalf("List second page: %v", err)
	}
	if len(secondPage.Artifacts) != 1 {
		t.Fatalf("expected 1 artifact on second page, got %d", len(secondPage.Artifacts))
	}
	if secondPage.NextCursor != "" {
		t.Fatalf("expected no next cursor on final page, got %q", secondPage.NextCursor)
	}

	deleted, err := store.Delete(ctx, firstPage.Artifacts[0].ID)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if !deleted.Deleted {
		t.Fatalf("expected deleted flag set")
	}

	_, err = store.Get(ctx, deleted.ID)
	if !errors.Is(err, artifacts.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}

	remaining, err := store.List(ctx, artifacts.ListOptions{JobID: "job-cursor", Limit: 10})
	if err != nil {
		t.Fatalf("List after delete: %v", err)
	}
	if len(remaining.Artifacts) != 2 {
		t.Fatalf("expected 2 remaining artifacts, got %d", len(remaining.Artifacts))
	}
}

func startTestEtcd(t *testing.T) (*embed.Etcd, *clientv3.Client) {
	t.Helper()
	dir := t.TempDir()
	cfg := embed.NewConfig()
	cfg.Dir = dir
	clientURL := mustParseURL("http://127.0.0.1:0")
	peerURL := mustParseURL("http://127.0.0.1:0")
	cfg.ListenClientUrls = []url.URL{clientURL}
	cfg.ListenPeerUrls = []url.URL{peerURL}
	cfg.AdvertiseClientUrls = []url.URL{clientURL}
	cfg.AdvertisePeerUrls = []url.URL{peerURL}
	cfg.Name = "artifacts-store-test"
	cfg.InitialCluster = fmt.Sprintf("%s=%s", cfg.Name, peerURL.String())
	cfg.ClusterState = embed.ClusterStateFlagNew
	cfg.InitialClusterToken = "artifacts-store"
	cfg.LogLevel = "panic"
	cfg.Logger = "zap"
	cfg.LogOutputs = []string{filepath.Join(dir, "etcd.log")}

	e, err := embed.StartEtcd(cfg)
	if err != nil {
		t.Fatalf("start etcd: %v", err)
	}
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
		t.Fatalf("etcd client: %v", err)
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

func fixedClock(t time.Time) func() time.Time {
	return func() time.Time { return t }
}
