# Server-driven Jobs + Exit Codes: Follow-ups

> When following this template:
> - Align to the template structure
> - Include steps to update relevant docs

Scope: Refine the new server-driven job model and exit-code plumbing across the control plane, node agent, and documentation. Fix enum defaults, make job completion and run completion semantics consistent with Build Gate + healing, remove redundant job updates, and align OpenAPI/docs/mods lifecycle to the new job-centric model.

Documentation:  
- Schema and store: `SCHEMA.sql`, `internal/store/schema.sql`, `internal/store/models.go`, `internal/store/queries/*.sql`, `internal/store/status_conversion.go`  
- Control plane handlers: `internal/server/handlers/nodes_complete.go`, `nodes_claim.go`, `nodes_ack.go`, `handlers_mods_ticket.go`, `handlers_jobs_diff.go`, `handlers_jobs_artifact.go`  
- Node agent: `internal/nodeagent/handlers.go`, `execution_orchestrator.go`, `execution_upload.go`, `statusuploader.go`, `claimer_loop.go`, `execution_healing.go`  
- Mods domain + lifecycle: `internal/mods/api/types.go`, `internal/mods/api/status_conversion.go`, `docs/mods-lifecycle.md`  
- OpenAPI + path docs: `docs/api/OpenAPI.yaml`, `docs/api/components/schemas/controlplane.yaml`, `docs/api/paths/nodes_id_complete.yaml`, `docs/api/paths/runs_run_id_jobs_job_id_diff.yaml`, `docs/api/paths/runs_run_id_jobs_job_id_artifact.yaml`, `docs/api/paths/diffs_id.yaml`, `docs/api/paths/artifacts_id.yaml`

Legend: [ ] todo, [x] done.

## 1. Fix `builds.status` enum default and remove `'pending'` from `job_status`

- [x] Align `builds.status` with `job_status` and drop `'pending'` from the enum — prevent schema apply failures and keep status vocabulary consistent
  - Component: server store schema (`SCHEMA.sql`, `internal/store/schema.sql`), generated models (`internal/store/models.go`), status helpers (`internal/store/status_conversion.go`), any tests referencing `JobStatusSkipped`/`JobStatusCreated`.
  - Scope:
    - Change `job_status` enum definition in both schema files:
      - From:
        - `CREATE TYPE job_status AS ENUM ('created', 'scheduled', 'running', 'succeeded', 'failed', 'skipped', 'canceled');`
      - To (drop `skipped`, keep only states actually used in code; if you want to keep `skipped`, see step 2.3):
        - ```sql
          CREATE TYPE job_status AS ENUM (
            'created',
            'scheduled',
            'running',
            'succeeded',
            'failed',
            'canceled'
          );
          ```
        - If you decide to keep `skipped`, update later steps accordingly.
    - Fix `builds.status` default so it is valid for `job_status`:
      - In `SCHEMA.sql` and `internal/store/schema.sql`, update the `builds` table:
        - From:
          - ```sql
            status       job_status NOT NULL DEFAULT 'pending',
            ```
        - To:
          - ```sql
            status       job_status NOT NULL DEFAULT 'created',
            ```
      - Confirm there is no remaining use of a literal `"pending"` for `builds.status` in Go code (search for `"pending"` and `BuildgateJobStatusPending` vs `job_status`).
    - Regenerate sqlc models if necessary:
      - Run `sqlc generate` so `internal/store/models.go` reflects the updated `job_status` enum (it should no longer allow `JobStatusSkipped` if you removed it).
    - Adjust status helper to match enum:
      - In `internal/store/status_conversion.go`, update:
        - `ConvertToJobStatus` mapping so it no longer returns `JobStatusSkipped` if you dropped that value.
        - `ValidateJobStatus` so the `switch` only accepts the enum values still present in `job_status`.
      - Example snippet:
        ```go
        func ValidateJobStatus(status string) (JobStatus, error) {
        	s := JobStatus(status)
        	switch s {
        	case JobStatusCreated, JobStatusScheduled, JobStatusRunning,
        		JobStatusSucceeded, JobStatusFailed, JobStatusCanceled:
        		return s, nil
        	default:
        		return "", fmt.Errorf("invalid job status: %q (expected: created, scheduled, running, succeeded, failed, canceled)", status)
        	}
        }
        ```
    - Update any tests that explicitly refer to `JobStatusSkipped`:
      - `internal/store/status_conversion_test.go`
      - `internal/mods/api/status_conversion_test.go`
      - Remove or rewrite cases that mention `skipped` if the enum no longer supports it.
  - Test:
    - Schema + store:
      - `go test ./internal/store/...` — `SCHEMA.sql` / `internal/store/schema.sql` should embed and generate cleanly; tests that rely on status conversion must still pass.
    - Integration smoke:
      - `make test` — verify no enum-related runtime failures when creating or updating builds.

