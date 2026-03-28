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

- [x] 2.1a Extract canonical gate-profile resolution service
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
  - Structural proof:
    - New canonical owner: `internal/workflow/gateprofile/service.go` with `GateOverrideForJobType`, `StackMatches`, and `SelectProfile` (+ `ProfileCandidate`/`ProfilePrecedence` types) as the single owner of gate-profile override derivation, stack-matching precedence, and default/exact/latest resolution precedence.
    - Removed from contracts (rg `GateProfileGateOverrideForJobType\|GateProfileStackMatches\|gateProfileTargetToBuildGateOverride\|gateProfileRuntimeGateEnv\|defaultGateProfileDockerHostSocket` `internal/workflow/contracts/gate_profile.go` returns no match): all five symbols deleted.
    - Adoption confirmed: `internal/server/handlers/claim_spec_mutator_gate.go` calls `gateprofile.GateOverrideForJobType` at both call sites; `internal/server/handlers/nodes_complete_healing_infra_candidate.go` calls `gateprofile.StackMatches`. No remaining `contracts.GateProfileGateOverrideForJobType` or `contracts.GateProfileStackMatches` references in codebase.
    - Superseded tests removed: `gate_profile_test.go` in contracts no longer contains override/stack-match tests; table-driven coverage lives in `internal/workflow/gateprofile/service_test.go` (`TestGateOverrideForJobType` 11 cases, `TestStackMatches` 8 cases).

- [x] 2.1b Adopt canonical gate-profile service in server paths and delete server-local precedence logic
  - Type: determined
  - Component: `internal/server/handlers/gate_profile_resolver.go`, `internal/server/handlers/gate_profile_persistence.go`, `internal/workflow/gateprofile/service.go`
  - Implementation:
    1. Route server `gate_profile_resolver` through the shared gateprofile service and delete server-local precedence branches.
    2. (n/a) `internal/server/handlers/gate_profile_persistence.go` was listed as a migration target, but it contains no profile-selection precedence mirroring the service. Its logic is write-side only: `resolveGateProfileStackRow` performs a DB lookup chain (by full expectation → lang/tool → image) to find the stack row to write against, `buildSuccessfulGateProfilePayload` constructs a new profile struct from detected gate metadata, and `persistGateProfilePayload` writes the blob and upserts DB rows. None of these duplicate the service's read-side `SelectProfile`/`GateOverrideForJobType`/`StackMatches` logic. No adoption of gateprofile service functions in persistence is applicable here.
    3. Delete server-local precedence/fallback branches replaced by shared service outputs (resolver only).
    4. Keep no dual resolver path in server handlers.
  - Verification:
    1. Run `go test ./internal/server/handlers ./internal/workflow/gateprofile`.
    2. Run `go test ./internal/... -run 'GateProfile|StackGate|BuildGate'`.
    3. Add structural proof in completion notes: removed server-local precedence symbols and canonical service call sites.
  - Reasoning: high (9 CFP)
  - Structural proof:
    - Removed `gateProfileResolverStackRow` from `gate_profile_resolver.go` (rg `gateProfileResolverStackRow` `internal/` returns no match).
    - Removed server-local precedence/fallback control (`if exactCand == nil` and nested `if latestCand == nil`) from `ResolveGateProfileForJob`; the `if`-chain and inline candidate accumulation are gone.
    - Added `gateprofile.SelectProfileLazy` to `internal/workflow/gateprofile/service.go` as the canonical owner of the lazy exact→latest→default fetch order; it calls fetchExact first and skips fetchLatest/fetchDefault when a higher-priority candidate is found.
    - Adoption confirmed: `gate_profile_resolver.go` now calls `gateprofile.SelectProfileLazy` at line 90 with three closures wrapping `fetchExactCandidate`, `fetchLatestCandidate`, `fetchDefaultCandidate`; no inline precedence decision branches remain in the resolver.
    - `SelectProfileLazy` covered by `TestSelectProfileLazy` (7 cases) in `internal/workflow/gateprofile/service_test.go`; resolver regression tests `TestGateProfileResolver_ExactHit_LatestDefaultErrorsNotReached` and `TestGateProfileResolver_LatestFound_DefaultErrorNotReached` continue to pass because lazy skip behavior is now owned by the service function.
    - Consolidated duplicate stack row type: `gateProfileResolverStackRow` deleted; `gate_profile_resolver.go` now uses `gateProfileStackRow` (from `gate_profile_persistence.go`) throughout, including the `gateProfileResolverStore` interface and `sqlGateProfileResolverStore.ResolveStackRowByImage`.
    - No dual precedence path remains in resolver: single call to `gateprofile.SelectProfileLazy` owns the exact/latest/default ordering and lazy fetch control.
    - Persistence de-scoped (see step 2 n/a): `gate_profile_persistence.go` has no profile-selection precedence to route; its write-side stack resolution and payload construction are not duplicates of any service function.

