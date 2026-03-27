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

- [x] 1.2 Extract shared claim/complete transition core and adopt it in server handlers
  - Type: determined
  - Component: `internal/workflow/lifecycle/orchestrator.go`, `internal/server/handlers/jobs_complete_service.go`, `internal/server/handlers/nodes_claim_service.go`, `internal/server/handlers/nodes_complete_healing.go`
  - Implementation:
    1. Add `internal/workflow/lifecycle/orchestrator.go` with explicit interfaces and result structs for claim decision, completion decision, and retry/healing transitions.
    2. Refactor server-side claim and completion service functions to delegate transition computation to the shared orchestrator core.
    3. Refactor server healing completion flow to consume shared retry/healing transition outputs instead of local branch trees.
    4. Add lifecycle plus handler tests that lock transition outputs for server claim/complete/healing paths.
  - Verification:
    1. Run `go test ./internal/workflow/lifecycle ./internal/server/handlers`.
    2. Run `go test ./internal/... -run 'Claim|Complete|Healing'`.
  - Reasoning: high (15 CFP)

## Execution Contract For Remaining Items (1.3+)

Goal lock: every unchecked item below is considered complete only when redundancy is physically removed, not when a shared abstraction is merely introduced.

- Completion gate (mandatory for each item):
  1. Keep one canonical owner for the behavior named in the item and route all listed components to it.
  2. Delete superseded local helpers/branch trees/wrappers in listed components in the same item closure.
  3. Prove removal with factual evidence: in PR notes, include `rg` evidence that old symbols/branches no longer exist in the scoped components.
  4. Prove adoption with factual evidence: in PR notes, include call-site evidence that scoped components now invoke only the canonical owner.
  5. Pass the item verification commands only after (2)-(4) are done; extraction-only closures are invalid.

- Scope rule:
  1. If an item cannot both extract and delete within one closure, split it into sub-items, but do not mark parent item complete until deletion sub-item is done.
  2. No backward-compatibility placeholders, dual paths, or transitional wrappers are kept after item completion unless explicitly required by the user.

- Review rule:
  1. Review comments and completion notes must use structural evidence (removed files/functions/branches and unified call paths), not LOC accounting summaries.
  2. `5.3` remains valuable for regression prevention, but it is not a substitute for structural deduplication evidence in each item closure.

