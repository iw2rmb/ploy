package main

import (
	"context"
	"fmt"

	"github.com/iw2rmb/ploy/internal/workflow/aster"
	"github.com/iw2rmb/ploy/internal/workflow/contracts"
	"github.com/iw2rmb/ploy/internal/workflow/environments"
	"github.com/iw2rmb/ploy/internal/workflow/lanes"
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

type laneRegistry interface {
	Describe(name string, opts lanes.DescribeOptions) (lanes.Description, error)
}

type laneRegistryLoaderFunc func(dir string) (laneRegistry, error)

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

type environmentFactoryFunc func(l laneRegistry, s snapshotRegistry) (environmentService, error)

type asterLocatorLoaderFunc func(dir string) (aster.Locator, error)

type workspacePreparerFactoryFunc func() (runner.WorkspacePreparer, error)

type laneCacheComposer struct {
	lanes laneRegistry
}

const (
	workflowSDKStateEnv = "GRID_WORKFLOW_SDK_STATE_DIR"
	lanesCatalogEnv     = "PLOY_LANES_DIR"
	gridEndpointEnv     = "GRID_ENDPOINT"
	gridAPIKeyEnv       = "GRID_API_KEY"
	gridIDEnv           = "GRID_ID"
)

var (
	runnerExecutor           runnerInvoker                = runnerInvokerFunc(runner.Run)
	eventsFactory            eventsFactoryFunc            = defaultEventsFactory
	gridFactory              gridFactoryFunc              = defaultGridFactory
	workspacePreparerFactory workspacePreparerFactoryFunc = defaultWorkspacePreparerFactory

	newJetStreamClient = contracts.NewJetStreamClient
)

var (
	knowledgeBaseAdvisorLoader knowledgeBaseAdvisorLoaderFunc = defaultKnowledgeBaseAdvisorLoader
)

// Compose produces cache keys by delegating to the configured lane registry.
func (c laneCacheComposer) Compose(ctx context.Context, req runner.CacheComposeRequest) (string, error) {
	_ = ctx
	if c.lanes == nil {
		return "", fmt.Errorf("lane registry unavailable")
	}
	manifestVersion := req.Stage.Constraints.Manifest.Manifest.Version
	desc, err := c.lanes.Describe(req.Stage.Lane, lanes.DescribeOptions{
		ManifestVersion: manifestVersion,
		AsterToggles:    req.Stage.Aster.Toggles,
	})
	if err != nil {
		return "", err
	}
	return desc.CacheKey, nil
}