- [x] 2.1c Adopt canonical gate-profile service in nodeagent paths and delete nodeagent-local precedence logic
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
  - Structural proof:
    - Removed from `execution_orchestrator_gate.go` (rg confirms no match): `deriveGateProfileSnapshotFromOverride`, `resolveGateProfileSnapshotTarget`, `resolveGateProfileSnapshotStack`, `gateProfileCommandFromOverride`, `copySnapshotEnv` — five local precedence/normalization symbols deleted.
    - Removed `strconv` import from `execution_orchestrator_gate.go` (was only used by `gateProfileCommandFromOverride`).
    - Adoption confirmed: `resolveGateProfileSnapshotRaw` in `execution_orchestrator_gate.go` now calls `gateprofile.DeriveProfileSnapshotFromOverride` as the single owner of override→profile snapshot derivation (including target resolution, stack precedence, and command normalization).
    - Canonical owner: `internal/workflow/gateprofile/service.go` gains `DeriveProfileSnapshotFromOverride` with helpers `resolveSnapshotTarget`, `resolveSnapshotStack`, `commandFromOverride`, `snapshotEnvCopy`. Precedence semantics: override.Stack > DetectedStackExpectation > ModStack name; explicit target > override.Target > all_tests default.
    - New coverage: `TestDeriveProfileSnapshotFromOverride` (11 cases) in `internal/workflow/gateprofile/service_test.go` locks shell/exec command forms, all three target selection paths, stack resolution tiers, and error cases (nil override, empty command, no stack, unsupported job type).
    - No dual snapshot derivation path remains: nodeagent `persistGateProfileSnapshot` calls `resolveGateProfileSnapshotRaw` which delegates entirely to service.

