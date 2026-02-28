# Prep Track 2: Universal Recovery Loop

Scope: Implement the adopted Universal Recovery Loop contract for gate failures in the unified jobs pipeline using one runtime loop (`agent -> re_gate`), explicit loop metadata (`loop_kind=healing`), router-driven `error_kind` classification, and strategy-aware healing insertion, without introducing additional loop families or compatibility paths.

Documentation:
- `design/prep-impl.md`
- `design/prep.md`
- `design/prep-simple.md`
- `design/prep-complex.md`
- `design/prep-states.md`
- `docs/build-gate/README.md`
- `docs/migs-lifecycle.md`

Legend: [ ] todo, [x] done.

## Phase 1: Normalize Runtime Loop
- [x] Remove inline recovery runtime path and keep only the discrete jobs loop — ensure one production recovery mechanism and remove dead competing flow.
  - Repository: `ploy`
  - Component: `internal/nodeagent` execution orchestration
  - Scope: Remove/retire `internal/nodeagent/execution_healing.go`, `internal/nodeagent/execution_healing_loop.go`, and inline-only tests; keep dispatch in `internal/nodeagent/execution_orchestrator.go` through `executeGateJob(...)` and `executeHealingJob(...)`; update references in `internal/nodeagent/doc.go` and `docs/build-gate/README.md`.
  - Snippets:
    ```go
    switch jobType {
    case types.JobTypePreGate, types.JobTypePostGate, types.JobTypeReGate:
        r.executeGateJob(ctx, req)
    case types.JobTypeHeal:
        r.executeHealingJob(ctx, req)
    }
    ```
  - Tests: `go test ./internal/nodeagent -run 'TestExecuteRun_'` — only discrete gate/heal job execution paths compile and pass.

- [x] Keep shared recovery parsing helpers but move naming to active-loop semantics — preserve behavior while removing misleading inline-healing naming.
  - Repository: `ploy`
  - Component: `internal/nodeagent` recovery helpers
  - Scope: Rename `internal/nodeagent/execution_healing_io.go` to recovery-oriented file naming (for example `internal/nodeagent/recovery_io.go`) and update callsites in gate/heal executors.
  - Snippets:
    ```go
    // keep helpers unchanged in behavior:
    parseBugSummary(...)
    parseActionSummary(...)
    gateLogPayloadFromMetadata(...)
    ```
  - Tests: `go test ./internal/nodeagent -run 'TestRunRouterForGateFailure|TestExecuteHealingJob'` — parser/helper behavior unchanged.

## Phase 2: Add Loop Metadata Contract
- [x] Extend gate metadata with recovery context fields — make loop contract explicit and persisted on failed gates.
  - Repository: `ploy`
  - Component: workflow contracts (`build_gate` metadata)
  - Scope: Update `internal/workflow/contracts/build_gate_metadata.go` and tests to add a typed recovery block including `loop_kind`, `error_kind`, `strategy_id`, confidence, and reason; enforce validation constraints (single-line, length bounds).
  - Snippets:
    ```go
    type BuildGateStageMetadata struct {
        BugSummary string `json:"bug_summary,omitempty"`
        Recovery   *BuildGateRecoveryMetadata `json:"recovery,omitempty"`
    }
    ```
  - Tests: `go test ./internal/workflow/contracts -run 'TestBuildGateStageMetadata'` — recovery metadata validates and round-trips.

- [x] Extend `JobMeta` and API projection with recovery metadata — expose loop state in persisted job metadata and repo job views.
  - Repository: `ploy`
  - Component: workflow contracts + handlers API
  - Scope: Update `internal/workflow/contracts/job_meta.go`, `internal/workflow/contracts/job_meta_test.go`, and `internal/server/handlers/runs_repo_jobs.go`; keep strict kind validation and allow recovery metadata only on gate and healing-relevant job metadata.
  - Snippets:
    ```go
    type JobMeta struct {
        Kind     JobKind `json:"kind"`
        Gate     *BuildGateStageMetadata `json:"gate,omitempty"`
        Recovery *RecoveryJobMetadata    `json:"recovery,omitempty"`
    }
    ```
  - Tests: `go test ./internal/workflow/contracts -run 'TestJobMeta'` and `go test ./internal/server/handlers -run 'TestListRunRepoJobs'` — recovery fields persist and surface.

## Phase 3: Router Classification Wiring
- [x] Parse structured router output on every gate failure — convert router from bug-summary-only to classifier output source.
  - Repository: `ploy`
  - Component: nodeagent gate execution
  - Scope: Extend `runRouterForGateFailure(...)` in `internal/nodeagent/execution_orchestrator_gate.go` and parser helpers to read `error_kind`, confidence, reason, optional strategy id, and expectations; keep `bug_summary`; default to `error_kind=unknown` on parse failure.
  - Snippets:
    ```go
    if bugSummary := parseBugSummary(routerOutDir); bugSummary != "" {
        gateResult.BugSummary = bugSummary
    }
    gateResult.Recovery = parseRouterDecision(routerOutDir) // new
    ```
  - Tests: `go test ./internal/nodeagent -run 'TestRunRouterForGateFailure'` — gate metadata contains both bug summary and recovery classifier fields.

