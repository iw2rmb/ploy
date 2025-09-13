package transflow

import (
    "context"

    "github.com/iw2rmb/ploy/internal/cli/common"
    "github.com/iw2rmb/ploy/internal/git/provider"
)

// Runner modularization interfaces (Phase 5)

type RepoManager interface {
    Clone(ctx context.Context, repoURL, ref, target string) error
    CreateBranch(ctx context.Context, repoPath, name string) error
    Commit(ctx context.Context, repoPath, message string) error
    Push(ctx context.Context, repoPath, remoteURL, branch string) error
}

type TransformationExecutor interface {
    RenderORWAssets(optionID string) (hclPath string, err error)
    PrepareInputTar(repoPath string) (tarPath string, err error)
    SubmitORWAndFetchDiff(ctx context.Context, renderedHCL string, outDir string) (diffPath string, err error)
}

type BuildGate interface {
    Check(ctx context.Context, cfg common.DeployConfig) (*common.DeployResult, error)
}

type HealingOrchestrator interface {
    RunFanout(ctx context.Context, runCtx any, branches []BranchSpec, maxParallel int) (BranchResult, []BranchResult, error)
}

type MRManager interface {
    CreateOrUpdate(ctx context.Context, cfg provider.MRConfig) (url string, meta map[string]any, err error)
}

type EventBus interface {
    Report(ctx context.Context, ev Event) error
}

// BuildGateAdapter wraps the existing BuildCheckerInterface to implement BuildGate.
type BuildGateAdapter struct { checker BuildCheckerInterface }

func NewBuildGateAdapter(c BuildCheckerInterface) *BuildGateAdapter { return &BuildGateAdapter{checker: c} }

func (a *BuildGateAdapter) Check(ctx context.Context, cfg common.DeployConfig) (*common.DeployResult, error) {
    return a.checker.CheckBuild(ctx, cfg)
}

// ModulesFactory provides helpers to construct modules from existing collaborators.
type ModulesFactory struct{}

func NewModulesFactory() *ModulesFactory { return &ModulesFactory{} }

func (f *ModulesFactory) ForBuildGate(c BuildCheckerInterface) BuildGate { return NewBuildGateAdapter(c) }

