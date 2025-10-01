package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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
			APIEndpoint:   "https://grid.dev",
			JetStreamURLs: []string{"nats://grid.dev:4222", "nats://grid.dev:5222"},
			IPFSGateway:   "https://ipfs.grid.dev",
			Features: map[string]string{
				"workspace_api":    "enabled",
				"snapshot_manager": "",
			},
			Version: "2025.9.29",
		}, nil
	}
	t.Cleanup(func() { fetchClusterInfoFn = fetchClusterInfo })

	cfg, err := resolveIntegrationConfig(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.JetStreamURL != "nats://grid.dev:4222" {
		t.Fatalf("unexpected primary jetstream url: %q", cfg.JetStreamURL)
	}
	if len(cfg.JetStreamURLs) != 2 {
		t.Fatalf("unexpected jetstream route count: %d", len(cfg.JetStreamURLs))
	}
	if cfg.JetStreamURLs[0] != "nats://grid.dev:4222" || cfg.JetStreamURLs[1] != "nats://grid.dev:5222" {
		t.Fatalf("unexpected jetstream routes: %+v", cfg.JetStreamURLs)
	}
	if cfg.IPFSGateway != "https://ipfs.grid.dev" {
		t.Fatalf("unexpected ipfs gateway: %q", cfg.IPFSGateway)
	}
	if cfg.APIEndpoint != "https://grid.dev" {
		t.Fatalf("unexpected api endpoint: %q", cfg.APIEndpoint)
	}
	if cfg.Version != "2025.9.29" {
		t.Fatalf("unexpected version: %q", cfg.Version)
	}
	if cfg.Features["workspace_api"] != "enabled" {
		t.Fatalf("expected workspace_api to be enabled, got %q", cfg.Features["workspace_api"])
	}
	if !cfg.FeatureEnabled("workspace_api") {
		t.Fatal("expected workspace_api feature to be enabled")
	}
	if cfg.FeatureEnabled("snapshot_manager") {
		t.Fatal("expected snapshot_manager feature to be disabled")
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

func TestResolveIntegrationConfigReturnsStubWhenDiscoveryFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	t.Setenv("GRID_ENDPOINT", srv.URL)
	resetDiscoveryState()
	fetchClusterInfoFn = fetchClusterInfo
	t.Cleanup(func() { fetchClusterInfoFn = fetchClusterInfo })

	cfg, err := resolveIntegrationConfig(context.Background())
	if err == nil {
		t.Fatalf("expected error from discovery failure")
	}
	if cfg.JetStreamURL != "" {
		t.Fatalf("expected empty jetstream url, got %q", cfg.JetStreamURL)
	}
	if len(cfg.JetStreamURLs) != 0 {
		t.Fatalf("expected no discovery jetstream routes, got %d", len(cfg.JetStreamURLs))
	}
	if cfg.IPFSGateway != "" {
		t.Fatalf("expected empty ipfs gateway, got %q", cfg.IPFSGateway)
	}
	expectedEndpoint := strings.TrimRight(srv.URL, "/")
	if cfg.APIEndpoint != expectedEndpoint {
		t.Fatalf("expected api endpoint %q, got %q", expectedEndpoint, cfg.APIEndpoint)
	}
	if cfg.Version != "" {
		t.Fatalf("expected empty version, got %q", cfg.Version)
	}
	if len(cfg.Features) != 0 {
		t.Fatalf("expected no features, got %+v", cfg.Features)
	}
	if cfg.FeatureEnabled("workspace_api") {
		t.Fatal("expected workspace_api feature to be disabled by fallback config")
	}
}

func TestIntegrationConfigFeatureEnabled(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		cfg     integrationConfig
		feature string
		want    bool
	}{
		{
			name:    "enabled lower case",
			cfg:     integrationConfig{Features: map[string]string{"scheduler_control": "enabled"}},
			feature: "scheduler_control",
			want:    true,
		},
		{
			name:    "enabled upper case",
			cfg:     integrationConfig{Features: map[string]string{"scheduler_control": "ENABLED"}},
			feature: "scheduler_control",
			want:    true,
		},
		{
			name:    "enabled with whitespace",
			cfg:     integrationConfig{Features: map[string]string{"scheduler_control": " enabled "}},
			feature: "scheduler_control",
			want:    true,
		},
		{
			name:    "disabled empty string",
			cfg:     integrationConfig{Features: map[string]string{"scheduler_control": ""}},
			feature: "scheduler_control",
			want:    false,
		},
		{
			name:    "missing feature",
			cfg:     integrationConfig{Features: map[string]string{"workspace_api": "enabled"}},
			feature: "scheduler_control",
			want:    false,
		},
		{
			name:    "nil features map",
			cfg:     integrationConfig{},
			feature: "scheduler_control",
			want:    false,
		},
	}

	for _, tt := range cases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.cfg.FeatureEnabled(tt.feature)
			if got != tt.want {
				t.Fatalf("FeatureEnabled(%q) = %v, want %v", tt.feature, got, tt.want)
			}
		})
	}
}

func resetDiscoveryState() {
	discoveryCache = newClusterInfoCache()
}
