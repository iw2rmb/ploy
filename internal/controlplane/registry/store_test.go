package registry_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/server/v3/embed"

	"github.com/iw2rmb/ploy/internal/controlplane/registry"
)

func TestRegistryStoreBlobLifecycle(t *testing.T) {
	t.Parallel()

	etcd, client := startRegistryTestEtcd(t)
	t.Cleanup(func() {
		etcd.Close()
		client.Close()
	})

	store, err := registry.NewStore(client, registry.StoreOptions{Clock: fixedClock(time.Date(2025, 10, 26, 12, 0, 0, 0, time.UTC))})
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	ctx := context.Background()
	blob := registry.BlobDocument{
		Repo:      "acme/registry",
		Digest:    "sha256:layerdeadbeef",
		MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
		Size:      4096,
		CID:       "bafy-layer",
	}

	created, err := store.PutBlob(ctx, blob)
	if err != nil {
		t.Fatalf("PutBlob: %v", err)
	}
	if created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
		t.Fatalf("expected timestamps to be set: %#v", created)
	}
	if created.Status != registry.BlobStatusAvailable {
		t.Fatalf("expected status available, got %q", created.Status)
	}

	fetched, err := store.GetBlob(ctx, blob.Repo, blob.Digest)
	if err != nil {
		t.Fatalf("GetBlob: %v", err)
	}
	if fetched.CID != blob.CID || fetched.MediaType != blob.MediaType {
		t.Fatalf("unexpected fetched blob: %#v", fetched)
	}

	deleted, err := store.DeleteBlob(ctx, blob.Repo, blob.Digest)
	if err != nil {
		t.Fatalf("DeleteBlob: %v", err)
	}
	if deleted.Status != registry.BlobStatusDeleted {
		t.Fatalf("expected deleted status, got %q", deleted.Status)
	}
	if _, err := store.GetBlob(ctx, blob.Repo, blob.Digest); err == nil {
		t.Fatalf("expected error after delete")
	}
}

func TestRegistryStoreManifestRequiresBlobs(t *testing.T) {
	t.Parallel()

	etcd, client := startRegistryTestEtcd(t)
	t.Cleanup(func() {
		etcd.Close()
		client.Close()
	})

	store, err := registry.NewStore(client, registry.StoreOptions{})
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	ctx := context.Background()
	manifest := registry.ManifestDocument{
		Repo:         "acme/registry",
		Digest:       "sha256:manifest01",
		MediaType:    "application/vnd.oci.image.manifest.v1+json",
		Payload:      []byte(`{"schemaVersion":2}`),
		ConfigDigest: "sha256:missing-config",
		LayerDigests: []string{"sha256:missing-layer"},
	}

	if _, err := store.PutManifest(ctx, manifest, "latest"); err == nil {
		t.Fatalf("expected error when blobs missing")
	}
}

func TestRegistryStoreManifestAndTags(t *testing.T) {
	t.Parallel()

	etcd, client := startRegistryTestEtcd(t)
	t.Cleanup(func() {
		etcd.Close()
		client.Close()
	})

	clock := fixedClock(time.Date(2025, 10, 26, 13, 30, 0, 0, time.UTC))
	store, err := registry.NewStore(client, registry.StoreOptions{Clock: clock})
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	ctx := context.Background()
	configDigest := "sha256:configdeadbeef"
	layerDigest := "sha256:layercafebabe"
	_, err = store.PutBlob(ctx, registry.BlobDocument{
		Repo:      "acme/registry",
		Digest:    configDigest,
		MediaType: "application/vnd.oci.image.config.v1+json",
		Size:      256,
		CID:       "bafy-config",
	})
	if err != nil {
		t.Fatalf("PutBlob config: %v", err)
	}
	_, err = store.PutBlob(ctx, registry.BlobDocument{
		Repo:      "acme/registry",
		Digest:    layerDigest,
		MediaType: "application/vnd.oci.image.layer.v1.tar",
		Size:      8192,
		CID:       "bafy-layer",
	})
	if err != nil {
		t.Fatalf("PutBlob layer: %v", err)
	}

	payloadBytes, err := json.Marshal(map[string]any{
		"schemaVersion": 2,
		"config": map[string]any{
			"mediaType": "application/vnd.oci.image.config.v1+json",
			"digest":    configDigest,
			"size":      256,
		},
		"layers": []map[string]any{
			{
				"mediaType": "application/vnd.oci.image.layer.v1.tar",
				"digest":    layerDigest,
				"size":      8192,
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}

	manifest := registry.ManifestDocument{
		Repo:         "acme/registry",
		Digest:       "sha256:manifestdeadbeef",
		MediaType:    "application/vnd.oci.image.manifest.v1+json",
		Payload:      payloadBytes,
		ConfigDigest: configDigest,
		LayerDigests: []string{layerDigest},
		Size:         int64(len(payloadBytes)),
	}

	stored, err := store.PutManifest(ctx, manifest, "latest")
	if err != nil {
		t.Fatalf("PutManifest: %v", err)
	}
	if stored.CreatedAt != clock() {
		t.Fatalf("expected manifest timestamps to use clock")
	}

	resolved, err := store.ResolveManifest(ctx, manifest.Repo, "latest")
	if err != nil {
		t.Fatalf("ResolveManifest: %v", err)
	}
	if string(resolved.Payload) != string(payloadBytes) {
		t.Fatalf("unexpected manifest payload")
	}

	tags, err := store.ListTags(ctx, manifest.Repo)
	if err != nil {
		t.Fatalf("ListTags: %v", err)
	}
	if len(tags) != 1 || tags[0].Name != "latest" || tags[0].Digest != manifest.Digest {
		t.Fatalf("unexpected tags: %#v", tags)
	}

	if err := store.DeleteManifest(ctx, manifest.Repo, manifest.Digest); err != nil {
		t.Fatalf("DeleteManifest: %v", err)
	}
	if _, err := store.GetManifest(ctx, manifest.Repo, manifest.Digest); err == nil {
		t.Fatalf("expected error after manifest delete")
	}
	tags, err = store.ListTags(ctx, manifest.Repo)
	if err != nil {
		t.Fatalf("ListTags after delete: %v", err)
	}
	if len(tags) != 0 {
		t.Fatalf("expected tags cleared after manifest delete, got %#v", tags)
	}
}

func startRegistryTestEtcd(t *testing.T) (*embed.Etcd, *clientv3.Client) {
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
	cfg.Name = "registry-store-test"
	cfg.InitialCluster = fmt.Sprintf("%s=%s", cfg.Name, peerURL.String())
	cfg.ClusterState = embed.ClusterStateFlagNew
	cfg.InitialClusterToken = "registry-store"
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
