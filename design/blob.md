# Garage Blob Flow Unification DD

## Summary
Unify Garage-related blob store/download/stream behavior by extending existing packages (`internal/server/blobpersist`, `internal/server/handlers`, `internal/nodeagent`) instead of adding new modules. The outcome is one canonical server-side blob I/O contract, fewer duplicated branches, clearer names, and table-driven tests that cover all current use-cases.

## Scope
In scope:
- Server-side Garage writes for logs, diffs, artifact bundles, spec bundles, gate profiles, and default gate catalog profiles.
- Server-side Garage reads/streams for artifact download, diff download, spec bundle download, and log backfill SSE.
- Blob reads used by server internals (`LoadRecoveryArtifact`, `ExtractSBOMRowsForJob`, diff clone source read).
- Nodeagent upload/download helpers that participate directly in Garage object flows through server APIs (`UploadDiff`, `UploadArtifact*`, `DownloadSpecBundle`, `FetchRunRepoDiffPatch`).
- Nodeagent workspace rehydration object-flow wiring (repo diff fetch/download + patch application entrypoints).
- Nodeagent `/tmp` bundle materialization and `/in`/`/out` execution directory flows where Garage-backed bundles are downloaded or persisted.
- Naming cleanup where ambiguity currently increases duplication risk.
- Test unification with table-driven suites and shared helpers in existing test files/packages.

Out of scope:
- SQL schema redesign or query-shape redesign in `internal/store`.
- API wire contract changes.
- Object store provider changes (`internal/blobstore/s3` stays the backend).
- Non-Garage orchestration logic unrelated to blob storage/read/stream.

Assumption:
- No backward-compatibility shims are required for internal function surfaces.

## Why This Is Needed
Current Garage behavior is correct in many paths but duplicated across handlers and services. Duplicate read/write/error branches already produced drift:
- Write patterns differ between blobpersist-backed handlers and gate-profile/catalog paths.
- Read/stream error mapping and key validation are repeated in multiple handlers.
- Internal readers in blobpersist duplicate open/read/close logic.
- Nodeagent has duplicated HTTP blob upload mechanics between uploader and log streamer.

This increases maintenance cost and makes future changes risky.

## Goals
- Keep one canonical server owner for Garage I/O semantics: `internal/server/blobpersist`.
- Keep handlers focused on HTTP validation/response, not low-level Garage read/write branches.
- Use shared helper functions and table-driven tests as the default pattern.
- Cover all current Garage use-cases with explicit adoption list.
- Reduce naming ambiguity (`bs`, `streamBlob`, `persistBlob`) that obscures ownership.
- Reduce duplicated LOC as a secondary success signal.

## Non-goals
- Introduce new top-level modules/packages for this work.
- Rework store query families in the same change.
- Change object key formats or external endpoint paths.
- Add compatibility wrappers for removed internal helpers.

## Current Baseline (Observed)
Codebase inspected twice (broad call-site pass + focused per-flow pass).

### Garage write paths
- Blobpersist coordinated writes:
  - `internal/server/blobpersist/service.go` (`CreateLog`, `CreateDiff`, `CreateArtifactBundle`, `CreateSpecBundle`, shared `persistBlob`).
- Direct Garage writes outside blobpersist:
  - `internal/server/handlers/gate_profile_persistence.go` (`persistGateProfilePayload`).
  - `internal/server/handlers/gate_profile_resolver.go` (fallback promotion copy write).
  - `cmd/ployd/gate_catalog_seed.go` (default profile upload).

### Garage read/stream paths
- Direct blob reads in handlers:
  - `internal/server/handlers/artifacts_download.go`.
  - `internal/server/handlers/diffs.go` (download mode).
  - `internal/server/handlers/spec_bundles.go`.
  - `internal/server/handlers/events.go` (SSE backfill of gzipped logs).
- Shared stream helper exists but is only partially adopted:
  - `internal/server/handlers/ingest_common.go` (`streamBlob`).
- Blobpersist internal readers:
  - `internal/server/blobpersist/service.go` (`CloneLatestDiffByJob`, `LoadRecoveryArtifact`).
  - `internal/server/blobpersist/sbom.go` (`ExtractSBOMRowsForJob`).

### Nodeagent garage-adjacent paths
- Upload/download helpers:
  - `internal/nodeagent/uploaders.go` (`UploadDiff`, `UploadArtifact*`, `DownloadSpecBundle`).
- Diff fetch/download path used by rehydration:
  - `internal/nodeagent/difffetcher.go` (`ListRunRepoDiffs`, `FetchRunRepoDiffPatch`, `FetchDiffsForJobRepo`).
- Rehydration + tmp materialization entrypoints:
  - `internal/nodeagent/execution_orchestrator_rehydrate.go`.
  - `internal/nodeagent/execution_orchestrator_tmpbundle.go`.
  - `internal/nodeagent/execution_orchestrator_jobs.go` (`withMaterializedTmpBundle`, temp dir setup).