- [x] 2.2 Unify gate-profile input contracts and manifest projection
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
  - Completion notes:
    - Added `BuildGateProfileOverrideToSpecMap` and `ApplyBuildGatePhaseToGateSpec` to `internal/workflow/contracts/build_gate_config.go`.
    - Deleted `buildGatePrepOverrideToMap` and `commandSpecToWireValue` from `internal/server/handlers/claim_spec_mutator_gate.go`; call site updated to `contracts.BuildGateProfileOverrideToSpecMap`.
    - `BuildGateOptions` in `internal/nodeagent/run_options.go` collapsed from 8 flattened fields (`PreStack`, `PostStack`, `PreGateProfile`, `PostGateProfile`, `PreTarget`, `PostTarget`, `PreAlways`, `PostAlways`) to 2 phase-config pointers (`Pre`, `Post *contracts.BuildGatePhaseConfig`); deleted `copyBuildGateProfileOverride`; `modsSpecToRunOptions` reduced to direct pointer assignment.
    - `applyGatePhaseOverrides` in `internal/nodeagent/execution_orchestrator_gate.go` updated to call `contracts.ApplyBuildGatePhaseToGateSpec`; re_gate case clears `StackDetect=nil` since it uses persisted stack from original gate run.
    - (n/a) `internal/nodeagent/manifest.go` was listed as a routing target, but it contains no gate-phase projection logic. `buildGateManifestFromRequest` is a pure manifest builder that constructs a base manifest with a blank `StepGateSpec`; it has no `Target`/`Always`/`GateProfile`/`StackDetect` assignment branches to canonicalize. All gate-phase projection for nodeagent gate jobs is owned by `applyGatePhaseOverrides` in `execution_orchestrator_gate.go`, which already calls `contracts.ApplyBuildGatePhaseToGateSpec` as the sole projection site. No adoption of the canonical projection function in `manifest.go` is applicable here.
    - (n/a) `internal/workflow/step/gate_plan_resolver.go` was listed as a routing target, but it contains no gate-phase projection logic. `gatePlanResolver.Resolve` is a downstream consumer of an already-projected `*contracts.StepGateSpec`: it receives `spec.GateProfile` and `spec.Target` after projection has been applied upstream by `contracts.ApplyBuildGatePhaseToGateSpec` (in `execution_orchestrator_gate.go`). `resolveGateCommand` consumes these already-projected fields to select the execution command; it does not duplicate any normalization or defaulting branches from `ApplyBuildGatePhaseToGateSpec` or `BuildGateProfileOverrideToSpecMap`. No adoption of the canonical projection functions in `gate_plan_resolver.go` is applicable here.
    - Test files updated: `execution_orchestrator_gate_stackdetect_test.go` and `run_options_test.go` migrated to `Pre`/`Post` phase-config accessors.
    - New table-driven suite: `internal/workflow/contracts/build_gate_projection_test.go` (15 cases for both canonical functions).

- Open items revalidated against codebase on `2026-03-28`:
  - `3.1*`: still needed (`internal/store/querier.go` still exposes parallel `List*` + `List*Meta*` families for logs/diffs/artifacts/events).
  - `3.2`: still needed, with expanded scope covering server read-path unification plus nodeagent diff/spec-bundle transfer wiring used by rehydration and `/tmp`/`/in`/`/out` execution flows.
  - `4.1`: still needed, but should extend existing `ingest_common.go` helpers instead of adding a parallel helper stack.
  - `4.2*`: still needed (`internal/domain/api` does not exist; DTO duplication remains spread across handlers/client/cli).
  - `5.1`: still needed (monolithic `mockStore` still anchored in `test_mock_store_core_test.go`).
  - `5.2`: still needed (`internal/testutil/workflowkit` does not exist), but should target only cross-module scenarios to avoid over-abstraction.
  - `5.3`: still needed (`Makefile` has no `redundancy-check`; docs have no guardrail flow).

- [ ] 3.1a Collapse duplicated log/diff list APIs into selector-based store methods
  - Type: assumption-bound
  - Component: `internal/store/querier.go`, `internal/store/logs.sql.go`, `internal/store/diffs.sql.go`, `internal/store/queries/logs.sql`, `internal/store/queries/diffs.sql`, `internal/store/list_meta_queries_test.go`
  - Assumptions: `sqlc` query regeneration remains the source of truth for store interfaces and generated files.
  - Implementation:
    1. Replace parallel log/diff `List*` + `List*Meta*` query families with selector-based families (`metadata_only` switch in one canonical list path per filter shape).
    2. Regenerate `sqlc` output and adapt `querier` signatures so call sites choose selector flags, not separate method names.
    3. Delete deprecated query names/wrappers and compatibility aliases.
    4. Refactor log/diff store tests to selector behavior and remove split meta/full-path suites.
  - Verification:
    1. Run `go test ./internal/store/... -run 'Log|Diff|List'`.
    2. Run `go test ./internal/store/...`.
    3. Add structural proof in completion notes: removed log/diff duplicate entrypoints and selector-only call paths.
  - Estimated LOC influence: `+140/-260` (net `-120`) in `internal/store/*`.
  - Clarity / complexity check: One list family per blob type reduces naming noise without adding new runtime branches.
  - Reasoning: high (14 CFP)

