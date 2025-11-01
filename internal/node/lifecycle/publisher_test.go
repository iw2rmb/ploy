//go:build legacy
// +build legacy

package lifecycle

import (
	"context"
	"encoding/json"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/server/v3/embed"
)

func TestPublisherWritesCapacityAndUpdatesCache(t *testing.T) {
	t.Helper()

	etcd, client := startTestEtcd(t)
	defer etcd.Close()

	cache := NewCache()
	collector := NewCollector(Options{
		Role:   "worker",
		NodeID: "worker-123",
		Docker: staticChecker{status: ComponentStatus{State: stateOK, CheckedAt: time.Now()}},
	})
	collector.resourcesFunc = func(context.Context) (resourceSnapshot, error) {
		return resourceSnapshot{
			CPUTotalMilli: 6000,
			CPUFreeMilli:  3000,
			CPULoad1:      3,
			MemoryTotalMB: 8192,
			MemoryFreeMB:  4096,
			DiskTotalMB:   102400,
			DiskFreeMB:    51200,
		}, nil
	}

	publisher, err := NewPublisher(PublisherOptions{
		Client:    client,
		Collector: collector,
		Cache:     cache,
		NodeID:    "worker-123",
	})
	if err != nil {
		t.Fatalf("NewPublisher error: %v", err)
	}
	defer func() {
		if err := publisher.Close(context.Background()); err != nil {
			t.Fatalf("publisher close: %v", err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := publisher.Publish(ctx); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	resp, err := client.Get(ctx, "nodes/worker-123/capacity")
	if err != nil {
		t.Fatalf("etcd get: %v", err)
	}
	if len(resp.Kvs) != 1 {
		t.Fatalf("expected one capacity record, got %d", len(resp.Kvs))
	}
	var payload map[string]any
	if err := json.Unmarshal(resp.Kvs[0].Value, &payload); err != nil {
		t.Fatalf("unmarshal capacity: %v", err)
	}
	if payload["cpu_free"].(float64) != 3000 {
		t.Fatalf("cpu_free mismatch: %v", payload["cpu_free"])
	}
	if payload["revision"].(float64) != 1 {
		t.Fatalf("revision mismatch: %v", payload["revision"])
	}

	status, ok := cache.LatestStatus()
	if !ok {
		t.Fatalf("expected cached status")
	}
	if status["node_id"] != "worker-123" {
		t.Fatalf("cached status node_id mismatch: %v", status["node_id"])
	}

	if err := publisher.Publish(ctx); err != nil {
		t.Fatalf("Publish() second call error = %v", err)
	}
	resp, err = client.Get(ctx, "nodes/worker-123/capacity")
	if err != nil {
		t.Fatalf("etcd get 2: %v", err)
	}
	if len(resp.Kvs) != 1 {
		t.Fatalf("expected one capacity record after second publish, got %d", len(resp.Kvs))
	}
	if err := json.Unmarshal(resp.Kvs[0].Value, &payload); err != nil {
		t.Fatalf("unmarshal capacity 2: %v", err)
	}
	if payload["revision"].(float64) != 2 {
		t.Fatalf("expected revision 2, got %v", payload["revision"])
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
	cfg.Name = "lifecycle-test"
	cfg.InitialCluster = cfg.Name + "=" + peerURL.String()
	cfg.ClusterState = embed.ClusterStateFlagNew
	cfg.InitialClusterToken = "lifecycle"
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
