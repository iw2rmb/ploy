package transflow

import (
    "context"

    "github.com/iw2rmb/ploy/internal/cli/common"
    "github.com/iw2rmb/ploy/internal/git/provider"
    "fmt"
    "path/filepath"
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

// TransformationExecutorAdapter adapts runner helpers to TransformationExecutor.
type TransformationExecutorAdapter struct{ r *TransflowRunner }

func NewTransformationExecutorAdapter(r *TransflowRunner) *TransformationExecutorAdapter { return &TransformationExecutorAdapter{r: r} }

func (a *TransformationExecutorAdapter) RenderORWAssets(optionID string) (string, error) {
    return a.r.RenderORWApplyAssets(optionID)
}
func (a *TransformationExecutorAdapter) PrepareInputTar(repoPath string) (string, error) {
    // Place tar next to runner workspace (orw-apply specific directories provide final destination later)
    tarPath := filepath.Join(a.r.workspaceDir, "input.tar")
    if err := createTarFromDir(repoPath, tarPath); err != nil {
        return "", err
    }
    return tarPath, nil
}
func (a *TransformationExecutorAdapter) SubmitORWAndFetchDiff(ctx context.Context, renderedHCL string, outDir string) (string, error) {
    // Not yet used: keep a minimal placeholder by validating job and returning expected diff path
    // Future: thread validate/submit helpers and seaweed/exec metadata here
    _ = ctx
    _ = renderedHCL
    diffPath := filepath.Join(outDir, "diff.patch")
    return diffPath, nil
}

// RepoManagerAdapter adapts GitOperationsInterface to RepoManager.
type RepoManagerAdapter struct{ git GitOperationsInterface }

func NewRepoManagerAdapter(git GitOperationsInterface) *RepoManagerAdapter { return &RepoManagerAdapter{git: git} }

func (a *RepoManagerAdapter) Clone(ctx context.Context, repoURL, ref, target string) error {
    return a.git.CloneRepository(ctx, repoURL, ref, target)
}
func (a *RepoManagerAdapter) CreateBranch(ctx context.Context, repoPath, name string) error {
    return a.git.CreateBranchAndCheckout(ctx, repoPath, name)
}
func (a *RepoManagerAdapter) Commit(ctx context.Context, repoPath, message string) error {
    return a.git.CommitChanges(ctx, repoPath, message)
}
func (a *RepoManagerAdapter) Push(ctx context.Context, repoPath, remoteURL, branch string) error {
    return a.git.PushBranch(ctx, repoPath, remoteURL, branch)
}

// MRManagerAdapter adapts GitProvider to MRManager.
type MRManagerAdapter struct{ gp provider.GitProvider }

func NewMRManagerAdapter(gp provider.GitProvider) *MRManagerAdapter { return &MRManagerAdapter{gp: gp} }

func (m *MRManagerAdapter) CreateOrUpdate(ctx context.Context, cfg provider.MRConfig) (string, map[string]any, error) {
    if m.gp == nil {
        return "", nil, fmt.Errorf("git provider not configured")
    }
    if err := m.gp.ValidateConfiguration(); err != nil {
        return "", nil, err
    }
    res, err := m.gp.CreateOrUpdateMR(ctx, cfg)
    if err != nil {
        return "", nil, err
    }
    meta := map[string]any{}
    if res == nil {
        return "", meta, nil
    }
    meta["created"] = res.Created
    return res.MRURL, meta, nil
}
// HealerFanoutAdapter adapts a HealingOrchestrator to the FanoutOrchestrator interface
type HealerFanoutAdapter struct{ H HealingOrchestrator }

func (a HealerFanoutAdapter) RunHealingFanout(ctx context.Context, runCtx interface{}, branches []BranchSpec, maxParallel int) (BranchResult, []BranchResult, error) {
    if a.H == nil { return BranchResult{}, nil, fmt.Errorf("healer not configured") }
    return a.H.RunFanout(ctx, runCtx, branches, maxParallel)
}
