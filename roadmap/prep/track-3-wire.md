# Prep Track 3: Infra Candidate Wiring

Scope: Wire the `infra` recovery candidate artifact (`/out/gate-profile-candidate.json`, schema `gate_profile_v1`) into control-plane behavior so it is validated, consumed by the immediate follow-up `re_gate`, and promoted to repo `gate_profile` only on successful `re_gate`, without compatibility shims.

Documentation:
- `design/prep-impl.md`
- `design/prep.md`
- `design/prep-simple.md`
- `design/prep-complex.md`
- `design/prep-states.md`
- `docs/build-gate/README.md`
- `docs/migs-lifecycle.md`
- `docs/envs/README.md`

Legend: [ ] todo, [x] done.

## Phase 1: Candidate Artifact Ingestion
- [x] Capture expected recovery artifacts from heal outputs at job completion.
  - Repository: `ploy`
  - Component: nodeagent artifact upload + server job completion metadata
  - Scope:
    - Ensure `heal` completion uploads expected artifact files referenced by merged `artifact_paths` (already merged from expectations on claim).
    - Add deterministic extraction path for `/out/gate-profile-candidate.json` in job artifacts for downstream server-side resolution.
    - Keep behavior strict: missing expected artifact is treated as missing candidate (no fallback).
  - Primary files:
    - `internal/nodeagent/execution_orchestrator_jobs.go`
    - `internal/nodeagent/execution_orchestrator_jobs_upload.go`
    - `internal/server/handlers/jobs_complete.go`
  - Snippets:
    ```go
    // heal completion path
    artifactRef := findArtifactPath("/out/gate-profile-candidate.json")
    ```
  - Tests:
    - `go test ./internal/nodeagent -run 'TestUpload|TestExecuteHealingJob'`
    - `go test ./internal/server/handlers -run 'TestCompleteJob_'`

- [x] Resolve candidate artifact bytes for server-side validation flow.
  - Repository: `ploy`
  - Component: server artifact retrieval boundary
  - Scope:
    - Add helper to read candidate artifact content from persisted artifact bundle for a completed `heal` job.
    - Return explicit typed errors: not found, unreadable, invalid json payload.
  - Primary files:
    - `internal/server/blobpersist/*`
    - `internal/server/handlers/nodes_complete_healing.go`
  - Snippets:
    ```go
    rawCandidate, err := loadRecoveryArtifact(ctx, healJobID, "/out/gate-profile-candidate.json")
    ```
  - Tests:
    - `go test ./internal/server/handlers -run 'TestMaybeCreateHealingJobs'`

## Phase 2: Validation and Runtime Use in Re-Gate
- [x] Validate infra candidate against prep schema boundary on healing insertion.
  - Repository: `ploy`
  - Component: server healing insertion + prep schema validator
  - Scope:
    - In `maybeCreateHealingJobs(...)`, when `error_kind=infra` and expectations declare `gate_profile_v1`, resolve candidate artifact from previous `heal` output.
    - Validate candidate via `prep.ValidateProfileJSONForSchema(raw, "gate_profile_v1")`.
    - Persist validation status and candidate payload reference in `re_gate` job metadata.
    - No fallback behavior; invalid candidate is recorded and treated as unusable for promotion/override.
  - Primary files:
    - `internal/server/handlers/nodes_complete_healing.go`
    - `internal/server/prep/schema.go`
    - `internal/workflow/contracts/job_meta.go`
  - Snippets:
    ```go
    if err := prep.ValidateProfileJSONForSchema(rawCandidate, contracts.GateProfileCandidateSchemaID); err != nil {
        recoveryMeta.CandidateValidationError = err.Error()
    }
    ```
  - Tests:
    - `go test ./internal/server/prep`
    - `go test ./internal/server/handlers -run 'TestMaybeCreateHealingJobs'`

