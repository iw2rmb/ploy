# Internal Redundancy Reduction Roadmap

Scope: Reduce cross-module redundancy in `internal/` by consolidating duplicated orchestration, gate-profile, blob-access, handler-contract, and test-harness logic without backward-compatibility constraints.

Documentation: `roadmap/reduct.md`, `README.md`, `internal/server/README.md`, `internal/tui/README.md`, `internal/client/README.md`.

- [x] 3.1a Collapse duplicated log/diff list APIs into selector-based store methods
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

- [x] 3.1b Collapse duplicated artifact/event list APIs into selector-based store methods
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
  - Completion notes:
    - **Removed duplicate entrypoints**: No `*Meta*` artifact or event list methods exist anywhere in the codebase. The pre-refactor `List*Meta*` parallel family is fully deleted.
    - **Selector-only querier surface** (`internal/store/querier.go`): Artifact list methods — `ListArtifactBundlesByRun` (line 154), `ListArtifactBundlesByRunAndJob` (line 158), `ListArtifactBundlesByCID` (line 161) — all return `[]ArtifactBundle` with explicit column projection (no `SELECT *`). Event list methods — `ListEventsByRun` (line 172), `ListEventsByRunSince` (line 175) — all return `[]Event` with explicit column projection.
    - **SQL query surface** (`internal/store/queries/artifact_bundles.sql`, `internal/store/queries/events.sql`): Every list query uses explicit `SELECT id, run_id, job_id, name, bundle_size, object_key, cid, digest, created_at` / `SELECT id, run_id, job_id, time, level, message, meta` — `SELECT *` is absent from all list paths.
    - **Handler call sites use selector methods only**: `artifacts_download.go:30` → `ListArtifactBundlesByCID`; `artifacts_repo.go:58` → `ListArtifactBundlesByRun`; `migs_ticket.go:153` → `ListArtifactBundlesByRunAndJob`. No legacy Meta-suffixed calls present.
    - **Test proof** (`internal/store/list_meta_queries_test.go`): `TestArtifactSelectorBehavior` (lines 56–106) asserts explicit column inclusion (`object_key`, `bundle_size`, `cid`, `digest`, `created_at`) and deterministic ordering for all three artifact list queries. `TestEventSelectorBehavior` (lines 112–156) asserts explicit column inclusion (`level`, `message`, `meta`, `time`) and ordering for both event list queries.

- [x] 3.1c Finalize selector-only store surface and remove remaining duplicate list entrypoints
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
  - Completion notes:
    - **No residual duplicate list entrypoints**: After 3.1a and 3.1b, the store surface was already clean — no `*Meta*`-suffixed list methods exist anywhere in `querier.go`, generated SQL files, or handler mocks. No compatibility wiring or proxy aliases were found.
    - **Selector-only querier surface** (`internal/store/querier.go`): All blob-type list methods (`ListLogsByRun*`, `ListDiffsByRun*`, `ListArtifactBundles*`, `ListEventsByRun*`) use explicit column projection — no `SELECT *` and no parallel Meta-suffixed entrypoints.
    - **Canonical selector-path tests consolidated** (`internal/store/list_meta_queries_test.go`): Replaced `TestListMetaQueriesDoNotReturnBlobs` (blob-exclusion only, no ordering) with `TestDiffSelectorBehavior` following the same pattern as `TestArtifactSelectorBehavior` and `TestEventSelectorBehavior`: checks explicit column selection (no SELECT *), patch blob exclusion, required metadata columns (`object_key`, `patch_size`, `summary`, `created_at`), and deterministic ordering.
    - **Transitional duplicate ordering tests removed** (`internal/store/list_queries_ordering_test.go`): Deleted the duplicate ordering assertions for `ListDiffsByRun`, `ListDiffsByRunRepo`, `ListArtifactBundlesByRun`, `ListArtifactBundlesByRunAndJob`, `ListArtifactBundlesByCID`, `ListEventsByRun`, and `ListEventsByRunSince` — all now covered comprehensively by the canonical selector behavior tests in `list_meta_queries_test.go`. Updated file comment to reflect the canonical test locations.
    - **Test proof**: `go test ./internal/store/... -run 'Log|Diff|List|Artifact|Event'` passes; `go test ./internal/store/...` passes; `go test ./internal/server/handlers/... -run 'Artifact|Diff|Event|Log'` passes.

- [x] 3.2 Standardize blob transfer/read flow across handlers, blobpersist, and nodeagent rehydration I/O
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
  - Completion notes:
    - **Shared server blob-read helper family** (`internal/server/handlers/ingest_common.go`): Added `openBlobForHTTP(w, r, bs, key *string, entityName string, logAttrs ...any) (io.ReadCloser, int64, bool)` — centralizes nil-key check → 404, `blobstore.ErrNotFound` → 404, other Get errors → 503, with structured slog on every failure path. Added `blobstore.ReadAll(ctx, bs, key) ([]byte, error)` in `internal/blobstore/blobstore.go` for non-HTTP reads.
    - **HTTP handler blob-read branches removed**: `artifacts_download.go` (11 lines), `diffs.go` (15 lines, also routes through `streamBlob` instead of inline `io.Copy`), `spec_bundles.go` (12 lines) — all now delegate to `openBlobForHTTP` + `streamBlob`.
    - **Non-HTTP blob-read branches removed**: `gate_profile_resolver.go:loadObject` body reduced to `return blobstore.ReadAll(ctx, r.bs, key)` (7 lines removed); `blobpersist/service.go:CloneLatestDiffByJob` (7 lines removed); `blobpersist/service.go:LoadRecoveryArtifact` (7 lines removed, using `bytes.NewReader` to pass buffered bytes to `readArtifactFromTarGz`); `blobpersist/sbom.go` (7 lines removed).
    - **Nodeagent transfer helpers**: Added `getBytesFromURL(ctx, fullURL, action) ([]byte, error)` method on `*baseUploader` in `internal/nodeagent/http.go` — covers GET + `httpx.CheckStatus(200)` + `io.ReadAll` + `httpx.DrainAndClose`. Added `postJSONBytes(ctx, client, u, body, action) error` package-level function for POST with pre-marshaled JSON accepting 200 or 201. `uploaders.go:DownloadSpecBundle` (16 lines → 2), `difffetcher.go:FetchRunRepoDiffPatch` (20 lines → 5), `logstreamer.go:sendChunk` (10 lines → 2) all delegate to these helpers.
    - **Stable semantics**: `backfillOneChunk` (streaming gzip reader) and `extractTmpBundle` / `withMaterializedTmpBundle` (tmp extraction + materialization) unchanged — streaming and `/tmp`/`/in`/`/out` wiring preserved as specified.
    - **Test proof**: `go test ./internal/server/handlers ./internal/server/blobpersist` passes; `go test ./internal/nodeagent ./internal/workflow/step -run 'Uploader|LogStreamer|DiffFetcher|TmpBundle|Artifact|Rehydrate'` passes; `go test ./internal/... -run 'Artifact|Diff|SpecBundle|Blob|SBOM|Rehydrate|TmpBundle'` passes (all packages).

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
