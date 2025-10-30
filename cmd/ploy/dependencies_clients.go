package main

import (
    "context"
    "errors"
    "fmt"
    "os"
    "path/filepath"
    "strings"

    "github.com/iw2rmb/ploy/internal/workflow/contracts"
    "github.com/iw2rmb/ploy/internal/workflow/runner"
    "github.com/iw2rmb/ploy/internal/workflow/runtime"
)

var (
    runtimeRegistry = runtime.NewRegistry()
)

// defaultEventsFactory builds an in-memory events client for local runs.
func defaultEventsFactory(tenant string) (runner.EventsClient, error) {
    _ = tenant
    return contracts.NewInMemoryBus(), nil
}

// defaultGridFactory returns a workflow runtime client according to the selected adapter.
func defaultGridFactory() (runner.GridClient, error) {
    selection := strings.TrimSpace(os.Getenv(runtimeAdapterEnv))
    if selection == "" {
        selection = "local-step"
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
        return nil, err
    }
    if err != nil {
        return nil, err
    }
    return client, nil
}

// prepareGridClientStateDir is retained for tests that reference workspace dirs; returns a stable path.
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

func firstEnvValue(keys ...string) string {
    for _, key := range keys {
        if v := strings.TrimSpace(os.Getenv(key)); v != "" {
            return v
        }
    }
    return ""
}

func envLabel(primary, fallback string) string {
    p := strings.TrimSpace(primary)
    f := strings.TrimSpace(fallback)
    if p == "" {
        return f
    }
    if f == "" {
        return p
    }
    return fmt.Sprintf("%s or %s", p, f)
}

func init() {
    if runtimeRegistry == nil {
        runtimeRegistry = runtime.NewRegistry()
    }
    if err := runtimeRegistry.Register(newLocalRuntimeAdapter()); err != nil {
        panic(err)
    }
}
