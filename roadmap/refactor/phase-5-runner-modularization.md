# Refactor Phase 5 — Runner Modularization (RepoManager/BuildGate/etc.)

Goal
- Formalize TransflowRunner into cohesive, testable components with clear responsibilities and interfaces, reducing coupling and improving maintainability.

Why
- Current runner is partially decomposed via helpers, but still aggregates multiple responsibilities (git ops, HCL rendering, job submission, build gating, MR, events). Formal modules/interfaces improve testability, reuse, and clarity.

Scope
- Introduce the following modules with minimal surface to cover current needs:
  - RepoManager
    - clone(repoURL, ref) → repoPath
    - createBranch(repoPath, name)
    - commit(repoPath, message)
    - push(repoPath, remoteURL, branch)
  - TransformationExecutor
    - renderORWAssets(optionID) → paths
    - prepareInputTar(repoPath) → tarPath
    - submitORWAndFetchDiff(ctx, hclPath, outDir) → diffPath
  - BuildGate
    - check(ctx, repoPath, cfg) → result (success/version)
  - HealingOrchestrator
    - fanout(ctx, runCtx, branches, maxParallel) → winner, results
  - MRManager
    - createOrUpdateMR(ctx, cfg) → url, metadata
  - EventBus
    - report(ctx, Event)

Design Notes
- Keep interfaces in `internal/mods` (or a small `internal/transflow/core` subpackage) to avoid wide-reaching imports.
- Concrete implementations:
  - Production implementations reuse existing helpers (Git, ORW submission, SharedPush, fanout orchestrator, Git provider wrapper, controller reporter).
  - Test implementations: mocks/fakes for fast unit tests.
- Wire modules via a thin runner orchestrator that composes modules; reduce direct env usage by passing explicit configs.

Non-Goals
- No behavior changes beyond structural refactor.
- No renaming of externally visible CLI flags beyond those already done.

Interfaces (draft)
```go
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
```

Plan (TDD)
1) Add interfaces + factory wiring (no behavior change)
2) Create production adapters that delegate to existing helpers
3) Introduce runner struct that composes modules; migrate calls incrementally
4) Add unit tests against interfaces (mocks/fakes) for key flows: apply+build, MR create, fanout
5) Keep E2E green on Dev VPS

Acceptance Criteria
- Clear interfaces defined and used by runner (no direct helper calls from orchestration layer)
- Unit tests cover repo operations, apply+build, fanout, MR path via mocks
- No regression: E2E JavaMigrationComplete passes on Dev VPS
- Docs updated (internal/mods/README.md) to reflect architecture

Risks/Mitigations
- Risk: silent behavior change during wiring → Mitigate with incremental PRs and E2E after each slice.
- Risk: test flakiness on VPS → Keep small, reversible steps; prefer unit tests locally.

Follow-ups (optional)
- Feature flag gating for KB integration to reduce coupling in specific environments
- Collapse event reporting into a shared package used by API/CLI