## 2. Simplify `maybeCompleteMultiStepRun` and base run status on final gate result

- [x] Remove redundant job mutation from `maybeCompleteMultiStepRun` and compute run status from job outcomes in a gate-aware way — avoid rewriting per-job terminal states and make healing semantics correct
  - Component: control-plane handlers (`internal/server/handlers/nodes_complete.go`), mods API + events (`internal/mods/api/status_conversion.go`, `internal/server/events/service.go`), tests (`internal/server/handlers/server_runs_complete_test.go`, `test_mock_store_test.go`).
  - Scope:
    - In `internal/server/handlers/nodes_complete.go`, locate `maybeCompleteMultiStepRun` (search for `func maybeCompleteMultiStepRun`).
    - Remove the post-completion block that mutates job 0:
      - Delete the `ListJobsByRun` call and the subsequent `UpdateJobStatus` call (around lines 360–387), leaving run completion + event publishing intact.
      - Before:
        ```go
        // Transition the run to its terminal status.
        err = st.UpdateRunCompletion(ctx, store.UpdateRunCompletionParams{ ... })
        if err != nil { ... }

        // Update job status to terminal and set finished_at/duration.
        if jobs, err := st.ListJobsByRun(ctx, runID); err == nil && len(jobs) > 0 {
            // ...
            _ = st.UpdateJobStatus(ctx, store.UpdateJobStatusParams{ ... })
        }
        ```
      - After:
        ```go
        err = st.UpdateRunCompletion(ctx, store.UpdateRunCompletionParams{
        	ID:     runID,
        	Status: runStatus,
        	Stats:  []byte("{}"),
        })
        if err != nil {
        	return fmt.Errorf("update run completion: %w", err)
        }
        ```
    - Change run-status derivation to use the “final gate result wins” rule:
      - Fetch all jobs once and derive:
        - The last gate-related job (`mod_type` in job meta is `pre_gate`, `post_gate`, or `re_gate`).
        - Whether any non-gate jobs (mods, heal jobs) failed or were canceled.
      - Extend `maybeCompleteMultiStepRun` to:
        - Decode `modsapi.StageMetadata` from each `job.Meta` to get `ModType`.
        - Track:
          - `hasNonGateFailure` (mods/heal jobs with `failed` or `canceled`).
          - `lastGateStatus` and its `StepIndex` (largest `step_index` among gate jobs).
        - Determine run status:
          - If `hasNonGateFailure`: `RunStatusFailed`.
          - Else if `lastGateStatus == JobStatusFailed`: `RunStatusFailed`.
          - Else if there are any canceled jobs (no failures): `RunStatusCanceled`.
          - Else: `RunStatusSucceeded`.
      - Example (simplified) pseudocode snippet:
        ```go
        jobs, err := st.ListJobsByRun(ctx, runID)
        if err != nil { return fmt.Errorf("list jobs: %w", err) }
        if len(jobs) == 0 { return fmt.Errorf("run has no jobs") }

        var (
        	terminalJobs      int64
        	hasNonGateFailure bool
        	lastGateIndex     float64
        	lastGateStatus    store.JobStatus
        	hasCanceled       bool
        )

        for _, job := range jobs {
        	switch job.Status {
        	case store.JobStatusSucceeded, store.JobStatusFailed, store.JobStatusCanceled:
        		terminalJobs++
        	}

        	var meta modsapi.StageMetadata
        	_ = json.Unmarshal(job.Meta, &meta)
        	isGate := meta.ModType == "pre_gate" || meta.ModType == "post_gate" || meta.ModType == "re_gate"

        	if isGate {
        		if job.StepIndex >= lastGateIndex {
        			lastGateIndex = job.StepIndex
        			lastGateStatus = job.Status
        		}
        		continue
        	}

        	// Non-gate jobs (mods, heal) drive failure precedence.
        	if job.Status == store.JobStatusFailed || job.Status == store.JobStatusCanceled {
        		hasNonGateFailure = true
        	}
        }

        if terminalJobs < int64(len(jobs)) {
        	// still in progress
        	return nil
        }

        var runStatus store.RunStatus
        switch {
        case hasNonGateFailure:
        	runStatus = store.RunStatusFailed
        case lastGateStatus == store.JobStatusFailed:
        	runStatus = store.RunStatusFailed
        case hasCanceled:
        	runStatus = store.RunStatusCanceled
        default:
        	runStatus = store.RunStatusSucceeded
        }
        ```
      - Make sure the loop that computes `hasCanceled` uses job statuses consistent with the final gate-based logic.
    - Update tests:
      - In `internal/server/handlers/server_runs_complete_test.go`, add tests that model:
        - Gate fails first, healing + re-gate later succeed, all mods succeed → overall run `succeeded`.
        - Mod job fails even if gates pass → overall run `failed`.
      - Wire the mock store in `test_mock_store_test.go` so `ListJobsByRun` returns a job set with:
        - gate jobs (`mod_type` in `Meta`) and mod jobs at different `StepIndex` values.
  - Test:
    - Unit:
      - `go test ./internal/server/handlers/...` — ensure new run-status derivation tests pass.
    - Integration:
      - Run an end-to-end healing scenario (existing integration tests that exercise Build Gate + healing) and verify the run-level status reflects the final gate outcome, not the initial failure.

