package main

import (
    "context"
    "errors"
    "fmt"
    "os"
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

// Grid client state helpers removed along with Grid integration.

func init() {
    if runtimeRegistry == nil {
        runtimeRegistry = runtime.NewRegistry()
    }
    if err := runtimeRegistry.Register(newLocalRuntimeAdapter()); err != nil {
        panic(err)
    }
}