- [ ] 3.1b Collapse duplicated artifact/event list APIs into selector-based store methods
  - Type: assumption-bound
  - Component: `internal/store/querier.go`, `internal/store/artifact_bundles.sql.go`, `internal/store/events.sql.go`, `internal/store/queries/artifact_bundles.sql`, `internal/store/queries/events.sql`, `internal/store/list_meta_queries_test.go`
  - Assumptions: `sqlc` query regeneration remains the source of truth for store interfaces and generated files.
  - Implementation:
    1. Replace parallel artifact/event `List*` + `List*Meta*` query families with selector-based families.
    2. Regenerate `sqlc` output and migrate call sites to selector-aware methods.
    3. Delete deprecated duplicate query definitions and wrappers.
    4. Refactor artifact/event tests to selector behavior and remove split-path tests.
  - Verification:
    1. Run `go test ./internal/store/... -run 'Artifact|Event|List'`.
    2. Run `go test ./internal/store/...`.
    3. Add structural proof in completion notes: removed artifact/event duplicate entrypoints and selector-only call paths.
  - Estimated LOC influence: `+130/-240` (net `-110`) in `internal/store/*`.
  - Clarity / complexity check: Keeps existing filter shapes; only list-surface naming is unified.
  - Reasoning: high (14 CFP)

- [ ] 3.1c Finalize selector-only store surface and remove remaining duplicate list entrypoints
  - Type: determined
  - Component: `internal/store/querier.go`, `internal/store/models.go`, `internal/store/queries`, `internal/server/handlers/test_mock_store_artifacts_diffs_test.go`
  - Implementation:
    1. Delete residual duplicate list entrypoints after `3.1a` and `3.1b`.
    2. Enforce selector-only method families in `querier` interfaces and test doubles.
    3. Remove compatibility wiring that proxies old names to selector methods.
    4. Keep one canonical selector-path test per blob type and delete transitional duplicate tests.
  - Verification:
    1. Run `go test ./internal/store/...`.
    2. Run `go test ./internal/server/handlers -run 'Artifact|Diff|Event|Log'`.
    3. Add structural proof in completion notes: selector-only interface surface and no duplicate list entrypoints.
  - Estimated LOC influence: `+20/-90` (net `-70`) across store adapters and mocks.
  - Clarity / complexity check: Final cleanup; no additional abstraction layer introduced.
  - Reasoning: medium (8 CFP)

- [ ] 3.2 Standardize blob transfer/read flow across handlers, blobpersist, and nodeagent rehydration I/O
  - Type: determined
  - Component: `internal/server/handlers/artifacts_download.go`, `internal/server/handlers/diffs.go`, `internal/server/handlers/spec_bundles.go`, `internal/server/handlers/events.go`, `internal/server/handlers/gate_profile_resolver.go`, `internal/server/blobpersist/service.go`, `internal/server/blobpersist/sbom.go`, `internal/server/handlers/ingest_common.go`, `internal/nodeagent/uploaders.go`, `internal/nodeagent/difffetcher.go`, `internal/nodeagent/logstreamer.go`, `internal/nodeagent/execution_orchestrator_rehydrate.go`, `internal/nodeagent/execution_orchestrator_tmpbundle.go`, `internal/nodeagent/execution_orchestrator_jobs.go`, `internal/nodeagent/execution_orchestrator_jobs_upload.go`, `internal/workflow/step/container_spec.go`
  - Implementation:
    1. Add one shared server blob-read helper family (key validation + `blobstore.Get` error mapping + stream/read utilities).
    2. Route artifact/diff/spec-bundle/download handlers, SSE log backfill reads, and gate-profile resolver object reads through shared helpers and delete local read/error branches.
    3. Route blobpersist recovery/SBOM/diff-clone bundle reads through the same helper family and delete local loader branches.
    4. Route nodeagent diff/spec-bundle download request mechanics through canonical transfer helpers and remove duplicate request/status/response branches (`uploaders`, `difffetcher`, `logstreamer`).
    5. Keep rehydration patch application semantics and `/tmp` extraction semantics stable while consolidating `/tmp` materialization and `/in`/`/out` wiring scaffolding where duplicated.
  - Verification:
    1. Run `go test ./internal/server/handlers ./internal/server/blobpersist`.
    2. Run `go test ./internal/nodeagent ./internal/workflow/step -run 'Uploader|LogStreamer|DiffFetcher|TmpBundle|Artifact|Rehydrate'`.
    3. Run `go test ./internal/... -run 'Artifact|Diff|SpecBundle|Blob|SBOM|Rehydrate|TmpBundle'`.
    4. Add structural proof in completion notes: removed local blob-read/transfer branches and canonical helper call sites in server + nodeagent.
  - Estimated LOC influence: `+190/-280` (net `-90`) with branch-count reduction across server and nodeagent transfer paths.
  - Clarity / complexity check: Centralizes repeated I/O and transfer plumbing while preserving endpoint/runtime-specific semantics.
  - Reasoning: high (13 CFP)

