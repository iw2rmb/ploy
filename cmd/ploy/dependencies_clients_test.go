package main

import (
	"context"
	"testing"

	gridclient "github.com/iw2rmb/grid/sdk/gridclient/go"
)

func TestAcquireGridClientUsesGridEnvFallback(t *testing.T) {
	t.Setenv(gridIDEnv, "")
	t.Setenv(gridAPIKeyEnv, "")
	t.Setenv(gridIDFallbackEnv, "fallback-grid")
	t.Setenv(gridAPIKeyFallbackEnv, "fallback-secret")
	stateDir := t.TempDir()
	t.Setenv(gridClientStateEnv, stateDir)

	prevNew := newGridClient
	resetGridClientState()
	var captured gridclient.Config
	stub := newStubGridClient(gridclient.Status{})
	newGridClient = func(ctx context.Context, cfg gridclient.Config) (gridClientAPI, error) {
		captured = cfg
		return stub, nil
	}
	t.Cleanup(func() {
		newGridClient = prevNew
		resetGridClientState()
	})

	client, err := acquireGridClient(context.Background())
	if err != nil {
		t.Fatalf("expected fallback credentials to be accepted, got error: %v", err)
	}
	if client == nil {
		t.Fatal("expected grid client instance, got nil")
	}
	if captured.GridID != "fallback-grid" {
		t.Fatalf("expected grid id %q, got %q", "fallback-grid", captured.GridID)
	}
	if captured.APIKey != "fallback-secret" {
		t.Fatalf("expected api key %q, got %q", "fallback-secret", captured.APIKey)
	}
	if captured.StateDir != stateDir {
		t.Fatalf("expected state dir %q, got %q", stateDir, captured.StateDir)
	}
}