## 3. Make job completion and node /complete use `job_id` instead of float `step_index`

- [x] Use `job_id` for job lookup and completion instead of `run_id + step_index` — avoid float equality issues and simplify handler logic
  - Component: control-plane handlers (`internal/server/handlers/nodes_complete.go`, `nodes_claim.go`), node agent status uploader (`internal/nodeagent/statusuploader.go`, `execution_upload.go`), types (`internal/domain/types/ids.go`, `internal/domain/types/stepindex`), OpenAPI docs.
  - Scope:
    - Extend the `/v1/nodes/{id}/complete` request payload to carry `job_id`:
      - In `internal/server/handlers/nodes_complete.go`, update the request struct:
        ```go
        var req struct {
        	RunID     domaintypes.RunID     `json:"run_id"`
        	JobID     domaintypes.JobID     `json:"job_id"`      // NEW
        	Status    string                `json:"status"`
        	ExitCode  *int32                `json:"exit_code,omitempty"`
        	Stats     json.RawMessage       `json:"stats,omitempty"`
        	StepIndex domaintypes.StepIndex `json:"step_index"` // kept for logging/compat or remove later
        }
        ```
      - Validate `job_id` similarly to `run_id` (non-zero, valid UUID):
        ```go
        if req.JobID.IsZero() {
        	http.Error(w, "job_id is required", http.StatusBadRequest)
        	return
        }
        jobID := domaintypes.ToPGUUID(req.JobID.String())
        if !jobID.Valid {
        	http.Error(w, "invalid job_id: invalid uuid", http.StatusBadRequest)
        	return
        }
        ```
    - Change the job lookup to use `GetJob` by `job_id` instead of `GetJobByRunAndStepIndex`:
      - Remove the `GetJobByRunAndStepIndex` call:
        ```go
        job, err := st.GetJobByRunAndStepIndex(...)
        ```
      - Replace it with:
        ```go
        job, err := st.GetJob(r.Context(), jobID)
        if err != nil { ... }
        ```
      - Keep the check that the job belongs to the specified `run_id`:
        ```go
        if job.RunID != runID {
        	http.Error(w, "job does not belong to this run", http.StatusBadRequest)
        	return
        }
        ```
    - Drop the `ListJobsByRun` “hasJobAssignment” pre-check:
      - Remove the loop that scans all jobs just to verify node has some job (`hasJobAssignment`).
      - Rely on the single `GetJob` + `job.NodeID == nodeID` check to enforce ownership.
    - Keep `StepIndex` only for logging:
      - Use `job.StepIndex` in logs instead of `req.StepIndex` if you want canonical values.
    - Node agent: include `job_id` in status upload:
      - In `internal/nodeagent/statusuploader.go`, extend payload:
        - Amend `UploadStatus` signature to accept a `jobID types.JobID` parameter:
          ```go
          func (u *StatusUploader) UploadStatus(ctx context.Context, runID, status string, exitCode *int32, stats types.RunStats, stepIndex types.StepIndex, jobID types.JobID) error {
          ```
        - Add `job_id` to the JSON payload:
          ```go
          payload := map[string]interface{}{
          	"run_id":     runID,
          	"job_id":     jobID,
          	"status":     status,
          	"step_index": stepIndex,
          }
          ```
      - Update call sites in `internal/nodeagent/execution_upload.go` and `execution_orchestrator.go` to pass `req.JobID`:
        ```go
        if uploadErr := statusUploader.UploadStatus(statusCtx, runID, status, exitCode, stats, stepIndex, req.JobID); uploadErr != nil { ... }
        ```
    - Adjust tests:
      - `internal/nodeagent/statusuploader_test.go` — expect `job_id` in payload.
      - `internal/server/handlers/server_runs_complete_test.go` — include `job_id` in request bodies and stop depending on `step_index` alone.
      - `internal/server/handlers/test_mock_store_test.go` — `GetJobByRunAndStepIndex` is no longer used by `nodes_complete`; keep it only for rehydration or remove later if unused.
  - Test:
    - Node agent unit tests:
      - `go test ./internal/nodeagent/...` — ensure status uploader tests pass and payload includes `job_id`.
    - Server handler tests:
      - `go test ./internal/server/handlers/...` — verify `/complete` tests now rely on `job_id`.