- [x] Inject gate phase and loop kind signals into router runtime env — provide router with required phase priors and loop context.
  - Repository: `ploy`
  - Component: nodeagent manifest + gate runtime execution
  - Scope: Update `internal/nodeagent/manifest.go` (`buildRouterManifest`) and `internal/nodeagent/execution_orchestrator_gate.go` to pass `PLOY_GATE_PHASE` (`pre_gate|post_gate|re_gate`) and `PLOY_LOOP_KIND=healing` for every gate-failure router execution.
  - Snippets:
    ```go
    manifest.Env["PLOY_GATE_PHASE"] = req.JobType.String()
    manifest.Env["PLOY_LOOP_KIND"] = "healing"
    ```
  - Tests: `go test ./internal/nodeagent -run 'TestBuildRouterManifest|TestRunRouterForGateFailure'` — router receives phase + loop env.

## Phase 4: Control-Plane Strategy Application
- [x] Use persisted `error_kind` to select healing strategy in chain rewiring — make `maybeCreateHealingJobs(...)` router-driven instead of static image-only.
  - Repository: `ploy`
  - Component: server job completion/healing insertion
  - Scope: Update `internal/server/handlers/nodes_complete_healing.go` to parse failed gate `jobs.meta` via `contracts.UnmarshalJobMeta(...)`, resolve strategy from `job_meta.gate.recovery.error_kind`, select heal image/contract, and persist selected strategy metadata into created `heal` and `re_gate` jobs.
  - Snippets:
    ```go
    gateMeta, _ := contracts.UnmarshalJobMeta(failedJob.Meta)
    strategy := resolveRecoveryStrategy(spec, gateMeta.Gate.Recovery.ErrorKind)
    healImage := strategy.Image
    ```
  - Tests: `go test ./internal/server/handlers -run 'TestMaybeCreateHealingJobs'` — inserted jobs reflect router-driven strategy selection.

- [x] Enforce conservative stop for `mixed|unknown` classifications — stop progression deterministically per adopted design.
  - Repository: `ploy`
  - Component: server healing insertion + cancellation policy
  - Scope: In `internal/server/handlers/nodes_complete_healing.go`, when failed gate classification is `mixed` or `unknown`, skip creating heal/re-gate jobs and call `cancelRemainingJobsAfterFailure(...)`; ensure this applies to initial gate and failed `re_gate`.
  - Snippets:
    ```go
    if errorKind == "mixed" || errorKind == "unknown" {
        return cancelRemainingJobsAfterFailure(ctx, st, failedJob)
    }
    ```
  - Tests: `go test ./internal/server/handlers -run 'TestCompleteJob_GateFailure_'` — no healing branch created for `mixed|unknown`, remaining chain cancelled.

## Phase 5: Strategy Selector Surface
- [x] Add recovery strategy selector schema to `build_gate` spec contract — provide typed configuration surface keyed by `error_kind`.
  - Repository: `ploy`
  - Component: workflow contracts + parser + nodeagent typed options
  - Scope: Update `internal/workflow/contracts/build_gate_config.go`, `mods_spec.go`, `mods_spec_parse.go`, and `internal/nodeagent/run_options.go` to use `build_gate.healing.by_error_kind` selector contract; parse/validate error-kind keyed actions and support server-injected `build_gate.healing.selected_error_kind` for heal claims.
  - Snippets:
    ```go
    type BuildGateConfig struct {
        Healing *HealingSpec `json:"healing,omitempty"`
        Router  *RouterSpec  `json:"router,omitempty"`
    }

    type HealingSpec struct {
        SelectedErrorKind string                           `json:"selected_error_kind,omitempty"`
        ByErrorKind       map[string]HealingActionSpec    `json:"by_error_kind,omitempty"`
    }
    ```
  - Tests: `go test ./internal/workflow/contracts -run 'TestParseModsSpecJSON|TestValidate'` and `go test ./internal/nodeagent -run 'TestParseSpec_ProducesTypedOptions'` — recovery selector parsed into typed options.

- [x] Wire infra strategy artifact contract gate to gate profile validation boundary — prepare handoff for profile candidate promotion flow.
  - Repository: `ploy`
  - Component: nodeagent artifact expectations + server gate profile validation path
  - Scope: Define expected artifact path/schema metadata in strategy selection (for example `/out/gate-profile-candidate.json`), and wire handoff boundaries to existing gate profile parser/validator (`internal/workflow/contracts/gate_profile.go`, `internal/server/prep/schema.go`) for next-track promotion logic.
  - Snippets:
    ```go
    if strategy.ErrorKind == "infra" {
        expectArtifact("/out/gate-profile-candidate.json", "gate_profile_v1")
    }
    ```
  - Tests: `go test ./internal/workflow/contracts -run 'TestGateProfile'` and `go test ./internal/server/prep` — artifact contract references align with existing prep schema validation.

## Validation
- [x] Run full validation suite for this track slice — prevent regressions in contracts, nodeagent routing, and server healing orchestration.
  - Repository: `ploy`
  - Component: CI/local validation
  - Scope: Execute:
    - `go test ./internal/workflow/contracts -run 'TestBuildGate|TestJobMeta|TestParseModsSpec|TestGateProfile'`
    - `go test ./internal/nodeagent -run 'TestRunRouterForGateFailure|TestBuildGateJobStats|TestParseSpec_ProducesTypedOptions'`
    - `go test ./internal/server/handlers -run 'TestMaybeCreateHealingJobs|TestCompleteJob_GateFailure_HealingInsertionRewiresNextChain'`
    - `go test ./internal/server/prep`
    - `make test`
    - `make vet`
    - `make staticcheck`
  - Snippets: `N/A`
  - Tests: All commands above pass — track is ready to implement incrementally with confidence.
