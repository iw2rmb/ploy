package main

import (
	"context"
	"strings"
	"testing"

	discovery "github.com/iw2rmb/grid/sdk/discovery/go"
	gridclient "github.com/iw2rmb/grid/sdk/gridclient/go"
)

func TestResolveIntegrationConfigFromGridClient(t *testing.T) {
	t.Setenv(gridIDEnv, "grid-dev")
	t.Setenv(gridAPIKeyEnv, "secret")
	t.Setenv(gridClientStateEnv, t.TempDir())
	withGridClientStub(t, newStubGridClient(gridclient.Status{
		Beacon: gridclient.BeaconStatus{
			APIEndpoint:      "https://api.grid.dev",
			WorkflowEndpoint: "https://workflow.grid.dev",
		},
		Discovery: discovery.ClusterInfo{
			APIEndpoint:   "https://api.grid.dev",
			JetStreamURLs: []string{" nats://grid.dev:4222 ", "nats://grid.dev:5222"},
			IPFSGateway:   " https://ipfs.grid.dev ",
			Features: map[string]string{
				"workspace_api":    "enabled",
				"snapshot_manager": "",
			},
			Version: "2025.9.29",
		},
	}))

	cfg, err := resolveIntegrationConfig(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.APIEndpoint != "https://api.grid.dev" {
		t.Fatalf("unexpected api endpoint: %q", cfg.APIEndpoint)
	}
	if cfg.JetStreamURL != "nats://grid.dev:4222" {
		t.Fatalf("unexpected primary jetstream url: %q", cfg.JetStreamURL)
	}
	if len(cfg.JetStreamURLs) != 2 {
		t.Fatalf("unexpected jetstream route count: %d", len(cfg.JetStreamURLs))
	}
	if cfg.JetStreamURLs[1] != "nats://grid.dev:5222" {
		t.Fatalf("unexpected jetstream routes: %+v", cfg.JetStreamURLs)
	}
	if cfg.IPFSGateway != "https://ipfs.grid.dev" {
		t.Fatalf("unexpected ipfs gateway: %q", cfg.IPFSGateway)
	}
	if cfg.Version != "2025.9.29" {
		t.Fatalf("unexpected version: %q", cfg.Version)
	}
	if !cfg.FeatureEnabled("workspace_api") {
		t.Fatal("expected workspace_api feature enabled")
	}
	if cfg.FeatureEnabled("snapshot_manager") {
		t.Fatal("expected snapshot_manager feature disabled")
	}
}

func TestResolveIntegrationConfigDisabledWithoutCredentials(t *testing.T) {
	t.Setenv(gridIDEnv, "")
	t.Setenv(gridAPIKeyEnv, "")
	cfg, err := resolveIntegrationConfig(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.APIEndpoint != "" {
		t.Fatalf("expected empty api endpoint, got %q", cfg.APIEndpoint)
	}
	if len(cfg.Features) != 0 {
		t.Fatalf("expected empty features map, got %+v", cfg.Features)
	}
}

func TestResolveIntegrationConfigPropagatesErrors(t *testing.T) {
	t.Setenv(gridIDEnv, "grid-dev")
	t.Setenv(gridAPIKeyEnv, "secret")
	t.Setenv(gridClientStateEnv, t.TempDir())

	errSentinel := context.DeadlineExceeded
	prevNew := newGridClient
	resetGridClientState()
	newGridClient = func(context.Context, gridclient.Config) (gridClientAPI, error) {
		return nil, errSentinel
	}
	t.Cleanup(func() {
		newGridClient = prevNew
		resetGridClientState()
	})

	_, err := resolveIntegrationConfig(context.Background())
	if err == nil || !strings.Contains(err.Error(), errSentinel.Error()) {
		t.Fatalf("expected error %v, got %v", errSentinel, err)
	}
}