- [x] 1.3 Adopt shared claim/complete transition core in nodeagent and add cross-path parity fixtures
  - Type: assumption-bound
  - Component: `internal/nodeagent/execution_orchestrator.go`, `internal/nodeagent/execution_orchestrator_jobs.go`, `internal/nodeagent/execution_orchestrator_rehydrate.go`, `internal/workflow/lifecycle/orchestrator.go`, `internal/server/handlers/jobs_complete_service.go`
  - Assumptions: Existing nodeagent retry and rehydrate transition behavior matches server transition semantics closely enough to consume shared orchestrator outputs without introducing new nodeagent-only transition types.
  - Implementation:
    1. Route nodeagent claim/complete decisions through `internal/workflow/lifecycle/orchestrator.go` as the only transition owner.
    2. (n/a) `internal/nodeagent/execution_orchestrator_rehydrate.go` was listed as a migration target, but it contains no retry/healing transition branches — it only implements workspace rehydration (git clone + ordered diff application) and diff upload for mig jobs. No adoption of shared orchestrator retry/healing outputs is applicable here. Route nodeagent retry/rehydrate decisions through shared retry/healing transition outputs and delete superseded nodeagent-local decision branches in the remaining components only.
    3. Add parity fixtures that execute identical transition cases through server and nodeagent paths to lock shared semantics.
    4. Delete now-redundant nodeagent-local transition helpers/wrappers and keep no dual-path fallback branches.
  - Verification:
    1. Run `go test ./internal/workflow/lifecycle ./internal/nodeagent ./internal/server/handlers`.
    2. Run `go test ./internal/... -run 'Claim|Complete|Healing|Rehydrate|Retry'`.
    3. Add structural proof in completion notes: removed nodeagent transition symbols and canonical lifecycle call sites.
  - Reasoning: high (8 CFP)
  - Structural proof:
    - Canonical lifecycle call sites in nodeagent (rg `JobStatusFromRunError` `internal/nodeagent/`): `execution_orchestrator.go:uploadFailureStatus` (line 129), `execution_orchestrator_jobs.go:executeStandardJob` (line 418), `execution_mr.go:executeMRJob` (line 220). No fourth call site exists — all error-to-status paths route through the canonical helper.
    - No local duplicate of `JobStatusFromRunError` in nodeagent (rg `func.*StatusFrom|func.*jobStatus` `internal/nodeagent/*.go` returns no match).
    - Gate infra error path (`execution_orchestrator_gate.go`) intentionally uses explicit `Cancelled` (not `JobStatusFromRunError`) to prevent healing activation on infrastructure failures — a deliberate semantic divergence documented inline, not a missing adoption.
    - Server completion paths (`jobs_complete_service_post_actions.go:{onFail,onCancelled,onSuccess}`) route through `lifecycle.EvaluateCompletionDecision` and `lifecycle.IsGateJobType`. No server-local transition branch remains.
    - Parity fixtures: `internal/server/handlers/cross_path_parity_test.go` — `TestCrossPathParity_StandardJobErrorToChainAction` locks the joint nodeagent (`JobStatusFromRunError`) + server (`EvaluateCompletionDecision`) path for mig/heal/MR jobs; `TestCrossPathParity_GateJobStatusToChainAction` locks the server's handling of all three statuses emitted by the gate nodeagent path.

- [x] 1.3.1 Reconcile item 1.3 closure with redundancy-removal contract
  - Type: determined
  - Component: `roadmap/reduct.md`, `internal/nodeagent/execution_orchestrator.go`, `internal/nodeagent/execution_orchestrator_jobs.go`, `internal/nodeagent/execution_orchestrator_gate.go`, `internal/nodeagent/execution_mr.go`, `internal/server/handlers/jobs_complete_service_post_actions.go`, `internal/workflow/lifecycle/orchestrator.go`
  - Implementation:
    1. Re-open `1.3` closure evidence with structural proof instead of fixture-presence-only evidence.
    2. Add one compact cross-path parity fixture suite that executes identical transition cases through concrete nodeagent and server completion paths (without reintroducing large duplicated fixture tables).
    3. Route remaining nodeagent terminal status assignment branches that mirror shared transition/error semantics through canonical lifecycle mapping helpers, then delete superseded local wrappers/branches where feasible.
    4. Mark `1.3` complete again only after parity coverage and structural dedup evidence are both present.
  - Verification:
    1. Run `go test ./internal/workflow/lifecycle ./internal/nodeagent ./internal/server/handlers`.
    2. Run `go test ./internal/... -run 'Claim|Complete|Healing|Rehydrate|Retry'`.
    3. Add structural proof in completion notes: parity fixtures present, and removed nodeagent-local transition branches/call paths replaced by lifecycle-owned outputs.
  - Reasoning: high (9 CFP)
  - Structural proof:
    - Parity fixture added: `internal/server/handlers/cross_path_parity_test.go` with `TestCrossPathParity_StandardJobErrorToChainAction` (6 cases — context.Canceled/DeadlineExceeded and runtime errors across mig/heal/MR job types) and `TestCrossPathParity_GateJobStatusToChainAction` (6 cases — gate infra Cancelled, gate test Fail, gate Success across pre/post/re-gate types).
    - Adoption confirmed (rg evidence): all standard job error-to-status paths in scope (`execution_orchestrator.go`, `execution_orchestrator_jobs.go`, `execution_mr.go`) call `lifecycle.JobStatusFromRunError`; no in-scope file duplicates this logic locally.
    - Remaining explicit status assignments in `execution_orchestrator_gate.go` (gate infra error → `Cancelled`, gate result → `Fail`/`Success`) are deliberate semantic choices, not lifecycle-mirrorings: the gate infra path intentionally diverges from `JobStatusFromRunError` to force `Cancelled` regardless of error type, preventing healing activation on infrastructure failures.
    - 1.3 re-marked complete with structural evidence above; no dual-path branches remain in the listed components.