- [ ] 4.1 Expand shared HTTP handler contract helpers and remove ad-hoc decode/respond branches
  - Type: determined
  - Component: `internal/server/handlers/ingest_common.go`, `internal/server/handlers/bootstrap.go`, `internal/server/handlers/migs_crud.go`, `internal/server/handlers/runs.go`, `internal/server/handlers/repos.go`, `internal/server/handlers/events.go`
  - Implementation:
    1. Extend `ingest_common.go` with canonical JSON response helpers (`writeJSON`, `writeJSONStatus`) and common query parsing reuse (`parsePagination` adoption).
    2. Route selected handlers through shared decode/respond helpers while preserving status codes and payload fields.
    3. Remove duplicated per-handler envelope/encode fragments replaced by helper calls.
    4. Keep endpoint-specific request/response structs local unless shape duplication is exact.
  - Verification:
    1. Run `go test ./internal/server/handlers`.
    2. Run `go test ./internal/... -run 'Handler|HTTP|DecodeJSON|invalid request'`.
    3. Add structural proof in completion notes: removed ad-hoc helper fragments and canonical helper call sites.
  - Estimated LOC influence: `+90/-190` (net `-100`) in handler entrypoints.
  - Clarity / complexity check: Reuses existing helper module instead of introducing a second helper domain.
  - Reasoning: high (16 CFP)

- [ ] 4.2a Create canonical domain-level DTO package for shared API payloads
  - Type: assumption-bound
  - Component: `internal/domain/api`, `internal/domain/types`, `internal/migs/api`
  - Assumptions: Shared payloads moved in this slice are stable across server/client/cli (`run`, `repo`, `mig` summary and common list envelopes).
  - Implementation:
    1. Create `internal/domain/api` DTO files for payloads reused by at least two boundaries (handlers + client/cli decode).
    2. Move canonical ownership of selected DTOs from `internal/migs/api` and handler-local mirrors into `internal/domain/api`.
    3. Delete superseded duplicate definitions where ownership moves.
    4. Add domain-level DTO tests for JSON field parity and backward wire-shape stability.
  - Verification:
    1. Run `go test ./internal/domain/... ./internal/migs/api`.
    2. Run `go test ./internal/... -run 'DTO|Payload|Schema'`.
    3. Add structural proof in completion notes: canonical DTO owners and removed duplicate definitions.
  - Estimated LOC influence: `+180/-70` (net `+110`) for package introduction.
  - Clarity / complexity check: Introduces one shared boundary package, but only for concretely duplicated payloads.
  - Reasoning: high (10 CFP)

