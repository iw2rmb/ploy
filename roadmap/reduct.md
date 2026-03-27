# Internal Redundancy Reduction Roadmap

Scope: Reduce cross-module redundancy in `internal/` by consolidating duplicated orchestration, gate-profile, blob-access, handler-contract, and test-harness logic without backward-compatibility constraints.

Documentation: `roadmap/reduct.md`, `README.md`, `internal/server/README.md`, `internal/tui/README.md`, `internal/client/README.md`.

- [x] 1.1 Consolidate run/repo terminal-state derivation into one lifecycle package
  - Type: determined
  - Component: `internal/workflow/lifecycle`, `internal/server/handlers/runs.go`, `internal/server/handlers/jobs_complete_logic.go`, `internal/server/recovery/reconcile_run_completion.go`, `internal/domain/types/statuses.go`
  - Implementation:
    1. Create `internal/workflow/lifecycle/status.go` with exported pure functions for run terminal checks, repo terminal checks, and derived batch status computation.
    2. Replace local status helpers in handlers and recovery code with calls to `workflow/lifecycle` functions.
    3. (n/a) `internal/nodeagent/execution_orchestrator_jobs.go` was originally listed as a migration target, but it contains no `RunStatus`/`RunRepoStatus` terminal checks — it only emits `JobStatus` values as job outputs. No adoption of lifecycle helpers is applicable here.
    4. Remove duplicate local helper functions and dead tests that only validate removed local wrappers.
  - Verification:
    1. Run `go test ./internal/workflow/lifecycle ./internal/server/handlers ./internal/server/recovery ./internal/nodeagent`.
    2. Run `go test ./internal/... -run 'RunRepo|DerivedStatus|Terminal'`.
  - Reasoning: high (12 CFP)

- [ ] 1.2 Extract shared claim/complete orchestration core from handlers and nodeagent
  - Type: assumption-bound
  - Component: `internal/workflow/lifecycle`, `internal/server/handlers/jobs_complete_service.go`, `internal/server/handlers/nodes_claim_service.go`, `internal/server/handlers/nodes_complete_healing.go`, `internal/nodeagent/execution_orchestrator.go`, `internal/nodeagent/execution_orchestrator_jobs.go`, `internal/nodeagent/execution_orchestrator_rehydrate.go`
  - Assumptions: Existing handler/nodeagent behavior around healing candidate and retry transitions is canonical and should be preserved exactly before simplification.
  - Implementation:
    1. Add `internal/workflow/lifecycle/orchestrator.go` with explicit interfaces for claim decision, completion decision, and retry/healing transition output.
    2. Refactor server-side claim and completion service functions to delegate transition computation to the shared orchestrator core.
    3. Refactor nodeagent execution orchestrator functions to consume the same transition outputs instead of local branch trees.
    4. Add parity tests that run identical transition fixtures against server path and nodeagent path.
  - Verification:
    1. Run `go test ./internal/workflow/lifecycle ./internal/server/handlers ./internal/nodeagent`.
    2. Run `go test ./internal/... -run 'Claim|Complete|Healing|Rehydrate'`.
  - Reasoning: xhigh (24 CFP)

- [ ] 2.1 Create a single gate-profile resolution service consumed by server and nodeagent
  - Type: determined
  - Component: `internal/workflow/contracts/gate_profile.go`, `internal/server/handlers/gate_profile_resolver.go`, `internal/server/handlers/gate_profile_persistence.go`, `internal/nodeagent/run_options.go`, `internal/nodeagent/execution_orchestrator_gate.go`, `internal/store/gate_profiles.sql.go`
  - Implementation:
    1. Introduce `internal/workflow/gateprofile/service.go` that resolves default, exact, and latest gate profile with one deterministic precedence algorithm.
    2. Refactor server `gate_profile_resolver` and `gate_profile_persistence` paths to use this service instead of local precedence code.
    3. Refactor nodeagent gate runtime option resolution to call the same service and use the same result type.
    4. Remove duplicated fallback and normalization logic from handlers and nodeagent packages.
  - Verification:
    1. Run `go test ./internal/workflow/gateprofile ./internal/server/handlers ./internal/nodeagent`.
    2. Run `go test ./internal/... -run 'GateProfile|StackGate|BuildGate'`.
  - Reasoning: xhigh (18 CFP)

