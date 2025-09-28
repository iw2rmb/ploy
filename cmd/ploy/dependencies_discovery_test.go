package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestResolveIntegrationConfigUsesDiscovery(t *testing.T) {
	t.Setenv("GRID_ENDPOINT", "https://grid.dev")
	resetDiscoveryState()

	var calls int32
	fetchClusterInfoFn = func(ctx context.Context, endpoint string) (clusterInfo, error) {
		atomic.AddInt32(&calls, 1)
		return clusterInfo{
			JetStreamURL: "nats://grid.dev:4222",
			IPFSGateway:  "https://ipfs.grid.dev",
		}, nil
	}
	t.Cleanup(func() { fetchClusterInfoFn = fetchClusterInfo })

	cfg, err := resolveIntegrationConfig(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.JetStreamURL != "nats://grid.dev:4222" {
		t.Fatalf("unexpected jetstream url: %q", cfg.JetStreamURL)
	}
	if cfg.IPFSGateway != "https://ipfs.grid.dev" {
		t.Fatalf("unexpected ipfs gateway: %q", cfg.IPFSGateway)
	}

	// ensure cache prevents additional fetches
	_, err = resolveIntegrationConfig(context.Background())
	if err != nil {
		t.Fatalf("unexpected error on cached read: %v", err)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("expected single discovery call, got %d", calls)
	}
}

func TestResolveIntegrationConfigFallsBackToEnv(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	t.Setenv("GRID_ENDPOINT", srv.URL)
	t.Setenv("JETSTREAM_URL", "nats://env:4222")
	t.Setenv("IPFS_GATEWAY", "https://ipfs.env")
	resetDiscoveryState()
	fetchClusterInfoFn = fetchClusterInfo
	t.Cleanup(func() { fetchClusterInfoFn = fetchClusterInfo })

	cfg, err := resolveIntegrationConfig(context.Background())
	if err == nil {
		t.Fatalf("expected error from discovery failure")
	}
	if cfg.JetStreamURL != "nats://env:4222" {
		t.Fatalf("expected fallback jetstream url, got %q", cfg.JetStreamURL)
	}
	if cfg.IPFSGateway != "https://ipfs.env" {
		t.Fatalf("expected fallback ipfs gateway, got %q", cfg.IPFSGateway)
	}
}

func resetDiscoveryState() {
	discoveryCache = newClusterInfoCache()
}