## 4. Align `/v1/nodes/{id}/complete` docs and 409 semantics with new behavior

- [x] Update node completion endpoint docs to reflect job-level completion and the new payload (including `job_id` and `exit_code`), and clarify 409 semantics — make OpenAPI match server behavior
  - Component: OpenAPI docs (`docs/api/paths/nodes_id_complete.yaml`, `docs/api/OpenAPI.yaml`), control-plane handler (`nodes_complete.go`), any CLI/tooling that relies on schema.
  - Scope:
    - Update `docs/api/paths/nodes_id_complete.yaml`:
      - Change summary/description to emphasize job completion:
        - From “Mark run as completed (running → terminal)” to e.g. “Mark job as completed and drive run completion”.
      - Replace `reason` with `job_id`, `step_index`, and `exit_code`:
        ```yaml
        properties:
          run_id:
            type: string
            format: uuid
            description: The run ID that owns the job being completed
          job_id:
            type: string
            format: uuid
            description: The job ID to complete
          status:
            type: string
            description: Terminal status derived from job execution
            enum: [ succeeded, failed, canceled ]
          exit_code:
            type: integer
            format: int32
            nullable: true
            description: Exit code reported by the node for this job
          step_index:
            type: number
            format: double
            nullable: true
            description: Optional step index for logging and diagnostics
          stats:
            type: object
            description: JSON object with job-level statistics/metrics
        required: [ run_id, job_id, status ]
        ```
      - Clarify 409 meaning:
        - Change `'409'` description to something like:
          - `description: Conflict (job not in 'running' state, or already completed)`
    - In `docs/api/OpenAPI.yaml`, ensure the path reference for `/v1/nodes/{id}/complete` still points to the updated YAML.
    - Make sure `docs/api/verify_openapi_test.go` has no assumptions about the old shape; update snapshots if needed.
  - Test:
    - OpenAPI validation:
      - `go test ./docs/api/...` or the existing `docs/api/verify_openapi_test.go` — confirm the OpenAPI file parses and passes structural tests.

## 5. Align `NodeClaimResponse` schema with job-centric claim response