- [ ] 2.1a Extract canonical gate-profile resolution service
  - Type: determined
  - Component: `internal/workflow/contracts/gate_profile.go`, `internal/workflow/gateprofile/service.go`
  - Implementation:
    1. Introduce `internal/workflow/gateprofile/service.go` as the single owner of default/exact/latest gate-profile precedence.
    2. Move shared fallback/normalization branches from existing resolver code into the service.
    3. Delete superseded contract-local precedence helpers that become redundant after extraction.
    4. Keep service-level table tests that lock precedence semantics.
  - Verification:
    1. Run `go test ./internal/workflow/gateprofile ./internal/workflow/contracts`.
    2. Run `go test ./internal/... -run 'GateProfile|StackGate|BuildGate'`.
    3. Add structural proof in completion notes: removed old precedence helpers and service-owned precedence paths.
  - Reasoning: high (12 CFP)

- [ ] 2.1b Adopt canonical gate-profile service in server paths and delete server-local precedence logic
  - Type: determined
  - Component: `internal/server/handlers/gate_profile_resolver.go`, `internal/server/handlers/gate_profile_persistence.go`, `internal/workflow/gateprofile/service.go`
  - Implementation:
    1. Route server `gate_profile_resolver` and `gate_profile_persistence` through the shared gateprofile service.
    2. Delete server-local precedence/fallback branches replaced by shared service outputs.
    3. Delete server-local normalization wrappers that only mirrored precedence behavior.
    4. Keep no dual resolver path in server handlers.
  - Verification:
    1. Run `go test ./internal/server/handlers ./internal/workflow/gateprofile`.
    2. Run `go test ./internal/... -run 'GateProfile|StackGate|BuildGate'`.
    3. Add structural proof in completion notes: removed server-local precedence symbols and canonical service call sites.
  - Reasoning: high (9 CFP)

- [ ] 2.1c Adopt canonical gate-profile service in nodeagent paths and delete nodeagent-local precedence logic
  - Type: determined
  - Component: `internal/nodeagent/run_options.go`, `internal/nodeagent/execution_orchestrator_gate.go`, `internal/workflow/gateprofile/service.go`
  - Implementation:
    1. Route nodeagent gate runtime option resolution through the shared gateprofile service and shared result types.
    2. Delete nodeagent-local precedence/fallback branches replaced by shared service outputs.
    3. Delete nodeagent-local normalization wrappers that only mirrored precedence behavior.
    4. Keep no dual resolver path in nodeagent gate option assembly.
  - Verification:
    1. Run `go test ./internal/nodeagent ./internal/workflow/gateprofile`.
    2. Run `go test ./internal/... -run 'GateProfile|StackGate|BuildGate'`.
    3. Add structural proof in completion notes: removed nodeagent-local precedence symbols and canonical service call sites.
  - Reasoning: high (9 CFP)

- [ ] 2.2 Unify gate-profile input contracts and manifest projection
  - Type: determined
  - Component: `internal/workflow/contracts/mods_spec.go`, `internal/workflow/contracts/step_manifest.go`, `internal/workflow/contracts/build_gate_metadata.go`, `internal/nodeagent/manifest.go`, `internal/server/handlers/claim_spec_mutator_gate.go`, `internal/workflow/step/gate_plan_resolver.go`
  - Implementation:
    1. Add one canonical gate-profile projection function in `workflow/contracts` for spec-to-runtime gate metadata.
    2. Route `nodeagent/manifest.go` and `handlers/claim_spec_mutator_gate.go` to the canonical projection function.
    3. Delete duplicate gate metadata normalization/defaulting branches from step planning and claim-spec mutation flows.
    4. Keep one contract-level table-driven suite and remove superseded duplicated projection tests.
  - Verification:
    1. Run `go test ./internal/workflow/contracts ./internal/nodeagent ./internal/server/handlers ./internal/workflow/step`.
    2. Run `go test ./internal/... -run 'Manifest|GatePlan|ClaimSpecMutator'`.
    3. Add structural proof in completion notes: removed projection branches and unified contract projection call sites.
  - Reasoning: high (14 CFP)

