# Remove BuildGate-specific tables and HTTP mode

Scope: Remove the dedicated `buildgate_jobs` and `builds` tables, remote HTTP Build Gate mode, and the `/v1/buildgate/validate` API in favor of a single unified `jobs` queue. All execution units (mods, healers, pre-/re-/post-gate) become `jobs` rows consumed FIFO by nodes, with gate/build metadata stored in `jobs.meta` or derived metrics.

Documentation: ROADMAP.md, docs/build-gate/README.md, docs/api/OpenAPI.yaml, docs/api/components/schemas/controlplane.yaml, docs/envs/README.md, tests/e2e/mods/README.md, tests/e2e/mods/scenario-remote-buildgate/*

Legend: [ ] todo, [x] done.

## Collapse BuildGate and build tracking into jobs
- [x] Model gate/build metadata on jobs — Make `jobs` the single execution primitive for mods and gates
  - Repository: github.com/iw2rmb/ploy
  - Component: internal/store/schema.sql, internal/store/queries/jobs.sql, internal/store/jobs.sql.go, internal/store/models.go, internal/workflow/contracts, internal/workflow/runtime/step
  - Scope:
    - Extend `jobs.meta` usage to carry gate/build metadata that is currently implicit in `buildgate_jobs` and `builds` (e.g., tool, command, status details, gate outcome).
    - Define a small, documented JSON shape for gate/build metadata (e.g., `{ "kind": "gate|build|mod", "gate": { ... }, "build": { ... } }`) in `internal/workflow/contracts` and reference it from the code comments on `jobs.meta` in `internal/store/schema.sql`.
    - Ensure `internal/store/models.go` `Job.Meta` is treated as opaque JSONB but with helper functions in a dedicated package (e.g., `internal/domain/types/jobmeta.go`) for encoding/decoding gate/build metadata.
  - Snippets:
    - Example meta shape in Go:
      ```go
      type JobKind string

      const (
        JobKindMod   JobKind = "mod"
        JobKindGate  JobKind = "gate"
        JobKindBuild JobKind = "build"
      )

      type JobMeta struct {
        Kind  JobKind                 `json:"kind"`
        Gate  *contracts.BuildGateStageMetadata `json:"gate,omitempty"`
        Build *BuildMeta              `json:"build,omitempty"`
      }
      ```
  - Tests: `go test ./internal/store/... ./internal/workflow/...` — Verify `Job.Meta` round-trips gate/build metadata correctly and that existing job claim/scheduling tests still pass.

## Remove buildgate_jobs table and store APIs
- [ ] Drop buildgate_jobs from schema and sqlc layer — Avoid separate queue for Build Gate
  - Repository: github.com/iw2rmb/ploy
  - Component: internal/store/schema.sql, internal/store/queries/buildgate_jobs.sql, internal/store/buildgate_jobs.sql.go, internal/store/models.go, internal/store/querier.go, internal/store/status_conversion.go, internal/store/status_conversion_test.go
  - Scope:
    - Remove the `buildgate_job_status` enum and `buildgate_jobs` table definition from `internal/store/schema.sql`, including `buildgate_jobs_status_idx`.
    - Delete `internal/store/queries/buildgate_jobs.sql` and regenerate sqlc output, removing `internal/store/buildgate_jobs.sql.go`.
    - Remove `BuildgateJobStatus` and `BuildgateJob` types from `internal/store/models.go` and any `NullBuildgateJobStatus` helpers plus validation helpers in `internal/store/status_conversion.go`.
    - Delete `CreateBuildGateJob`, `ClaimBuildGateJob`, `GetBuildGateJob`, `ListPendingBuildGateJobs`, `AckBuildGateJobStart`, and `UpdateBuildGateJobCompletion` from `internal/store/querier.go` and the `Store` interface.
  - Snippets:
    - Schema removal in `internal/store/schema.sql`:
      ```sql
      -- Build Gate Jobs (async gate validation jobs)
      -- CREATE TABLE IF NOT EXISTS buildgate_jobs (...);
      -- DROP this section entirely in favor of jobs-based gate scheduling.
      ```
  - Tests: `go test ./internal/store/...` — Expect compilation errors until all references are removed; final run should pass with no references to `buildgate_jobs` or `BuildgateJobStatus`.

## Remove /v1/buildgate/validate and HTTP Build Gate client
- [ ] Delete HTTP Build Gate handlers and routes — Make node-run gate the only canonical path
  - Repository: github.com/iw2rmb/ploy
  - Component: internal/server/handlers/buildgate.go, internal/server/handlers/register.go, internal/server/handlers/buildgate_validate_test.go
  - Scope:
    - Remove `validateBuildGateHandler`, `getBuildGateJobStatusHandler`, `claimBuildGateJobHandler`, `completeBuildGateJobHandler`, and `ackBuildGateJobStartHandler` from `internal/server/handlers/buildgate.go`.
    - Delete the route registrations for `POST /v1/buildgate/validate`, `GET /v1/buildgate/jobs/{id}`, and `/v1/nodes/{id}/buildgate/*` endpoints from `internal/server/handlers/register.go`.
    - Remove associated tests in `internal/server/handlers/buildgate_validate_test.go` and any helpers that specifically target Build Gate HTTP endpoints.
  - Snippets:
    - Route removal in `internal/server/handlers/register.go`:
      ```go
      // s.HandleFunc("POST /v1/buildgate/validate", validateBuildGateHandler(st), auth.RoleControlPlane, auth.RoleWorker)
      // Remove all buildgate-specific handlers; nodes will only interact via jobs claim/complete endpoints.
      ```
  - Tests: `go test ./internal/server/handlers/...` — Ensure all buildgate HTTP tests are removed or updated and remaining handlers compile and pass.

- [ ] Remove HTTP Gate mode from runtime — Always use Docker-based gate executor
  - Repository: github.com/iw2rmb/ploy
  - Component: internal/workflow/runtime/step/gate_http_client.go, internal/workflow/runtime/step/gate_http.go, internal/workflow/runtime/step/gate_iface.go, internal/workflow/runtime/step/gate_factory.go, internal/nodeagent/execution_orchestrator.go, internal/nodeagent/execution_healing.go, internal/workflow/runtime/step/runner_gate_test.go
  - Scope:
    - Remove `BuildGateHTTPClient` and its implementation (`gate_http_client.go`) as well as `httpGateExecutor` (`gate_http.go`) and related tests.
    - Simplify `GateExecutor` interface in `gate_iface.go` to only cover the Docker-based executor and remove references to HTTP mode.
    - In `gate_factory.go`, drop `GateExecutorModeRemoteHTTP` and `PLOY_BUILDGATE_MODE`-based branching; `NewGateExecutor` should always return a Docker gate executor.
    - In `internal/nodeagent/execution_orchestrator.go`, remove `createGateExecutor`’s HTTP client path and environment variable checks; construct the gate executor directly from the container runtime.
    - Update `execution_healing.go` comments to describe only the Docker gate path as canonical and remove references to `/v1/buildgate/validate` equivalence.
    - Remove HTTP-mode-specific tests from `runner_gate_test.go` that assert calls to `/v1/buildgate/validate`.
  - Snippets:
    - Simplified factory in `gate_factory.go`:
      ```go
      func NewGateExecutor(mode string, rt ContainerRuntime, _ BuildGateHTTPClient) GateExecutor {
        return NewDockerGateExecutor(rt)
      }
      ```
  - Tests: `go test ./internal/workflow/runtime/step/... ./internal/nodeagent/...` — Confirm no references to `PLOY_BUILDGATE_MODE`, `BuildGateHTTPClient`, or HTTP gate remain and Docker gate tests still pass.

## Remove builds table and build_id foreign keys
- [ ] Drop builds table and build_id FKs from logs/artifact_bundles — Use job-level grouping only
  - Repository: github.com/iw2rmb/ploy
  - Component: internal/store/schema.sql, internal/store/queries/logs.sql, internal/store/queries/artifact_bundles.sql, internal/store/logs.sql.go, internal/store/artifact_bundles.sql.go, internal/store/models.go
  - Scope:
    - Remove the `builds` table definition and its indexes from `internal/store/schema.sql`.
    - Drop the `build_id` column and FK from `logs` and `artifact_bundles` tables; simplify unique index `logs_run_job_build_chunk_uniq` to `logs_run_job_chunk_uniq` (run_id, job_id, chunk_no).
    - Update `internal/store/queries/logs.sql` and `internal/store/queries/artifact_bundles.sql` to remove `build_id` fields from SELECT/INSERT clauses and regenerate sqlc Go files.
    - Remove `Build` type from `internal/store/models.go` and `BuildID` fields from `Log` and `ArtifactBundle` structs.
  - Snippets:
    - Schema change in `internal/store/schema.sql`:
      ```sql
      -- logs: drop build_id and adjust unique index
      CREATE TABLE IF NOT EXISTS logs (
        id        BIGSERIAL PRIMARY KEY,
        run_id    TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
        job_id    TEXT REFERENCES jobs(id) ON DELETE SET NULL,
        chunk_no  INTEGER NOT NULL,
        data      BYTEA NOT NULL CHECK (octet_length(data) <= 1048576),
        created_at TIMESTAMPTZ NOT NULL DEFAULT now()
      );
      CREATE UNIQUE INDEX IF NOT EXISTS logs_run_job_chunk_uniq ON logs(run_id, job_id, chunk_no);
      ```
  - Tests: `go test ./internal/store/...` — Ensure all log/artifact bundle paths compile and tests using `build_id` are updated or removed.

## Simplify node queueing to single jobs queue
- [ ] Ensure nodes claim from jobs only (FIFO by step_index) — Remove Build Gate-specific claim paths
  - Repository: github.com/iw2rmb/ploy
  - Component: internal/server/handlers/nodes_claim.go, internal/store/queries/jobs.sql, internal/store/jobs.sql.go, internal/store/claims_state_test.go, internal/store/runs.sql.go
  - Scope:
    - Confirm `ClaimJob` in `internal/store/queries/jobs.sql` already implements FIFO by `step_index` for `status='pending'` jobs and that nodes only use this claim path.
    - Remove any `ClaimBuildGateJob` call sites from server handlers (e.g., `nodes_*` handlers) as part of the buildgate_jobs removal.
    - Update comments in `nodes_claim.go` and related tests to describe a single unified queue with no Build Gate specialization.
  - Snippets:
    - Comment update in `internal/store/queries/jobs.sql`:
      ```sql
      -- Atomically claim the next pending job for a node (single unified queue).
      -- Jobs are ordered by step_index; no special handling for gate vs mod jobs.
      ```
  - Tests: `go test ./internal/server/handlers/... ./internal/store/...` — Verify claim behavior remains FIFO by `step_index` and Build Gate-specific claim tests are removed.

## Update OpenAPI and docs to remove HTTP Build Gate API
- [ ] Remove Build Gate HTTP endpoints from OpenAPI — Reflect jobs-only gate model
  - Repository: github.com/iw2rmb/ploy
  - Component: docs/api/OpenAPI.yaml, docs/api/components/schemas/controlplane.yaml, docs/api/paths/*
  - Scope:
    - Delete `/v1/buildgate/validate` and `/v1/buildgate/jobs/{id}` paths from `docs/api/OpenAPI.yaml` and remove `$ref` entries pointing to any buildgate-specific path YAML files.
    - Remove `BuildGateValidateRequest`, `BuildGateValidateResponse`, `BuildGateJobStatusResponse`, and `NodeBuildGateClaimResponse` schemas from `docs/api/components/schemas/controlplane.yaml`.
    - Ensure remaining job/run/log/artifact schemas do not reference `buildgate_jobs` or HTTP Build Gate semantics.
  - Snippets:
    - Path removal in `docs/api/OpenAPI.yaml`:
      ```yaml
      # /v1/buildgate/validate: removed; gate runs as jobs in the Mods pipeline.
      ```
  - Tests: `go test ./docs/api/...` or OpenAPI completeness tests — Confirm no missing references and that the spec validates without Build Gate HTTP endpoints.

- [ ] Update narrative docs and env reference — Document single-queue, no-HTTP-gate behavior
  - Repository: github.com/iw2rmb/ploy
  - Component: docs/build-gate/README.md, docs/mods-lifecycle.md, docs/envs/README.md, tests/e2e/mods/README.md, tests/e2e/mods/scenario-remote-buildgate/*
  - Scope:
    - Rewrite `docs/build-gate/README.md` to describe gate as part of the Mods pipeline backed solely by `jobs` (no `/v1/buildgate/validate`, no remote HTTP workers). Remove sequence diagrams and SQL examples that reference `buildgate_jobs`.
    - In `docs/mods-lifecycle.md`, update sections that mention Build Gate HTTP mode to refer to gate jobs in the unified queue.
    - Remove `PLOY_BUILDGATE_MODE` and `PLOY_BUILDGATE_WORKER_ENABLED` from `docs/envs/README.md` or mark them as removed, explaining that all nodes pull work from the same `jobs` queue.
    - Delete or repurpose `tests/e2e/mods/scenario-remote-buildgate` and related entries in `tests/e2e/mods/README.md` that assume HTTP Build Gate mode.
  - Snippets:
    - Env docs change in `docs/envs/README.md`:
      ```markdown
      - (removed) `PLOY_BUILDGATE_MODE` — Gate execution always runs as part of the Mods job pipeline; nodes claim work from the unified jobs queue.
      ```
  - Tests: `rg "buildgate_jobs" .` and `rg "PLOY_BUILDGATE_MODE" .` — Expect no remaining mentions outside of historical notes; `go test ./tests/e2e/mods/...` after updating/removing the remote-buildgate scenario.