- [ ] 4.2b Migrate handlers to canonical DTOs and delete handler-local contract duplicates
  - Type: determined
  - Component: `internal/server/handlers/*.go`, `internal/domain/api`
  - Implementation:
    1. Route handler contracts to `internal/domain/api` DTO imports where payload shapes are exact matches.
    2. Delete duplicated inline handler request/response structs replaced by canonical DTOs.
    3. Delete compatibility copy types that only map duplicate shapes.
    4. Keep no dual contract shapes for the same endpoint payload in handlers.
  - Verification:
    1. Run `go test ./internal/server/handlers ./internal/domain/api`.
    2. Run `go test ./internal/... -run 'Handler|DTO|Decode|Encode'`.
    3. Add structural proof in completion notes: removed handler-local duplicates and canonical DTO usage.
  - Estimated LOC influence: `+70/-170` (net `-100`) in handlers.
  - Clarity / complexity check: Reduces contract drift by importing canonical DTOs; no runtime behavior change.
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
  - Estimated LOC influence: `+60/-130` (net `-70`) across client/cli adapters.
  - Clarity / complexity check: Unifies decode contracts without changing transport behavior.
  - Reasoning: high (9 CFP)

- [ ] 5.1 Replace monolithic mockStore in handlers with focused fixture builders
  - Type: determined
  - Component: `internal/server/handlers/test_mock_store_core_test.go`, `internal/server/handlers/test_mock_store_jobs_test.go`, `internal/server/handlers/test_mock_store_migs_runs_test.go`, `internal/server/handlers/test_mock_store_artifacts_diffs_test.go`, `internal/server/handlers/test_mock_store_spec_bundles_test.go`, `internal/server/handlers/test_helpers_test.go`
  - Implementation:
    1. Split `mockStore` into domain-focused fixtures (`runFixtureStore`, `jobFixtureStore`, `artifactFixtureStore`, `migFixtureStore`) with minimal method sets.
    2. Introduce shared fixture builders for repeated setup patterns (run creation, repo rows, job chains, artifact rows).
    3. Migrate handler tests to targeted fixtures and delete unused fields/methods in each migrated slice.
    4. Remove the legacy monolithic `mockStore` type and compatibility wrappers when migration is complete.
  - Verification:
    1. Run `go test ./internal/server/handlers`.
    2. Run `go test ./internal/server/... -run 'handlers|recovery'`.
    3. Add structural proof in completion notes: removed monolithic mockStore entrypoints and focused fixture usage.
  - Estimated LOC influence: `+240/-620` (net `-380`) in handler test scaffolding.
  - Clarity / complexity check: Reduces cognitive load in tests; fixture scope remains domain-local, not global.
  - Reasoning: high (15 CFP)

- [ ] 5.2 Create shared internal testkit for cross-module orchestration scenarios
  - Type: determined
  - Component: `internal/testutil/workflowkit`, `internal/server/recovery`, `internal/nodeagent`, `internal/workflow/step`, `internal/store`, `internal/cli/follow`
  - Implementation:
    1. Add `internal/testutil/workflowkit` as the canonical owner for cross-module scenario builders only.
    2. Route cross-module orchestration tests (claim/complete/recover/heal/gate-profile override) through `workflowkit`; keep local unit fixtures local.
    3. Keep one canonical scenario per behavior path with explicit assertions.
    4. Delete redundant near-duplicate cross-module scenarios that provide no additional assertions.
  - Verification:
    1. Run `go test ./internal/server/... ./internal/nodeagent/... ./internal/workflow/... ./internal/store/... ./internal/cli/follow/...`.
    2. Run `make test`.
    3. Add structural proof in completion notes: removed duplicate cross-module builders/tests and workflowkit-only assembly paths.
  - Estimated LOC influence: `+260/-170` (net `+90`) while reducing duplicated scenario setup.
  - Clarity / complexity check: Scope is constrained to cross-module tests to prevent framework overreach.
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
  - Estimated LOC influence: `+130/-10` (net `+120`) for guardrail script and docs.
  - Clarity / complexity check: Adds tooling overhead but reduces future review ambiguity and regression risk.
  - Reasoning: medium (8 CFP)

