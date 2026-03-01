# Internal Workflow Redundancy / Overengineering Refactor

Scope: remove duplicated gate flow logic, centralize file-existence helpers, eliminate repeated `pyproject.toml` reads, flatten gate planning control flow, and replace panic-based `ModsSpec.ToMap()` with error-returning API.

Documentation: `AGENTS.md`; `internal/workflow/step/runner.go`; `internal/workflow/step/gate_only.go`; `internal/workflow/step/gate_docker.go`; `internal/workflow/step/gate_docker_stack_gate.go`; `internal/workflow/stackdetect/detector.go`; `internal/workflow/stackdetect/python.go`; `internal/workflow/stackdetect/maven.go`; `internal/workflow/stackdetect/rust.go`; `internal/workflow/contracts/mods_spec_wire.go`; `docs/build-gate/README.md`; `docs/migs-lifecycle.md`

Legend: [ ] todo, [x] done.

## Phase 1: Deduplicate Gate Stage Flow
- [x] Extract shared hydration and gate execution stage helpers.
  - Repository: `ploy`
  - Component: `internal/workflow/step`
  - Scope:
    - implement one private hydration helper used by both `Runner.Run` and `RunGateOnly`
    - implement one private gate helper that:
      - calls `Gate.Execute`
      - stores metadata
      - applies `StaticChecks[0].Passed` rule
      - returns wrapped `ErrBuildGateFailed` with caller-provided message
    - keep timing field behavior unchanged
  - Files:
    - `internal/workflow/step/runner.go`
    - `internal/workflow/step/gate_only.go`
    - optional new helper file: `internal/workflow/step/runner_gate_stage.go`
  - Tests:
    - `go test ./internal/workflow/step -run 'TestRun|TestRunGateOnly'`

## Phase 2: Centralize File Existence Utility
- [x] Replace duplicated `fileExists` functions with one shared helper.
  - Repository: `ploy`
  - Component: `internal/workflow`
  - Scope:
    - add `internal/workflow/fsutil/file.go` with `FileExists(path string) bool`
    - remove package-local `fileExists` from:
      - `internal/workflow/step/gate_docker.go`
      - `internal/workflow/stackdetect/detector.go`
    - update all call sites in `step` and `stackdetect`
  - Tests:
    - `go test ./internal/workflow/step ./internal/workflow/stackdetect`

## Phase 3: Eliminate Repeated `pyproject.toml` Reads
- [x] Move pyproject parsing to scan phase and reuse cached result.
  - Repository: `ploy`
  - Component: `internal/workflow/stackdetect`
  - Scope:
    - extend `scanResult` with pyproject parse/cache fields
    - read `pyproject.toml` once in `scanWorkspace`
    - reuse cached data in:
      - `DetectTool` tool selection (`pip` vs `poetry`)
      - Python detection flow
    - remove duplicated Poetry checks and repeated file reads
    - preserve precedence and error semantics
  - Files:
    - `internal/workflow/stackdetect/detector.go`
    - `internal/workflow/stackdetect/python.go`
  - Tests:
    - `go test ./internal/workflow/stackdetect -run 'TestDetectTool|TestDetectPython|TestDetector'`

## Phase 4: Flatten Gate Plan Control Flow
- [x] Simplify stack-gate execution planning internals without behavior changes.
  - Repository: `ploy`
  - Component: `internal/workflow/step`
  - Scope:
    - reduce terminal-state wrapper layering in `gate_docker_stack_gate.go`
    - consolidate repeated failure metadata builders into smaller focused helpers
    - keep existing error codes/messages and `RuntimeImage` propagation intact
  - Files:
    - `internal/workflow/step/gate_docker_stack_gate.go`
    - `internal/workflow/step/gate_docker.go` (minimal wiring updates only)
  - Tests:
    - `go test ./internal/workflow/step -run 'TestDockerGate|TestGate|TestResolve'`