- [x] Update `NodeClaimResponse` to expose `job_id`, `job_name`, `job_meta`, and the non-null float `step_index` — keep schema aligned with `claimJobHandler` response
  - Component: OpenAPI components (`docs/api/components/schemas/controlplane.yaml`), server handler (`internal/server/handlers/nodes_claim.go`), CLI/nodeagent consumers.
  - Scope:
    - Edit `NodeClaimResponse` in `docs/api/components/schemas/controlplane.yaml`:
      - Expand schema:
        ```yaml
        NodeClaimResponse:
          type: object
          properties:
            id:
              type: string
              format: uuid
              description: Run ID
            job_id:
              type: string
              format: uuid
              description: Claimed job ID
            job_name:
              type: string
              description: Logical job name (e.g., "pre-gate", "mod-0", "post-gate")
            job_meta:
              type: object
              additionalProperties: true
              description: Job metadata (mod_type, mod_image, etc.)
            repo_url: { type: string }
            status:   { type: string }
            node_id:  { type: string, format: uuid }
            base_ref: { type: string }
            target_ref: { type: string }
            commit_sha: { type: string, format: string, nullable: true }
            step_index:
              type: number
              format: double
              description: Float index used for job ordering and rehydration
            started_at: { type: string, format: date-time }
            created_at: { type: string, format: date-time }
            spec:
              type: object
              additionalProperties: true
              description: Opaque run spec with job_id and any server defaults merged in
          required:
            - id
            - job_id
            - job_name
            - repo_url
            - status
            - node_id
            - base_ref
            - target_ref
            - step_index
            - started_at
            - created_at
        ```
    - Confirm `internal/server/handlers/nodes_claim.go` response struct fields:
      - `ID`, `JobID`, `JobName`, `JobMeta`, `StepIndex`, `RepoURL`, `Status`, `NodeID`, `BaseRef`, `TargetRef`, `CommitSha`, `StartedAt`, `CreatedAt`, `Spec` — already match the schema.
    - If there is any dedicated client expecting the previous `NodeClaimResponse` shape (e.g., CLI), update it to use `job_id` now.
  - Test:
    - Node claim tests:
      - `go test ./internal/server/handlers/...` — `TestClaimJob_Success` and related tests should still pass.

## 6. Update Mods lifecycle docs to the job-centric model

- [x] Rewrite `docs/mods-lifecycle.md` sections that reference `stages` / `run_steps` to describe the `jobs` table, `step_index` floats, and server-driven scheduling — keep lifecycle doc authoritative for the new model
  - Component: `docs/mods-lifecycle.md`, mods API types (`internal/mods/api/types.go`), job creation handler (`handlers_mods_ticket.go`), diff and artifact handlers.
  - Scope:
    - In `docs/mods-lifecycle.md`:
      - Replace references to:
        - `stages`, `run_steps`, `run_steps.step_index`, `stages.meta.step_index`.
      - With references to:
        - `jobs` table (`jobs.status`, `jobs.step_index`, `jobs.meta.mod_type`, `jobs.meta.mod_image`).
      - Update the “Jobs and diffs” subsection to describe:
        - Jobs created at run submission:
          - `pre-gate` (step_index=1000, `status=scheduled`).
          - `mod-k` (step_index=2000+k*1000, `status=created`).
          - `post-gate` (step_index after last mod, `status=created`).
        - Healing jobs inserted between existing jobs using float `step_index` midpoints (align with `maybeCreateHealingJobs`).
        - Server-driven scheduling:
          - Only `status=scheduled` jobs are claimable (`ClaimJob` in `internal/store/queries/jobs.sql`).
          - After a job succeeds, `ScheduleNextJob` transitions the first `created` job to `scheduled`.
      - Document that:
        - Diffs are job-scoped (`diffs.job_id`) and rehydration uses `jobs.step_index`.
        - Node claims jobs, not runs; `job_id` is used to correlate logs, diffs, and artifacts.
    - In `internal/mods/api/types.go`:
      - Update comments for `TicketSummary.Stages` and `StageStatus.StepIndex` to explain that “stage” is now logically a `job` row and `StepIndex` mirrors `jobs.step_index`.
      - Example comment adjustment:
        ```go
        // Stages is keyed by job UUID (jobs.id). StepIndex mirrors jobs.step_index
        // and is used to order jobs in multi-step Mods runs.
        ```
    - Make sure the references at the end of `docs/mods-lifecycle.md` under “Database” point to:
      - `SCHEMA.sql` for jobs.
      - `internal/store/queries/jobs.sql` for `ClaimJob` and `ScheduleNextJob`.
  - Test:
    - Docs consistency:
      - Manually verify the described flow matches:
        - `handlers_mods_ticket.go` (`createJobsFromSpec`, `createSingleModJob`).
        - `internal/server/handlers/nodes_claim.go`.
        - `internal/nodeagent/claimer_loop.go` and `StartRunRequest`.

## 7. Harmonize `Run` / `TicketStatus` schemas with new fields (`reason` → `stats`/`exit_code`)

