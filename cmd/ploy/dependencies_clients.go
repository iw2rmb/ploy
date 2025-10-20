package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	gridclient "github.com/iw2rmb/grid/sdk/gridclient/go"
	workflowsdk "github.com/iw2rmb/grid/sdk/workflowrpc/go"
	helper "github.com/iw2rmb/grid/sdk/workflowrpc/helper"

	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/grid"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
	"github.com/iw2rmb/ploy/internal/workflow/runtime"
)

// gridClientAPI captures the shared methods ploy consumes from the grid client.
type gridClientAPI interface {
	Status() gridclient.Status
	WorkflowClient(context.Context) (*workflowsdk.Client, error)
	HTTPClient(context.Context) (*http.Client, error)
}

var (
	errGridClientDisabled = errors.New("grid client disabled")

	gridClientOnce      sync.Once
	gridClientInstance  gridClientAPI
	gridClientErr       error
	gridClientStatePath string
	gridClientGridID    string

	newGridClient = func(ctx context.Context, cfg gridclient.Config) (gridClientAPI, error) {
		return gridclient.New(ctx, cfg)
	}

	runtimeRegistry = runtime.NewRegistry()
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

// defaultGridFactory returns either an in-memory grid client or the shared grid client adapter.
func defaultGridFactory() (runner.GridClient, error) {
	selection := strings.TrimSpace(os.Getenv(runtimeAdapterEnv))
	if selection == "" {
		selection = "grid"
	}
	if runtimeRegistry == nil {
		return nil, fmt.Errorf("configure runtime adapters: registry unavailable")
	}
	adapter, _, err := runtimeRegistry.Resolve(selection)
	if err != nil {
		return nil, fmt.Errorf("resolve runtime adapter %q: %w", selection, err)
	}
	client, err := adapter.Connect(context.Background())
	if errors.Is(err, runtime.ErrAdapterDisabled) {
		return runner.NewInMemoryGrid(), nil
	}
	if err != nil {
		return nil, err
	}
	return client, nil
}

// acquireGridClient instantiates or returns the shared grid client instance.
func acquireGridClient(ctx context.Context) (gridClientAPI, error) {
	gridID := firstEnvValue(gridIDEnv, gridIDFallbackEnv)
	apiKey := firstEnvValue(gridAPIKeyEnv, gridAPIKeyFallbackEnv)

	if gridID == "" && apiKey == "" {
		return nil, errGridClientDisabled
	}
	if gridID == "" {
		return nil, fmt.Errorf("%s must be set when %s is provided", envLabel(gridIDEnv, gridIDFallbackEnv), envLabel(gridAPIKeyEnv, gridAPIKeyFallbackEnv))
	}
	if apiKey == "" {
		return nil, fmt.Errorf("%s must be set when %s is provided", envLabel(gridAPIKeyEnv, gridAPIKeyFallbackEnv), envLabel(gridIDEnv, gridIDFallbackEnv))
	}

	gridClientOnce.Do(func() {
		stateDir, err := prepareGridClientStateDir(gridID)
		if err != nil {
			gridClientErr = err
			return
		}

		cfg := gridclient.Config{
			GridID:   gridID,
			APIKey:   apiKey,
			StateDir: stateDir,
		}
		if beaconURL := strings.TrimSpace(os.Getenv(gridClientBeaconEnv)); beaconURL != "" {
			cfg.BeaconURL = beaconURL
		}

		instance, err := newGridClient(ctxOrBackground(ctx), cfg)
		if err != nil {
			gridClientErr = err
			return
		}

		gridClientInstance = instance
		gridClientStatePath = stateDir
		gridClientGridID = gridID
	})

	if gridClientErr != nil {
		return nil, gridClientErr
	}

	if gridClientGridID != "" && !strings.EqualFold(gridClientGridID, gridID) {
		return nil, fmt.Errorf("grid client already configured for %s; restart the CLI to target grid %s", gridClientGridID, gridID)
	}
	if gridClientInstance == nil {
		return nil, fmt.Errorf("grid client unavailable")
	}
	return gridClientInstance, nil
}

// ctxOrBackground ensures a non-nil context for client construction.
func ctxOrBackground(ctx context.Context) context.Context {
	if ctx != nil {
		return ctx
	}
	return context.Background()
}

func firstEnvValue(keys ...string) string {
	for _, key := range keys {
		if trimmed := strings.TrimSpace(os.Getenv(key)); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func envLabel(primary, fallback string) string {
	trimmed := strings.TrimSpace(fallback)
	if trimmed == "" || trimmed == primary {
		return primary
	}
	return primary + "/" + trimmed
}

// prepareGridClientStateDir resolves and prepares the state directory used by the shared grid client.
func prepareGridClientStateDir(gridID string) (string, error) {
	candidates := []string{
		strings.TrimSpace(os.Getenv(gridClientStateEnv)),
		strings.TrimSpace(os.Getenv(workflowSDKStateEnv)),
	}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if err := os.MkdirAll(candidate, 0o755); err != nil {
			return "", fmt.Errorf("prepare grid client state dir: %w", err)
		}
		return candidate, nil
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve config dir: %w", err)
	}
	baseDir := filepath.Join(configDir, "ploy", "grid")
	stateDir := filepath.Join(baseDir, sanitizePathComponent(gridID))
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return "", fmt.Errorf("prepare grid client state dir: %w", err)
	}
	return stateDir, nil
}

// resetGridClientState clears cached grid client state; intended for tests.
func resetGridClientState() {
	gridClientOnce = sync.Once{}
	gridClientInstance = nil
	gridClientErr = nil
	gridClientStatePath = ""
	gridClientGridID = ""
}

func gridStreamOptions() helper.StreamOptions {
	return helper.StreamOptions{
		HeartbeatInterval: 20 * time.Second,
		MinBackoff:        200 * time.Millisecond,
		MaxBackoff:        5 * time.Second,
	}
}

type gridRuntimeAdapter struct{}

func newGridRuntimeAdapter() runtime.Adapter {
	return &gridRuntimeAdapter{}
}

func (gridRuntimeAdapter) Metadata() runtime.AdapterMetadata {
	return runtime.AdapterMetadata{
		Name:        "grid",
		Aliases:     []string{"grid-workflow"},
		Description: "Grid Workflow RPC adapter",
	}
}

func (gridRuntimeAdapter) Connect(ctx context.Context) (runner.GridClient, error) {
	client, err := acquireGridClient(ctx)
	if errors.Is(err, errGridClientDisabled) {
		return nil, runtime.ErrAdapterDisabled
	}
	if err != nil {
		return nil, err
	}

	status := client.Status()
	endpoint := strings.TrimSpace(status.Beacon.WorkflowEndpoint)
	if endpoint == "" {
		return nil, fmt.Errorf("configure grid client: workflow endpoint unavailable from beacon metadata")
	}
	if gridClientStatePath == "" {
		return nil, fmt.Errorf("configure grid client: workflow state directory unresolved")
	}

	options := grid.Options{
		Endpoint:           endpoint,
		StreamOptions:      gridStreamOptions(),
		CursorStoreFactory: grid.NewCursorStoreFactory(gridClientStatePath),
		ControlPlaneHTTP: func(ctx context.Context) (*http.Client, error) {
			return client.HTTPClient(ctx)
		},
		ControlPlaneStatus: func() grid.ControlPlaneStatus {
			status := client.Status()
			return grid.ControlPlaneStatus{APIEndpoint: strings.TrimSpace(status.Beacon.APIEndpoint)}
		},
		LogTailLines:          500,
		WorkflowClientFactory: func(ctx context.Context) (*workflowsdk.Client, error) { return client.WorkflowClient(ctx) },
	}

	return grid.NewClient(options)
}

func init() {
	if runtimeRegistry == nil {
		runtimeRegistry = runtime.NewRegistry()
	}
	if err := runtimeRegistry.Register(newGridRuntimeAdapter()); err != nil {
		panic(err)
	}
}
