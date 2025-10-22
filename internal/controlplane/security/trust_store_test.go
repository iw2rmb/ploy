package security

import (
	"context"
	"fmt"
	"net/url"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/server/v3/embed"
)

func TestTrustStoreUpdateAndFetch(t *testing.T) {
	ctx := context.Background()
	etcd, client := newTestEtcd(t)
	defer etcd.Close()
	defer func() {
		_ = client.Close()
	}()

	store, err := NewTrustStore(client, "cluster-alpha")
	if err != nil {
		t.Fatalf("NewTrustStore: %v", err)
	}

	if _, _, err := store.Current(ctx); err == nil {
		t.Fatalf("expected Current to error when bundle not set")
	}

	first := TrustBundle{
		Version:      "v1",
		UpdatedAt:    time.Now().UTC(),
		CABundlePEM:  "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n",
		CABundleHash: "hash-v1",
	}
	if err := store.Update(ctx, first); err != nil {
		t.Fatalf("Update v1: %v", err)
	}

	retrieved, rev1, err := store.Current(ctx)
	if err != nil {
		t.Fatalf("Current v1: %v", err)
	}
	if rev1 == 0 {
		t.Fatalf("expected revision recorded")
	}
	if retrieved.Version != "v1" {
		t.Fatalf("expected version v1, got %s", retrieved.Version)
	}
	if retrieved.CABundleHash == "" {
		t.Fatalf("expected bundle hash populated")
	}

	second := TrustBundle{
		Version:      "v2",
		UpdatedAt:    time.Now().UTC().Add(5 * time.Minute),
		CABundlePEM:  "-----BEGIN CERTIFICATE-----\nMIIC\n-----END CERTIFICATE-----\n",
		CABundleHash: "hash-v2",
	}
	if err := store.Update(ctx, second); err != nil {
		t.Fatalf("Update v2: %v", err)
	}
	retrieved, rev2, err := store.Current(ctx)
	if err != nil {
		t.Fatalf("Current v2: %v", err)
	}
	if rev2 <= rev1 {
		t.Fatalf("expected revision to increase after update")
	}
	if retrieved.Version != "v2" {
		t.Fatalf("expected version v2, got %s", retrieved.Version)
	}
	if retrieved.CABundlePEM != second.CABundlePEM {
		t.Fatalf("expected stored bundle to match input")
	}
}

func newTestEtcd(t *testing.T) (*embed.Etcd, *clientv3.Client) {
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
	cfg.Name = "default"
	cfg.InitialCluster = fmt.Sprintf("%s=%s", cfg.Name, peerURL.String())
	cfg.ClusterState = embed.ClusterStateFlagNew
	cfg.InitialClusterToken = "trust-store-test"
	cfg.LogLevel = "panic"
	cfg.Logger = "zap"
	cfg.LogOutputs = []string{fmt.Sprintf("%s/etcd.log", dir)}

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