- [x] Apply validated candidate as highest-precedence re-gate prep override source.
  - Repository: `ploy`
  - Component: claim-time spec merge for gate jobs
  - Scope:
    - Extend claim merge pipeline for `re_gate` jobs:
      1. explicit `build_gate.<phase>.gate_profile` from spec
      2. validated infra candidate override (from recovery metadata)
      3. persisted repo `gate_profile`
      4. default tool command
    - Candidate only affects `re_gate`; do not alter `pre_gate`/`post_gate` claim logic in this track.
  - Primary files:
    - `internal/server/handlers/nodes_claim.go`
    - `internal/workflow/contracts/gate_profile.go`
  - Snippets:
    ```go
    mergedSpec = mergeRecoveryCandidateIntoSpec(mergedSpec, reGateRecoveryMeta, jobType)
    ```
  - Tests:
    - `go test ./internal/server/handlers -run 'TestClaimJob_'`
    - `go test ./internal/workflow/contracts -run 'TestGateProfile'`

## Phase 3: Promotion on Successful Re-Gate
- [x] Promote validated infra candidate to repo `gate_profile` only when corresponding `re_gate` succeeds.
  - Repository: `ploy`
  - Component: server job completion + repo profile persistence
  - Scope:
    - On successful `re_gate` completion, if recovery metadata contains validated infra candidate payload, persist it to `mig_repos.gate_profile` (+ optional artifact metadata update) transactionally.
    - Never promote on failed `re_gate`.
    - Promotion is idempotent for retries/replays by guarding on job id + status.
  - Primary files:
    - `internal/server/handlers/jobs_complete.go`
    - `internal/server/handlers/nodes_complete_healing.go`
    - `internal/store/queries/mig_repos.sql`
  - Snippets:
    ```go
    if jobType == re_gate && status == Success && candidate.Validated {
        st.UpdateMigRepoGateProfile(ctx, ...)
    }
    ```
  - Tests:
    - `go test ./internal/server/handlers -run 'TestCompleteJob_GateFailure_HealingInsertionRewiresNextChain|TestCompleteJob_'`
    - `go test ./internal/store/...`

- [x] Persist promotion audit fields in job/recovery metadata.
  - Repository: `ploy`
  - Component: contracts + API projection
  - Scope:
    - Extend recovery metadata projection with fields needed for observability:
      - candidate schema id
      - candidate artifact path
      - validation status/error
      - promoted bool
    - Expose via existing repo jobs API projection.
  - Primary files:
    - `internal/workflow/contracts/build_gate_metadata.go`
    - `internal/server/handlers/runs_repo_jobs.go`
  - Tests:
    - `go test ./internal/workflow/contracts -run 'TestBuildGateStageMetadata|TestJobMeta'`
    - `go test ./internal/server/handlers -run 'TestListRunRepoJobs'`

## Phase 4: CLI/Docs Wiring Clarity
- [x] Finalize healing fragment examples for infra/code/router using `spec_path`.
  - Repository: `ploy`
  - Component: CLI docs + examples
  - Scope:
    - Document end-to-end expectation that infra healing writes `/out/gate-profile-candidate.json`.
    - Document that candidate promotion is conditional on successful `re_gate`.
  - Primary files:
    - `cmd/ploy/README.md`
    - `docs/schemas/mig.example.yaml`
    - `docs/migs-lifecycle.md`
    - `docs/envs/README.md`
  - Tests:
    - `go test ./cmd/ploy -run 'TestBuildSpecPayload_|TestRunSubmit'`

## Validation
- [x] Run full validation suite for track-3 slice.
  - Repository: `ploy`
  - Component: CI/local validation
  - Scope: Execute:
    - `go test ./internal/workflow/contracts -run 'TestBuildGate|TestJobMeta|TestGateProfile|TestParseModsSpec'`
    - `go test ./internal/nodeagent -run 'TestExecuteHealingJob|TestUpload|TestParseSpec_ProducesTypedOptions'`
    - `go test ./internal/server/handlers -run 'TestMaybeCreateHealingJobs|TestCompleteJob_|TestClaimJob_|TestListRunRepoJobs'`
    - `go test ./internal/server/prep`
    - `go test ./cmd/ploy -run 'TestBuildSpecPayload_|TestRunSubmit'`
    - `make test`
    - `make vet`
    - `make staticcheck`
  - Snippets: `N/A`
  - Tests: All commands above pass.