- [x] 6.1 Consolidate duplicate Docker client fakes into composable test mock
  - Type: determined
  - Component: `internal/nodeagent/testutil_test.go`, `internal/nodeagent/crash_reconcile_test.go`, `internal/nodeagent/claim_cleanup_test.go`
  - Implementation:
    1. Added `fakeDockerClient` to `testutil_test.go` with composable fields covering both `crashReconcileDockerClient` and `claimCleanupDockerClient` interfaces.
    2. Replaced `fakeCrashReconcileDockerClient` across `crash_reconcile_test.go`, `claimer_loop_test.go`, `agent_claim_test.go`, `startup_reconcile_test_helpers_test.go`.
    3. Replaced `fakeClaimCleanupDockerClient` in `claim_cleanup_test.go` and deleted both type definitions.
  - Reasoning: medium (7 CFP)

- [x] 6.2 Convert `recovery_io_test.go` to table-driven tests
  - Type: determined
  - Component: `internal/nodeagent/recovery_io_test.go`
  - Implementation:
    1. Collapsed 2 `parseActionSummary` tests into 1 table-driven `TestParseActionSummary` (2 cases).
    2. Collapsed 3 defaults-to-unknown tests into 1 table-driven `TestParseRouterDecision_DefaultsToUnknown` (3 cases); kept `TestParseRouterDecision_ParsesStructuredFields` as-is.
    3. Collapsed 4 `parseORWFailureMetadata` tests into 1 table-driven `TestParseORWFailureMetadata` (4 cases).
  - Reasoning: low (3 CFP)

- [x] 6.3 Convert `verifyBundleDigest` and `extractTmpBundle` rejection tests to table-driven form
  - Type: determined
  - Component: `internal/nodeagent/execution_orchestrator_tmpbundle_test.go`
  - Implementation:
    1. Merged 5 `TestVerifyBundleDigest_*` functions into 1 table-driven `TestVerifyBundleDigest` (5 cases).
    2. Merged symlink + hardlink rejection tests into `TestExtractTmpBundle_RejectsUnsafeEntryType` (2 cases).
    3. Merged absolute-path, traversal, and duplicate-path rejection tests into `TestExtractTmpBundle_RejectsUnsafePath` (4 cases).
  - Reasoning: medium (5 CFP)

- [x] 6.4 Collapse `execution_orchestrator_helpers_test.go` into table-driven tests
  - Type: determined
  - Component: `internal/nodeagent/execution_orchestrator_helpers_test.go`
  - Implementation:
    1. Merged 2 `TestWithTempDir_*` tests into 1 table-driven `TestWithTempDir`.
    2. Merged 3 `TestSnapshotWorkspaceForNoIndexDiff_*` tests into 1 table-driven `TestSnapshotWorkspaceForNoIndexDiff`.
    3. Merged 2 `TestClearManifestHydration_*` tests into 1 table-driven `TestClearManifestHydration`.
    4. Merged 2 `TestDisableManifestGate_*` tests into 1 table-driven `TestDisableManifestGate`.
    5. Stripped redundant doc comments restating test names.
  - Reasoning: medium (5 CFP)

- [x] 6.5 Convert `heartbeat_connection_test.go` BuildURL and `tls_test.go` BootstrapTLS error tests to table-driven form
  - Type: determined
  - Component: `internal/nodeagent/heartbeat_connection_test.go`, `internal/nodeagent/tls_test.go`
  - Implementation:
    1. Merged 3 `TestBuildURL*` success-path tests into 1 table-driven `TestBuildURL` (3 cases).
    2. Merged `TestBootstrapTLS_InvalidCAFile` + `_MissingCAFile` into 1 table-driven `TestBootstrapTLS_CAFileErrors` (2 cases).
  - Reasoning: low (3 CFP)

- [x] 6.6 Strip redundant test doc comments across nodeagent test files
  - Type: determined
  - Component: all `internal/nodeagent/*_test.go` files
  - Implementation:
    1. Deleted 37 doc comments on test functions where the comment only restated the test name.
    2. Kept comments conveying non-obvious setup context, invariants, or contracts.
  - Reasoning: low (2 CFP)