- [x] Remove or redefine `reason` from run/ticket schemas and rely on `stats.exit_code` and metadata for failure context — prevent schema drift from DB model
  - Component: `docs/api/components/schemas/controlplane.yaml`, any CLI that renders reasons, mods API (`internal/mods/api/types.go`) if it carries reason.
  - Scope:
    - In `controlplane.yaml`:
      - For `Run` and `TicketStatus`, remove the `reason` field from `properties` unless you deliberately expose a derived field.
      - Optionally replace with a metadata field that carries derived reason:
        ```yaml
        metadata:
          description: Additional ticket metadata (e.g., mr_url, human-readable reason)
          type: object
          additionalProperties:
            type: string
        ```
      - Ensure `stats` is present and documented as JSON object with job/run metrics, including `exit_code` where available.
    - If any GET `/v1/mods/{id}` handler sets a `reason` string in its response (search in `internal/server/handlers/handlers_mods_ticket.go`), either:
      - Stop populating it and move to metadata, or
      - Update docs to state that `reason` is derived from `runs.stats` and not stored in `runs` table directly.
  - Test:
    - `go test ./internal/server/handlers/...` to ensure response structs still match the updated schema.

## 8. Consider job-level completion surface `/v1/jobs/{job_id}`

- [ ] Evaluate and, if approved, introduce a job-level completion endpoint `/v1/jobs/{job_id}` — simplify node → server contract by addressing jobs directly
  - Component: server router (`internal/server/handlers/register.go`), new handler (e.g., `internal/server/handlers/jobs_complete.go`), store interface (`internal/store/querier.go`), nodeagent `StatusUploader`, OpenAPI docs.
  - Scope:
    - Design:
      - New endpoint: `POST /v1/jobs/{job_id}/complete` (or similar).
      - Request body:
        ```json
        {
          "status": "succeeded" | "failed" | "canceled",
          "exit_code": 0,
          "stats": { ... }
        }
        ```
      - Authentication/authorization still relies on node identity (mTLS) and must enforce that:
        - The job is assigned to the calling node.
      - Handler responsibilities:
        - Look up job by `job_id`.
        - Derive `run_id` from `job.RunID`.
        - Verify node assignment via `job.NodeID`.
        - Update job completion (`UpdateJobCompletion`).
        - Call `maybeCompleteMultiStepRun` to drive run completion.
      - Node agent changes:
        - `StatusUploader` would call `/v1/jobs/{job_id}/complete` instead of `/v1/nodes/{id}/complete`.
        - Payload no longer needs `run_id` / `step_index` in the body; those are implied or used only for stats.
    - Implementation sketch for handler:
      - New file: `internal/server/handlers/jobs_complete.go`:
        ```go
        func completeJobHandler(st store.Store, eventsService *events.Service) http.HandlerFunc {
        	return func(w http.ResponseWriter, r *http.Request) {
        		jobIDStr := r.PathValue("job_id")
        		// parse jobID, load job, load run, verify node via cert/identity,
        		// validate status, stats, exit_code, then call UpdateJobCompletion
        		// and maybeCompleteMultiStepRun.
        	}
        }
        ```
      - Register in `internal/server/handlers/register.go` under the worker role.
    - Docs:
      - Add `jobs_job_id_complete.yaml` under `docs/api/paths`.
      - Reference from `docs/api/OpenAPI.yaml`.
    - This step is **optional** and should be gated on design approval, since it changes the public API surface.
  - Test:
    - New handler unit tests:
      - `internal/server/handlers/jobs_complete_test.go` with scenarios mirroring current `server_runs_complete_test.go`.
    - Node agent end-to-end tests:
      - Adjust `execution_upload_test.go` / integration tests to call the new endpoint and verify behavior.

## 9. TDD + validation discipline

- [ ] Maintain RED→GREEN→REFACTOR discipline and verify coverage for all above changes — ensure changes remain well-tested
  - Component: repo-wide tests, coverage tooling, binary size guardrails.
  - Scope:
    - For each slice of changes above:
      - Start with a failing test (RED) that captures the desired new behavior (status derivation, job_id usage, docs shape where tested by Go).
      - Implement minimal code changes to make tests pass (GREEN).
      - Refactor shared helpers (`maybeCompleteMultiStepRun`, status conversion) once tests are stable (REFACTOR).
    - Use existing validation scripts:
      - Run `./scripts/validate-tdd-discipline.sh` to enforce:
        - `go test -cover ./...`
        - coverage thresholds (≥60% overall, ≥90% on critical workflow runner packages).
        - binary size guardrails.
  - Test:
    - `./scripts/validate-tdd-discipline.sh` — expect all phases to pass with updated code and docs.