- `/out` persistence and configured artifact path handling:
  - `internal/nodeagent/execution_orchestrator_jobs_upload.go` (`uploadOutDirBundle`, `uploadConfiguredArtifacts`).
- Independent chunk upload path:
  - `internal/nodeagent/logstreamer.go` (`sendChunk`) duplicates HTTP post mechanics.

### Naming and ambiguity
- `bs` and `blobstore` names are overloaded across handlers/services.
- `persistBlob` currently implies generic behavior but is gzip+DB-specific.
- `streamBlob` name hides that it always writes attachment headers.

### Test duplication
- Stub/fake patterns duplicated across:
  - `internal/server/blobpersist/service_test.go`.
  - `internal/server/handlers/gate_profile_resolver_test.go`.
  - `internal/nodeagent/uploaders_test.go` + `internal/nodeagent/testutil_mockservers_test.go`.
- Coverage for Garage read error mapping is spread across handler tests instead of a single canonical table.

## Target Contract / Target Architecture

### Ownership rules
- `internal/server/blobpersist` becomes the canonical owner of Garage read/write semantics on server side.
- Handlers may keep HTTP-specific behavior, but Garage open/read/write/error mapping must call blobpersist helpers.
- Direct `bs.Put`/`bs.Get` in handlers should be eliminated for Garage object flows covered in scope.
- `internal/nodeagent` keeps transport behavior but reuses one uploader HTTP helper path for blob uploads.
- `internal/nodeagent` keeps execution semantics, but Garage-backed transfer mechanics (upload/download/request/status handling) use one canonical helper surface.

### Canonical server contracts (extend existing package)
Add/rename helpers in `internal/server/blobpersist`:
- `persistCompressedObjectWithRollback(...)` (existing `persistBlob` renamed for clarity).
- `uploadObjectThenFinalize(...)` for flows requiring object-first write then DB finalize (gate profiles, default catalog).
- `openObjectByKey(ctx, key) (rc, size, err)` with key validation and normalized not-found/unavailable mapping.
- `readAllObjectByKey(ctx, key) ([]byte, err)` using `openObjectByKey`.
- `streamObjectByKey(ctx, key, streamFn)` where streamFn remains handler-owned for headers.

### Error taxonomy rules
- Introduce typed blobpersist errors for server call sites:
  - missing/empty key,
  - not found,
  - unreadable/unavailable.
- Handlers map these to endpoint-specific HTTP statuses without duplicating low-level checks.

### Naming rules
- Prefer `objects` (or `objectStore`) over `bs` in new/modified code.
- Rename `streamBlob` to `writeBlobAttachment` (or equivalent explicit name).
- Rename `persistBlob` to `persistCompressedObjectWithRollback`.
- Keep `objectKey` term for DB/url field parity; use `blobKey` only in local helper internals if needed.

## Implementation Notes

### 1) Extend blobpersist write helpers
- Keep current `CreateLog/CreateDiff/CreateArtifactBundle/CreateSpecBundle` behavior.
- Extract gate-profile and catalog write-finalize flows into blobpersist helper(s):
  - upload object,
  - finalize DB record/link,
  - delete uploaded object when finalize fails.
- Adopt helper in:
  - `internal/server/handlers/gate_profile_persistence.go`.
  - `internal/server/handlers/gate_profile_resolver.go`.
  - `cmd/ployd/gate_catalog_seed.go`.

### 2) Extend blobpersist read helpers and adopt in handlers
- Add open/read helper functions in blobpersist and route these paths through them:
  - `artifacts_download.go`.
  - `diffs.go` download mode.
  - `spec_bundles.go` download.
  - `events.go` log backfill chunk read.
- Keep HTTP-level response formatting local to handlers.

### 3) Route blobpersist internal readers through one helper path
- Migrate in-package readers:
  - `CloneLatestDiffByJob`.
  - `LoadRecoveryArtifact`.
  - `ExtractSBOMRowsForJob`.
- Keep bundle/tar parsing logic local where domain-specific.

### 4) Nodeagent upload path simplification (Garage-focused only)
- Reuse canonical request helpers for:
  - log chunk upload (`logstreamer`),
  - spec bundle download (`DownloadSpecBundle`),
  - diff listing + diff patch download (`difffetcher`).
- Keep `logstreamer` chunking/compression behavior and rehydration patch-apply semantics; remove duplicated request/status boilerplate.
- Keep `/in` and `/tmp` materialization behavior stable while reducing duplicated temp-dir/materialization scaffolding.
- Keep `/out` upload semantics stable while consolidating path-resolution and upload call plumbing.
- Do not add new nodeagent package.

### 5) Test unification (table-driven first)
- In `internal/server/blobpersist/service_test.go`:
  - add table-driven suite for key validation + not-found/unreadable mapping.
  - add table-driven suite for upload-then-finalize cleanup behavior.
- In `internal/server/handlers/*blob*` tests:
  - keep endpoint status/header assertions,
  - remove repeated low-level blob error branch setup when covered by blobpersist tests.
- In `internal/nodeagent/uploaders_test.go` and `testutil_mockservers_test.go`:
  - centralize upload server behavior with table-driven cases per endpoint family.