- [ ] 2.2 Unify gate-profile input contracts and manifest projection
  - Type: determined
  - Component: `internal/workflow/contracts/mods_spec.go`, `internal/workflow/contracts/step_manifest.go`, `internal/workflow/contracts/build_gate_metadata.go`, `internal/nodeagent/manifest.go`, `internal/server/handlers/claim_spec_mutator_gate.go`, `internal/workflow/step/gate_plan_resolver.go`
  - Implementation:
    1. Add one canonical gate-profile projection function in `workflow/contracts` that converts spec inputs into runtime gate metadata.
    2. Replace gate-profile projection logic in `nodeagent/manifest.go` and `handlers/claim_spec_mutator_gate.go` with the canonical function.
    3. Remove duplicate gate metadata normalization and defaulting branches in step planning and claim-spec mutation flows.
    4. Keep one contract-level table-driven test suite that is imported by both server and nodeagent-facing tests.
  - Verification:
    1. Run `go test ./internal/workflow/contracts ./internal/nodeagent ./internal/server/handlers ./internal/workflow/step`.
    2. Run `go test ./internal/... -run 'Manifest|GatePlan|ClaimSpecMutator'`.
  - Reasoning: high (14 CFP)

- [ ] 3.1 Collapse duplicated blob list APIs into selector-based store methods
  - Type: assumption-bound
  - Component: `internal/store/querier.go`, `internal/store/artifact_bundles.sql.go`, `internal/store/logs.sql.go`, `internal/store/diffs.sql.go`, `internal/store/events.sql.go`, `internal/store/models.go`, `internal/store/queries`
  - Assumptions: `sqlc` query regeneration remains the required source of truth for store interfaces and generated files.
  - Implementation:
    1. Replace parallel `List*` versus `List*Meta*` query families with one selector-based query family per blob type.
    2. Regenerate store interfaces and models so call sites consume a single method with explicit projection selection.
    3. Refactor store tests to validate selector behavior instead of separate meta/full method behavior.
    4. Remove deprecated duplicate query definitions and generated wrappers.
  - Verification:
    1. Run `go test ./internal/store/...`.
    2. Run `make test`.
  - Reasoning: xhigh (26 CFP)

- [ ] 3.2 Standardize blob download/read flow across handlers and blobpersist
  - Type: determined
  - Component: `internal/server/handlers/artifacts_download.go`, `internal/server/handlers/artifacts_repo.go`, `internal/server/handlers/diffs.go`, `internal/server/handlers/spec_bundles.go`, `internal/server/blobpersist/service.go`, `internal/nodeagent/uploaders.go`
  - Implementation:
    1. Add a shared blob read helper package with typed readers for artifact bundle, diff, log, and spec bundle by selector.
    2. Refactor artifact and diff handlers to use the shared read helpers and common not-found/error mapping.
    3. Refactor blobpersist recovery artifact loading to reuse the same typed reader interfaces.
    4. Refactor nodeagent uploader response decoding to use one canonical blob metadata response struct.
  - Verification:
    1. Run `go test ./internal/server/handlers ./internal/server/blobpersist ./internal/nodeagent`.
    2. Run `go test ./internal/... -run 'Artifact|Diff|SpecBundle|Blob'`.
  - Reasoning: high (16 CFP)

- [ ] 4.1 Introduce shared HTTP contract helpers for handlers
  - Type: determined
  - Component: `internal/server/handlers/ingest_common.go`, `internal/server/handlers/bootstrap.go`, `internal/server/handlers/migs_crud.go`, `internal/server/handlers/runs.go`, `internal/server/handlers/repos.go`, `internal/server/handlers/events.go`
  - Implementation:
    1. Add `internal/server/handlers/http_contract.go` with shared `RespondJSON`, `DecodeRequest`, and uniform error-envelope helpers.
    2. Migrate handler entrypoints to use shared request decode and response write helpers while preserving status codes and payload fields.
    3. Replace per-file ad-hoc response struct duplicates with shared contract structs where payload shapes are identical.
    4. Remove dead helper fragments in individual handler files after migration.
  - Verification:
    1. Run `go test ./internal/server/handlers`.
    2. Run `go test ./internal/... -run 'Handler|HTTP|DecodeJSON|invalid request'`.
  - Reasoning: xhigh (20 CFP)