## Phase 5: Replace Panic-Based `ModsSpec.ToMap`
- [x] Change `ModsSpec.ToMap()` to return `(map[string]any, error)` and update call sites.
  - Repository: `ploy`
  - Component: `internal/workflow/contracts` + direct callers
  - Scope:
    - update `ModsSpec.ToMap()` signature and implementation
    - remove panic path
    - update tests and all `.ToMap()` callers to handle error explicitly
  - Files:
    - `internal/workflow/contracts/mods_spec_wire.go`
    - all `ModsSpec.ToMap()` call sites found by `rg -n "\\.ToMap\\(\\)"`
  - Tests:
    - `go test ./internal/workflow/contracts`
    - `go test ./...` (compile guard for all call sites)

## Phase 6: Docs and Final Verification
- [x] Sync docs for changed internals and run full validation suite.
  - Repository: `ploy`
  - Component: docs + workflow packages
  - Scope:
    - update `docs/build-gate/README.md` to reflect simplified internal gate planning flow
    - update `docs/migs-lifecycle.md` only where gate execution path wording depends on old duplication
    - keep behavior contract docs unchanged where runtime behavior did not change
  - Validation:
    - `make test`
    - `make vet`
    - `make staticcheck`

## Open Questions
- None.

---

# Pre-Gate Router Healing Loop Refactor

Scope: simplify `pre_gate -> router -> healing -> re_gate` flow, remove cross-node fragile state, consolidate claim-time spec mutation, split overloaded modules by recovery domain, and remove legacy/duplicate recovery contracts.

Documentation: `AGENTS.md`; `internal/nodeagent/execution_orchestrator_gate.go`; `internal/nodeagent/execution_orchestrator_jobs.go`; `internal/nodeagent/manifest.go`; `internal/nodeagent/recovery_io.go`; `internal/nodeagent/run_options.go`; `internal/server/handlers/nodes_complete_healing.go`; `internal/server/handlers/nodes_claim.go`; `internal/server/handlers/jobs_complete.go`; `internal/workflow/contracts/build_gate_config.go`; `internal/workflow/contracts/build_gate_metadata.go`; `internal/workflow/contracts/mods_spec.go`; `internal/workflow/contracts/mods_spec_parse.go`; `docs/build-gate/README.md`; `docs/migs-lifecycle.md`; `docs/envs/README.md`; `docs/api/components/schemas/controlplane.yaml`

Legend: [ ] todo, [x] done.

## Phase 1: Remove Node-Local Recovery Coupling
- [ ] Make heal/re-gate execution independent from node-local run cache files.
  - Repository: `ploy`
  - Component: `internal/nodeagent`, `internal/server/handlers`
  - Scope:
    - introduce typed recovery context payload in claim response for recovery jobs
    - include selected error kind, resolved healing image, and gate/recovery inputs needed by node
    - stop depending on run-local files as the only source of stack/log/profile recovery context
    - keep run-local cache as optional optimization only
  - Files:
    - `internal/server/handlers/nodes_claim.go`
    - `internal/server/handlers/nodes_complete_healing.go`
    - `internal/nodeagent/claimer_loop.go`
    - `internal/nodeagent/handlers.go`
    - `internal/nodeagent/execution_orchestrator_jobs.go`
    - `internal/nodeagent/execution_orchestrator_gate.go`
  - Tests:
    - `go test ./internal/nodeagent -run 'TestExecuteHealingJob|TestPopulateHealingInDir|TestRunRouterForGateFailure'`
    - `go test ./internal/server/handlers -run 'TestMaybeCreateHealingJobs|TestCompleteJob_GateFailure_HealingInsertionRewiresNextChain'`

## Phase 2: Consolidate Claim Spec Mutation to One Typed Pass
- [ ] Replace repeated parse/marshal spec mutations in claim path with a single typed mutator pipeline.
  - Repository: `ploy`
  - Component: `internal/server/handlers`
  - Scope:
    - parse claim spec JSON once
    - apply ordered mutators in one pass: job_id, gitlab defaults, global env, gate profile overrides, recovery candidate overrides, healing selected kind, healing schema env
    - serialize once at the end
    - preserve existing precedence rules (run spec values override defaults)
  - Files:
    - `internal/server/handlers/nodes_claim.go`
    - `internal/server/handlers/claim_spec_mutator.go` (new)
    - `internal/server/handlers/claim_spec_mutator_test.go` (new)
  - Tests:
    - `go test ./internal/server/handlers -run 'TestClaim|TestServerRunsClaim|TestMerge'`

