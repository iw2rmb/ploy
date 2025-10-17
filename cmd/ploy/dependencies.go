package main

import (
	"context"

	"github.com/iw2rmb/ploy/internal/workflow/aster"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/environments"
	"github.com/iw2rmb/ploy/internal/workflow/mods"
	"github.com/iw2rmb/ploy/internal/workflow/runner"
	"github.com/iw2rmb/ploy/internal/workflow/snapshots"
)

type runnerInvoker interface {
	Run(ctx context.Context, opts runner.Options) error
}

type runnerInvokerFunc func(context.Context, runner.Options) error

// Run executes the injected runner function implementation.
func (f runnerInvokerFunc) Run(ctx context.Context, opts runner.Options) error {
	return f(ctx, opts)
}

type eventsFactoryFunc func(tenant string) (runner.EventsClient, error)

type gridFactoryFunc func() (runner.GridClient, error)

type jobComposerFactoryFunc func() runner.JobComposer

type cacheComposerFactoryFunc func() runner.CacheComposer

type knowledgeBaseAdvisorLoaderFunc func(path string) (mods.Advisor, error)

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
	workflowSDKStateEnv = "GRID_WORKFLOW_SDK_STATE_DIR"
	gridAPIKeyEnv       = "GRID_BEACON_API_KEY"
	gridIDEnv           = "PLOY_GRID_ID"
	gridClientBeaconEnv = "GRID_BEACON_URL"
	gridClientStateEnv  = "GRID_CLIENT_STATE_DIR"
)

var (
	runnerExecutor           runnerInvoker                = runnerInvokerFunc(runner.Run)
	eventsFactory            eventsFactoryFunc            = defaultEventsFactory
	gridFactory              gridFactoryFunc              = defaultGridFactory
	workspacePreparerFactory workspacePreparerFactoryFunc = defaultWorkspacePreparerFactory
	jobComposerFactory       jobComposerFactoryFunc       = func() runner.JobComposer { return runner.NewStaticJobComposer() }
	cacheComposerFactory     cacheComposerFactoryFunc     = func() runner.CacheComposer { return runner.NewDefaultCacheComposer() }

	newJetStreamClient = contracts.NewJetStreamClient
)

var (
	knowledgeBaseAdvisorLoader knowledgeBaseAdvisorLoaderFunc = defaultKnowledgeBaseAdvisorLoader
)