- [ ] 3.1a Collapse duplicated log/diff list APIs into selector-based store methods
  - Type: assumption-bound
  - Component: `internal/store/querier.go`, `internal/store/logs.sql.go`, `internal/store/diffs.sql.go`, `internal/store/models.go`, `internal/store/queries`
  - Assumptions: `sqlc` query regeneration remains the required source of truth for store interfaces and generated files.
  - Implementation:
    1. Replace parallel `ListLogs*`/`ListDiffs*` meta/full query families with selector-based query families.
    2. Regenerate store interfaces/models so log/diff call sites use one selector-aware method per blob type.
    3. Delete deprecated log/diff duplicate query definitions/generated wrappers and compatibility aliases.
    4. Refactor log/diff store tests to selector behavior and delete split meta/full-path tests.
  - Verification:
    1. Run `go test ./internal/store/... -run 'Log|Diff|List'`.
    2. Run `go test ./internal/store/...`.
    3. Add structural proof in completion notes: removed log/diff duplicate entrypoints and selector-only call paths.
  - Reasoning: high (14 CFP)

- [ ] 3.1b Collapse duplicated artifact/event list APIs into selector-based store methods
  - Type: assumption-bound
  - Component: `internal/store/querier.go`, `internal/store/artifact_bundles.sql.go`, `internal/store/events.sql.go`, `internal/store/models.go`, `internal/store/queries`
  - Assumptions: `sqlc` query regeneration remains the required source of truth for store interfaces and generated files.
  - Implementation:
    1. Replace parallel `ListArtifact*`/`ListEvents*` meta/full query families with selector-based query families.
    2. Regenerate store interfaces/models so artifact/event call sites use one selector-aware method per blob type.
    3. Delete deprecated artifact/event duplicate query definitions/generated wrappers and compatibility aliases.
    4. Refactor artifact/event store tests to selector behavior and delete split meta/full-path tests.
  - Verification:
    1. Run `go test ./internal/store/... -run 'Artifact|Event|List'`.
    2. Run `go test ./internal/store/...`.
    3. Add structural proof in completion notes: removed artifact/event duplicate entrypoints and selector-only call paths.
  - Reasoning: high (14 CFP)

- [ ] 3.1c Finalize selector-only store surface and remove remaining duplicate list entrypoints
  - Type: determined
  - Component: `internal/store/querier.go`, `internal/store/models.go`, `internal/store/queries`
  - Implementation:
    1. Delete remaining duplicate list entrypoints left after `3.1a` and `3.1b`.
    2. Enforce selector-only method families in `querier` interfaces and adapters.
    3. Remove residual compatibility wiring that proxies old method names to selector methods.
    4. Keep one canonical selector-path test per blob type and delete transitional duplicate tests.
  - Verification:
    1. Run `go test ./internal/store/...`.
    2. Run `make test`.
    3. Add structural proof in completion notes: selector-only interface surface and no duplicate list entrypoints.
  - Reasoning: medium (8 CFP)