## Phase 3: Split Overloaded Recovery Modules by Domain
- [ ] Extract focused modules for gate runtime, router runtime, healing runtime, and recovery chain rewiring.
  - Repository: `ploy`
  - Component: `internal/nodeagent`, `internal/server/handlers`
  - Scope:
    - keep orchestrator entrypoints thin
    - move router run/hydration logic into dedicated router runtime file
    - move healing in-dir/workspace-policy logic into dedicated healing runtime file
    - split server healing insertion file into:
      - chain rewiring
      - recovery classification/context resolution
      - infra candidate evaluation
      - linked-job cancellation traversal
    - keep behavior and APIs unchanged during split
  - Files:
    - `internal/nodeagent/execution_orchestrator_gate.go`
    - `internal/nodeagent/execution_orchestrator_jobs.go`
    - `internal/server/handlers/nodes_complete_healing.go`
    - extracted files in same packages (new)
  - Tests:
    - `go test ./internal/nodeagent -run 'TestRunRouterForGateFailure|TestBuildRouterManifest|TestExecuteHealingJob|TestUploadHealingWorkspacePolicyFailure'`
    - `go test ./internal/server/handlers -run 'TestMaybeCreateHealingJobs|TestCompleteJob_GateFailure_MixedClassificationCancelsRemaining'`

## Phase 4: Normalize Recovery Contracts and Remove Legacy Paths
- [ ] Replace stringly-typed recovery fields with typed constants/helpers and remove obsolete loop paths.
  - Repository: `ploy`
  - Component: `internal/workflow/contracts`, `internal/nodeagent`, docs/e2e
  - Scope:
    - add typed constants/helpers for `loop_kind` and `error_kind` resolution/validation
    - replace duplicated literal checks (`"healing"`, `"infra"`, `"code"`, `"mixed"`, `"unknown"`) with contract helpers
    - remove legacy healing diff-type branching that no longer participates in active chain
    - remove dead/unused recovery helper functions and update tests
    - align e2e examples/docs to canonical `build_gate.healing.by_error_kind` schema only
  - Files:
    - `internal/workflow/contracts/build_gate_metadata.go`
    - `internal/workflow/contracts/build_gate_config.go`
    - `internal/workflow/contracts/mods_spec.go`
    - `internal/workflow/contracts/mods_spec_parse.go`
    - `internal/nodeagent/recovery_io.go`
    - `internal/nodeagent/job.go`
    - `internal/nodeagent/difffetcher.go`
    - `tests/e2e/migs/*`
    - `docs/build-gate/README.md`
    - `docs/migs-lifecycle.md`
    - `docs/envs/README.md`
  - Tests:
    - `go test ./internal/workflow/contracts -run 'Test.*Healing|Test.*Recovery|Test.*BuildGate'`
    - `go test ./internal/nodeagent -run 'TestParseRouterDecision|TestExecuteHealingJob|TestDiffFetcher'`
    - `go test ./tests/guards -run 'TestDocsGuard'`

## Phase 5: Integrated Validation and Documentation Sync
- [ ] Validate the full recovery loop and keep docs synchronized with the implemented behavior.
  - Repository: `ploy`
  - Component: nodeagent + server handlers + contracts + docs
  - Scope:
    - run focused suites for nodeagent/handlers/contracts recovery paths
    - run project hygiene checks
    - update docs and API schema snippets for new claim/recovery context and normalized contracts
  - Validation:
    - `go test ./internal/nodeagent`
    - `go test ./internal/server/handlers`
    - `go test ./internal/workflow/contracts`
    - `make test`
    - `make vet`
    - `make staticcheck`
