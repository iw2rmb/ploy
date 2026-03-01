# Pre-Gate Router Healing Loop Refactor

Scope: simplify `pre_gate -> router -> healing -> re_gate` flow, remove cross-node fragile state, consolidate claim-time spec mutation, split overloaded modules by recovery domain, and remove legacy/duplicate recovery contracts.

Documentation: `AGENTS.md`; `internal/nodeagent/execution_orchestrator_gate.go`; `internal/nodeagent/execution_orchestrator_jobs.go`; `internal/nodeagent/manifest.go`; `internal/nodeagent/recovery_io.go`; `internal/nodeagent/run_options.go`; `internal/server/handlers/nodes_complete_healing.go`; `internal/server/handlers/nodes_claim.go`; `internal/server/handlers/jobs_complete.go`; `internal/workflow/contracts/build_gate_config.go`; `internal/workflow/contracts/build_gate_metadata.go`; `internal/workflow/contracts/mods_spec.go`; `internal/workflow/contracts/mods_spec_parse.go`; `docs/build-gate/README.md`; `docs/migs-lifecycle.md`; `docs/envs/README.md`; `docs/api/components/schemas/controlplane.yaml`

Legend: [ ] todo, [x] done.

## Phase 1: Remove Node-Local Recovery Coupling
- [x] Make heal/re-gate execution independent from node-local run cache files.
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