- [ ] 3.2 Standardize blob download/read flow across handlers and blobpersist
  - Type: determined
  - Component: `internal/server/handlers/artifacts_download.go`, `internal/server/handlers/artifacts_repo.go`, `internal/server/handlers/diffs.go`, `internal/server/handlers/spec_bundles.go`, `internal/server/blobpersist/service.go`, `internal/nodeagent/uploaders.go`
  - Implementation:
    1. Add one shared blob-read helper package with typed readers for artifact bundle, diff, log, and spec bundle selectors.
    2. Route artifact and diff handlers through shared readers/error mapping and delete handler-local read/error branches.
    3. Route blobpersist recovery artifact loading through the same typed reader interfaces and delete local loaders.
    4. Route nodeagent uploader response decoding through one canonical blob metadata response struct and delete duplicate decoders.
  - Verification:
    1. Run `go test ./internal/server/handlers ./internal/server/blobpersist ./internal/nodeagent`.
    2. Run `go test ./internal/... -run 'Artifact|Diff|SpecBundle|Blob'`.
    3. Add structural proof in completion notes: removed local blob-read/decoder branches and shared-reader call sites.
  - Reasoning: high (16 CFP)

- [ ] 4.1 Introduce shared HTTP contract helpers for handlers
  - Type: determined
  - Component: `internal/server/handlers/ingest_common.go`, `internal/server/handlers/bootstrap.go`, `internal/server/handlers/migs_crud.go`, `internal/server/handlers/runs.go`, `internal/server/handlers/repos.go`, `internal/server/handlers/events.go`
  - Implementation:
    1. Add `internal/server/handlers/http_contract.go` as the canonical handler HTTP contract helper surface.
    2. Route handler entrypoints through shared decode/respond helpers while preserving existing status codes/payload fields.
    3. Replace per-file duplicated response structs with shared contract structs where payload shapes match.
    4. Delete superseded per-file helper fragments and duplicated envelope logic; keep no dual helper stacks.
  - Verification:
    1. Run `go test ./internal/server/handlers`.
    2. Run `go test ./internal/... -run 'Handler|HTTP|DecodeJSON|invalid request'`.
    3. Add structural proof in completion notes: removed ad-hoc handler helpers and canonical helper call sites.
  - Reasoning: xhigh (20 CFP)

- [ ] 4.2a Create canonical domain-level DTO package for shared API payloads
  - Type: assumption-bound
  - Component: `internal/domain/api`, `internal/domain/types`, `internal/migs/api`
  - Assumptions: Existing endpoint payload schemas are stable enough to centralize without immediate follow-up API changes.
  - Implementation:
    1. Create `internal/domain/api` DTO files for shared run/repo/mig/artifact/status payload shapes.
    2. Consolidate equivalent existing DTO definitions into canonical domain DTO definitions.
    3. Delete superseded duplicate DTO definitions in `internal/domain/types`/`internal/migs/api` where ownership is moved.
    4. Keep domain-level DTO tests validating field parity for moved contracts.
  - Verification:
    1. Run `go test ./internal/domain/... ./internal/migs/api`.
    2. Run `go test ./internal/... -run 'DTO|Payload|Schema'`.
    3. Add structural proof in completion notes: canonical DTO owners and removed duplicate definitions.
  - Reasoning: high (10 CFP)

- [ ] 4.2b Migrate handlers to canonical DTOs and delete handler-local contract duplicates
  - Type: determined
  - Component: `internal/server/handlers/*.go`, `internal/domain/api`
  - Implementation:
    1. Route handler request/response contracts to `internal/domain/api` DTO imports where payloads match.
    2. Delete duplicated inline handler request/response structs replaced by canonical DTOs.
    3. Delete handler-local compatibility copy types that mirror canonical DTOs.
    4. Keep no dual contract shapes for the same endpoint payload in handlers.
  - Verification:
    1. Run `go test ./internal/server/handlers ./internal/domain/api`.
    2. Run `go test ./internal/... -run 'Handler|DTO|Decode|Encode'`.
    3. Add structural proof in completion notes: removed handler-local duplicates and canonical DTO usage.
  - Reasoning: high (9 CFP)

