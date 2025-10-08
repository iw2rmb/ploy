package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	helper "github.com/iw2rmb/grid/sdk/workflowrpc/helper"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/grid"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
)

// defaultEventsFactory builds an events client, preferring JetStream when configured.
func defaultEventsFactory(tenant string) (runner.EventsClient, error) {
	trimmedTenant := strings.TrimSpace(tenant)
	if trimmedTenant == "" {
		return nil, fmt.Errorf("tenant is required for events client")
	}
	cfg, _ := resolveIntegrationConfig(context.Background())
	jetstreamURL := strings.TrimSpace(cfg.JetStreamURL)
	if jetstreamURL != "" {
		client, err := newJetStreamClient(contracts.JetStreamOptions{
			URL:    jetstreamURL,
			Tenant: trimmedTenant,
		})
		if err != nil {
			return nil, err
		}
		return client, nil
	}
	return contracts.NewInMemoryBus(trimmedTenant), nil
}

// defaultGridFactory returns either an in-memory grid client or the configured endpoint client.
func defaultGridFactory() (runner.GridClient, error) {
	endpoint := strings.TrimSpace(os.Getenv(gridEndpointEnv))
	if endpoint == "" {
		return runner.NewInMemoryGrid(), nil
	}
	stateDir, err := ensureWorkflowSDKStateDir()
	if err != nil {
		return nil, err
	}

	options := grid.Options{
		Endpoint:           endpoint,
		StreamOptions:      gridStreamOptions(),
		CursorStoreFactory: grid.NewCursorStoreFactory(stateDir),
	}
	if token := strings.TrimSpace(os.Getenv(gridAPIKeyEnv)); token != "" {
		options.BearerToken = token
	}
	client, err := grid.NewClient(options)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func gridStreamOptions() helper.StreamOptions {
	return helper.StreamOptions{
		HeartbeatInterval: 20 * time.Second,
		MinBackoff:        200 * time.Millisecond,
		MaxBackoff:        5 * time.Second,
	}
}

func ensureWorkflowSDKStateDir() (string, error) {
	if existing := strings.TrimSpace(os.Getenv(workflowSDKStateEnv)); existing != "" {
		if err := os.MkdirAll(existing, 0o755); err != nil {
			return "", fmt.Errorf("prepare workflow sdk state dir: %w", err)
		}
		return existing, nil
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve config dir: %w", err)
	}
	stateDir := filepath.Join(configDir, "ploy", "grid")
	if gridID := sanitizePathComponent(os.Getenv(gridIDEnv)); gridID != "" {
		stateDir = filepath.Join(stateDir, gridID)
	}
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return "", fmt.Errorf("prepare workflow sdk state dir: %w", err)
	}
	if err := os.Setenv(workflowSDKStateEnv, stateDir); err != nil {
		return "", fmt.Errorf("set %s: %w", workflowSDKStateEnv, err)
	}
	return stateDir, nil
}
