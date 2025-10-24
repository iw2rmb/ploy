package main

import (
	"context"

	"github.com/iw2rmb/ploy/internal/workflow/aster"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/environments"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
	"github.com/iw2rmb/ploy/internal/workflow/snapshots"
)

type gridFactoryFunc func() (runner.GridClient, error)

type snapshotRegistry interface {
	Plan(ctx context.Context, name string) (snapshots.PlanReport, error)
	Capture(ctx context.Context, name string, opts snapshots.CaptureOptions) (snapshots.CaptureResult, error)
}

type snapshotRegistryLoaderFunc func(dir string) (snapshotRegistry, error)

type manifestCompilerLoaderFunc func(dir string) (runner.ManifestCompiler, error)

type environmentService interface {
	Materialize(ctx context.Context, req environments.Request) (environments.Result, error)
}

type environmentFactoryFunc func(s snapshotRegistry) (environmentService, error)

type asterLocatorLoaderFunc func(dir string) (aster.Locator, error)

type workspacePreparerFactoryFunc func() (runner.WorkspacePreparer, error)

const (
	workflowSDKStateEnv   = "GRID_WORKFLOW_SDK_STATE_DIR"
	gridAPIKeyEnv         = "GRID_BEACON_API_KEY"
	gridAPIKeyFallbackEnv = "GRID_API_KEY"
	gridIDEnv             = "PLOY_GRID_ID"
	gridIDFallbackEnv     = "GRID_ID"
	gridClientBeaconEnv   = "GRID_BEACON_URL"
	gridClientStateEnv    = "GRID_CLIENT_STATE_DIR"
	runtimeAdapterEnv     = "PLOY_RUNTIME_ADAPTER"
)

var (
	gridFactory              gridFactoryFunc              = defaultGridFactory
	workspacePreparerFactory workspacePreparerFactoryFunc = defaultWorkspacePreparerFactory

	newJetStreamClient = contracts.NewJetStreamClient
)