- [ ] 4.2 Move reusable request/response DTOs from handlers to domain-level api package
  - Type: assumption-bound
  - Component: `internal/domain/types`, `internal/migs/api`, `internal/server/handlers/*.go`, `internal/client/tui`, `internal/cli/migs`, `internal/cli/runs`
  - Assumptions: Existing endpoint payload schemas are stable enough to centralize without immediate follow-up API changes.
  - Implementation:
    1. Create `internal/domain/api` DTO files for shared run, repo, mig, artifact, and status payload shapes.
    2. Replace duplicated inline handler request/response structs with imported DTO types.
    3. Update client/tui and cli call sites to decode into the same DTOs when endpoint payloads match.
    4. Remove duplicate contract structs from handler and cli packages.
  - Verification:
    1. Run `go test ./internal/server/handlers ./internal/cli/... ./internal/client/... ./internal/migs/api ./internal/domain/...`.
    2. Run `make test`.
  - Reasoning: xhigh (18 CFP)

- [ ] 5.1 Replace monolithic mockStore in handlers with focused fixture builders
  - Type: determined
  - Component: `internal/server/handlers/test_mock_store_core_test.go`, `internal/server/handlers/test_mock_store_jobs_test.go`, `internal/server/handlers/test_mock_store_migs_runs_test.go`, `internal/server/handlers/test_mock_store_artifacts_sbom_test.go`, `internal/server/handlers/test_helpers_test.go`
  - Implementation:
    1. Split `mockStore` into domain-focused fixtures (`runFixtureStore`, `jobFixtureStore`, `artifactFixtureStore`, `migFixtureStore`) with minimal method sets.
    2. Introduce shared fixture builder helpers for repeated setup patterns (run creation, repo rows, job chains, artifact rows).
    3. Migrate handler tests incrementally to targeted fixtures and delete unused fields/methods after each migration batch.
    4. Remove the legacy monolithic `mockStore` type once all tests are migrated.
  - Verification:
    1. Run `go test ./internal/server/handlers`.
    2. Run `go test ./internal/server/... -run 'handlers|recovery'`.
  - Reasoning: high (15 CFP)

- [ ] 5.2 Create shared internal testkit for cross-module orchestration scenarios
  - Type: determined
  - Component: `internal/testutil`, `internal/server/recovery`, `internal/nodeagent`, `internal/workflow/step`, `internal/store`, `internal/cli/follow`
  - Implementation:
    1. Add `internal/testutil/workflowkit` with reusable scenario builders for run states, repo attempts, job graphs, and gate-profile variants.
    2. Replace duplicated scenario assembly in server, nodeagent, workflow, and store tests with `workflowkit` builders.
    3. Add one canonical cross-module golden scenario per behavior path (claim, complete, recover, heal, gate-profile override).
    4. Delete redundant near-duplicate tests that validate the same behavior path without extra assertions.
  - Verification:
    1. Run `go test ./internal/server/... ./internal/nodeagent/... ./internal/workflow/... ./internal/store/... ./internal/cli/follow/...`.
    2. Run `make test`.
  - Reasoning: xhigh (19 CFP)

- [ ] 5.3 Add LOC and duplication guardrails to keep reductions from regressing
  - Type: determined
  - Component: `Makefile`, `scripts/`, `internal/server/handlers`, `internal/nodeagent`, `internal/workflow/contracts`, `internal/store`, `docs/testing-workflow.md`
  - Implementation:
    1. Add a script in `scripts/` that reports per-package non-test LOC and duplicate-symbol heuristics for `internal/` hotspots.
    2. Add `make redundancy-check` target that fails when hotspot package budgets are exceeded after the reduction passes.
    3. Wire `redundancy-check` into CI checks used by contributors for pre-merge validation.
    4. Document guardrail usage and target budgets in `docs/testing-workflow.md`.
  - Verification:
    1. Run `make redundancy-check`.
    2. Run `make test`.
  - Reasoning: medium (8 CFP)