- [ ] 4.2c Migrate client and CLI decode paths to canonical DTOs and delete duplicate decode structs
  - Type: determined
  - Component: `internal/client/tui`, `internal/cli/migs`, `internal/cli/runs`, `internal/domain/api`
  - Implementation:
    1. Route client/tui/cli decode paths to canonical DTOs where endpoint payloads match.
    2. Delete duplicate decode structs from client and CLI packages replaced by canonical DTOs.
    3. Delete compatibility decode wrappers that only map duplicate structs to canonical DTOs.
    4. Keep no dual decode struct definitions for the same payload shape.
  - Verification:
    1. Run `go test ./internal/cli/... ./internal/client/... ./internal/domain/api`.
    2. Run `make test`.
    3. Add structural proof in completion notes: removed duplicate decode structs and canonical decode/import paths.
  - Reasoning: high (9 CFP)

- [ ] 5.1 Replace monolithic mockStore in handlers with focused fixture builders
  - Type: determined
  - Component: `internal/server/handlers/test_mock_store_core_test.go`, `internal/server/handlers/test_mock_store_jobs_test.go`, `internal/server/handlers/test_mock_store_migs_runs_test.go`, `internal/server/handlers/test_mock_store_artifacts_sbom_test.go`, `internal/server/handlers/test_helpers_test.go`
  - Implementation:
    1. Split `mockStore` into domain-focused fixtures (`runFixtureStore`, `jobFixtureStore`, `artifactFixtureStore`, `migFixtureStore`) with minimal method sets.
    2. Introduce shared fixture builders for repeated setup patterns (run creation, repo rows, job chains, artifact rows).
    3. Migrate handler tests to targeted fixtures and delete unused fields/methods in each migrated slice.
    4. Remove the legacy monolithic `mockStore` type and any compatibility wrappers once migration is complete.
  - Verification:
    1. Run `go test ./internal/server/handlers`.
    2. Run `go test ./internal/server/... -run 'handlers|recovery'`.
    3. Add structural proof in completion notes: removed monolithic mockStore entrypoints and focused fixture usage.
  - Reasoning: high (15 CFP)

- [ ] 5.2 Create shared internal testkit for cross-module orchestration scenarios
  - Type: determined
  - Component: `internal/testutil`, `internal/server/recovery`, `internal/nodeagent`, `internal/workflow/step`, `internal/store`, `internal/cli/follow`
  - Implementation:
    1. Add `internal/testutil/workflowkit` as the canonical cross-module scenario-builder owner.
    2. Route server/nodeagent/workflow/store test scenario assembly through `workflowkit` and delete local scenario builders.
    3. Keep one canonical cross-module golden scenario per behavior path (claim/complete/recover/heal/gate-profile override).
    4. Delete redundant near-duplicate tests that cover identical behavior paths without additional assertions.
  - Verification:
    1. Run `go test ./internal/server/... ./internal/nodeagent/... ./internal/workflow/... ./internal/store/... ./internal/cli/follow/...`.
    2. Run `make test`.
    3. Add structural proof in completion notes: removed duplicate scenario builders/tests and workflowkit-only assembly paths.
  - Reasoning: xhigh (19 CFP)

- [ ] 5.3 Add LOC and duplication guardrails to keep reductions from regressing
  - Type: determined
  - Component: `Makefile`, `scripts/`, `internal/server/handlers`, `internal/nodeagent`, `internal/workflow/contracts`, `internal/store`, `docs/testing-workflow.md`
  - Implementation:
    1. Add a script in `scripts/` that reports structural duplication signals in `internal/` hotspots (duplicate symbol patterns and parallel entrypoint families).
    2. Add `make redundancy-check` that fails on newly introduced duplicate-symbol/parallel-entrypoint findings in scoped hotspot packages.
    3. Wire `redundancy-check` into contributor CI checks so new duplicate paths are blocked at PR time.
    4. Document guardrail usage and failure interpretation in `docs/testing-workflow.md` with remediation flow.
  - Verification:
    1. Run `make redundancy-check`.
    2. Run `make test`.
    3. Add structural proof in completion notes: baseline findings resolved and no newly introduced duplicate paths in scoped hotspots.
  - Reasoning: medium (8 CFP)