## Milestones

### Milestone 1: Canonical Blob Read Path
Scope:
- Extend blobpersist read helpers.
- Adopt in artifact/diff/spec download handlers and SSE backfill.

Expected results:
- No direct handler-side `bs.Get` branches in scoped files.
- One canonical key/error mapping path.

Testable outcome:
- `go test ./internal/server/blobpersist ./internal/server/handlers -run 'Blob|Artifact|Diff|SpecBundle|Logs'`

### Milestone 2: Canonical Blob Write-Finalize Path
Scope:
- Add upload-then-finalize helper in blobpersist.
- Adopt in gate profile persistence/resolver and gate catalog seed.

Expected results:
- No ad-hoc object-write + DB-finalize duplication in scoped files.
- Cleanup semantics for finalize failure are explicit and tested.

Testable outcome:
- `go test ./internal/server/blobpersist ./internal/server/handlers ./cmd/ployd -run 'GateProfile|Catalog|Blob'`

### Milestone 3: Nodeagent Garage Upload Helper Unification
Scope:
- Reuse canonical request helpers across nodeagent Garage object transfer paths.
- Cover upload (`UploadDiff`, `UploadArtifact*`, log chunks) and download (`DownloadSpecBundle`, diff list/patch fetch) flows.
- Consolidate `/tmp` materialization and `/in`/`/out` execution directory plumbing where duplicated scaffolding exists.
- Keep behavior stable.

Expected results:
- Less duplicated HTTP request/response handling in nodeagent Garage transfer code.
- Rehydration and temp bundle materialization keep one canonical object-flow wiring path.

Testable outcome:
- `go test ./internal/nodeagent ./internal/workflow/step -run 'Uploader|LogStreamer|DiffFetcher|TmpBundle|Artifact|Rehydrate'`

### Milestone 4: Naming + Test Cleanup
Scope:
- Apply agreed renames in touched files.
- Convert duplicated cases to table-driven tests.

Expected results:
- Lower ambiguity in function/variable naming.
- Shared fixtures replace repeated setup blocks.

Testable outcome:
- `go test ./internal/server/blobpersist ./internal/server/handlers ./internal/nodeagent ./cmd/ployd`

## Acceptance Criteria
- All Garage read/write/stream use-cases listed in scope call canonical blobpersist helpers (except `internal/blobstore/s3` backend implementation).
- No scoped handler files keep direct low-level Garage `Get`/`Put` branches.
- Gate-profile and catalog Garage writes use shared finalize/cleanup flow.
- Nodeagent blob upload code has one shared HTTP post path for uploader/log chunk requests.
- Nodeagent diff/spec-bundle download and rehydration object fetch paths use shared transfer helpers.
- Nodeagent `/tmp` bundle materialization and `/out` persistence paths keep one canonical upload/download wiring path per flow family.
- New/updated tests in scoped packages are primarily table-driven.
- Commands pass:
  - `go test ./internal/server/blobpersist ./internal/server/handlers ./internal/nodeagent ./internal/workflow/step ./cmd/ployd`

## Risks
- Error mapping consolidation could accidentally change endpoint status behavior if handler mapping tables are not explicit.
- Moving gate-profile/catalog writes may expose latent assumptions about object-key timing.
- Over-aggressive helper extraction could hide endpoint-specific semantics; keep HTTP response logic local.

## Estimated LOC Influence (secondary metric)
Estimated net change after full milestone completion:
- Server blobpersist + handler simplification: `-120` to `-220` LOC.
- Gate-profile/catalog write path unification: `-40` to `-90` LOC.
- Nodeagent upload/download + rehydration I/O helper reuse: `-70` to `-160` LOC.
- Added helper/test code: `+170` to `+280` LOC.
- Net estimate: `-90` to `-240` LOC with lower branch duplication.

This LOC estimate is secondary; primary success is canonical ownership and reduced duplicate branches.

## References
- `internal/server/blobpersist/service.go`
- `internal/server/blobpersist/sbom.go`
- `internal/server/handlers/artifacts_download.go`
- `internal/server/handlers/diffs.go`
- `internal/server/handlers/spec_bundles.go`
- `internal/server/handlers/events.go`
- `internal/server/handlers/gate_profile_persistence.go`
- `internal/server/handlers/gate_profile_resolver.go`
- `cmd/ployd/gate_catalog_seed.go`
- `internal/nodeagent/uploaders.go`
- `internal/nodeagent/difffetcher.go`
- `internal/nodeagent/logstreamer.go`
- `internal/nodeagent/execution_orchestrator_rehydrate.go`
- `internal/nodeagent/execution_orchestrator_tmpbundle.go`
- `internal/nodeagent/execution_orchestrator_jobs.go`
- `internal/nodeagent/execution_orchestrator_jobs_upload.go`
- `internal/workflow/step/container_spec.go`
- `roadmap/reduct.md` (items `3.2`, related redundancy scope)
- `docs/envs/README.md` (Object Store section)
