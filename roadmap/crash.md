# Crash Startup Reconciliation (Interim 120s Policy)

Scope: Implement `design/crash.md` for node startup after node process crash/restart. Before normal claim polling, reconcile existing ploy job containers: restore tracking for running containers, and complete recently-finished terminal containers (`finished_at >= now-120s`) through the canonical `POST /v1/jobs/{job_id}/complete` path.

Documentation: `design/crash.md`; `internal/nodeagent/claimer_loop.go`; `internal/nodeagent/claimer.go`; `internal/nodeagent/agent.go`; `internal/nodeagent/http.go`; `internal/nodeagent/logstreamer.go`; `internal/nodeagent/claim_cleanup.go`; `internal/workflow/step/container_spec.go`; `internal/workflow/step/runner.go`; `internal/workflow/step/gate_docker.go`; `internal/domain/types/ids.go`; `internal/server/handlers/jobs_complete.go`; `docs/migs-lifecycle.md`; `docs/envs/README.md`; `cmd/ployd-node/README.md`

Legend: [ ] todo, [x] done.

## Phase 0: Behavior Contract Tests
- [x] Add startup reconciliation tests before implementation to lock target behavior and ordering. — Prevent regressions in crash semantics and startup sequence.
  - Repository: `ploy`
  - Component: `internal/nodeagent`
  - Scope: Add a dedicated suite (new file, for example `internal/nodeagent/crash_reconcile_test.go`) and extend `internal/nodeagent/claimer_loop_test.go` to verify:
    - startup reconciliation runs before the first normal claim attempt;
    - container classification uses runtime state (`running` vs terminal `exited`/`dead`);
    - terminal reconciliation uses `finished_at` cutoff (`>= now-120s`) and not container creation time;
    - stale terminal containers (older than 120s) are skipped.
  - Snippets:
    - claim loop anchor: `func (c *ClaimManager) Start(ctx context.Context) error`
    - current claim path: `claimed, err := c.claimAndExecute(ctx)`
  - Tests: `go test ./internal/nodeagent -run 'Crash|Reconcile|ClaimLoop'` — Reconciliation should execute before claim polling and classify containers deterministically.

## Phase 1: Container Identity Prerequisite
- [x] Ensure all job containers carry both `run_id` and `job_id` labels for startup discovery. — Startup reconciliation must map containers to exact jobs without server-side guesswork.
  - Repository: `ploy`
  - Component: `internal/workflow/step`, `internal/nodeagent`
  - Scope:
    - Extend step container label construction so non-gate job containers include `com.ploy.job_id` in addition to `com.ploy.run_id`.
    - Keep gate path aligned with the same label contract (gate already injects labels via context).
    - Do not add compatibility fallbacks for unlabeled legacy containers.
  - Snippets:
    - current non-gate label path: `labels = map[string]string{types.LabelRunID: runID.String()}` in `internal/workflow/step/container_spec.go`
    - label constants: `types.LabelRunID`, `types.LabelJobID` in `internal/domain/types/ids.go`
    - gate label injection: `withGateExecutionLabels(...)` in `internal/nodeagent/gate_context.go`
  - Tests:
    - update/add `internal/workflow/step/labels_test.go`
    - update/add `internal/workflow/step/gate_docker_test.go`
    - `go test ./internal/workflow/step -run 'Label|Gate'` — Every ploy-managed container should expose both labels when job identity exists.

## Phase 2: Startup Discovery and Classification
- [x] Add a startup crash reconciler service in nodeagent. — Centralize crash policy and keep claim loop focused on polling/backoff.
  - Repository: `ploy`
  - Component: `internal/nodeagent`
  - Scope:
    - Add a dedicated reconciler (new file, for example `internal/nodeagent/crash_reconcile.go`) with Docker discovery + classification logic.
    - Discover candidate containers using ploy labels, then inspect each container for authoritative runtime state and timestamps.
    - Partition into:
      - running: restore tracking;
      - terminal (`exited`/`dead`): reconcile only if `finished_at >= now-120s`.
    - Use terminal timestamp (`finished_at`) only; never use `Created` for the 120s policy.
  - Snippets:
    - Docker discovery pattern from existing cleanup: `listed, err := c.docker.ContainerList(ctx, client.ContainerListOptions{All: true})` in `internal/nodeagent/claim_cleanup.go`
    - daemon metadata pattern: `info, err := c.docker.Info(ctx, client.InfoOptions{})`
    - wait/inspect state shape reference: `ContainerWait(...)` + `ContainerInspect(...)` in `internal/workflow/step/container_docker.go`
  - Tests: `go test ./internal/nodeagent -run 'CrashReconcile|Startup'` — Reconciler should produce stable running/terminal partitions and enforce the 120s window by `finished_at`.

## Phase 3: Restore Running Container Tracking
- [x] Reattach running containers at startup and continue wait/log/status flow. — Running work should complete through the same node->server reporting contracts.
  - Repository: `ploy`
  - Component: `internal/nodeagent`
  - Scope:
    - For each recovered running container, start a monitor goroutine that:
      - waits for container termination;
      - uploads logs under `(run_id, job_id)`;
      - uploads terminal job status through job completion API.
    - Reserve/release node concurrency slots around recovered monitors so claim concurrency remains correct.
    - Keep runtime errors isolated per recovered container; do not crash the whole agent loop on one monitor failure.
  - Snippets:
    - concurrency contract: `AcquireSlot(ctx)` / `ReleaseSlot()` in `internal/nodeagent/controller.go`
    - log uploader path: `NewLogStreamer(cfg, runID, jobID)` in `internal/nodeagent/logstreamer.go`
    - status upload path: `UploadJobStatus(ctx, jobID, status, exitCode, stats)` in `internal/nodeagent/http.go`
  - Tests:
    - new monitor-focused tests in `internal/nodeagent/crash_reconcile_test.go`
    - extend `internal/nodeagent/agent_claim_test.go` for startup + claim concurrency interaction
    - `go test ./internal/nodeagent -run 'Recovered|Startup|Concurrency'` — Recovered running jobs should finish and report without blocking the node indefinitely.

## Phase 4: Terminal Reconciliation and Idempotent Completion
- [x] Reconcile recent terminal containers through `/v1/jobs/{job_id}/complete` and treat already-terminal conflicts as non-fatal. — Startup replay must be safe under races with server stale-recovery.
  - Repository: `ploy`
  - Component: `internal/nodeagent` (client behavior), `internal/server/handlers` (contract tests)
  - Scope:
    - Add reconciliation-specific completion upload behavior that accepts terminal conflict responses as non-fatal (idempotent replay semantics).
    - Keep canonical completion endpoint unchanged (`POST /v1/jobs/{job_id}/complete`).
    - Preserve strict ownership validation and non-terminal error handling.
  - Snippets:
    - canonical endpoint: `fmt.Sprintf("/v1/jobs/%s/complete", jobID)` in `internal/nodeagent/http.go`
    - current server conflict branch: `if job.Status != store.JobStatusRunning { ... http.StatusConflict ... }` in `internal/server/handlers/jobs_complete.go`
    - current retry policy behavior: `postJSONWithRetry` (4xx permanent) in `internal/nodeagent/http.go`
  - Tests:
    - extend `internal/nodeagent/statusuploader_test.go` for 409-as-non-fatal reconciliation path
    - add handler regression assertions in `internal/server/handlers/jobs_complete_test.go`
    - `go test ./internal/nodeagent ./internal/server/handlers -run 'Complete|Idempotent|Conflict'` — Duplicate/replayed completion should not fail startup reconciliation.

## Phase 5: Startup Wiring Before Claim Loop
- [x] Wire reconciliation into node startup before normal claim polling. — Enforce design ordering exactly.
  - Repository: `ploy`
  - Component: `internal/nodeagent/claimer_loop.go`, `internal/nodeagent/claimer.go`, `internal/nodeagent/agent.go`
  - Scope:
    - Execute one startup reconciliation pass before entering ticker-based claim polling.
    - Keep existing claim backoff mechanics unchanged after startup pass.
    - Ensure startup reconciliation cannot run multiple times in the same process lifetime.
  - Snippets:
    - claim loop skeleton in `Start(...)`:
      - `ticker := time.NewTicker(...)`
      - `claimed, err := c.claimAndExecute(ctx)`
    - claim path remains: `POST /v1/nodes/{id}/claim`
  - Tests:
    - extend `internal/nodeagent/claimer_loop_test.go` and `internal/nodeagent/agent_test.go`
    - `go test ./internal/nodeagent -run 'Startup|ClaimLoop|Ordering'` — First claim must happen only after startup reconciliation completes.

## Phase 6: Docs and Contract Sync
- [ ] Update docs to reflect startup crash reconciliation policy and 120s terminal window. — Keep runtime docs aligned with implemented behavior.
  - Repository: `ploy`
  - Component: docs
  - Scope:
    - Add startup crash reconciliation behavior to `docs/migs-lifecycle.md` (node startup and stale-recovery interplay).
    - Update `docs/envs/README.md` with operational notes (fixed 120s window, no config knob).
    - Update `cmd/ployd-node/README.md` startup flow to mention pre-claim reconciliation pass.
  - Snippets:
    - policy text anchor: `finished_at >= now-120s` from `design/crash.md`
    - stale recovery defaults to cross-reference: `scheduler.stale_job_recovery_interval=30s`, `scheduler.node_stale_after=1m`
  - Tests:
    - `go test ./docs/api/...` (if affected)
    - `go test ./cmd/ployd-node/...` (if README/help tests exist)
    - Docs should match code behavior and endpoint contracts.

## Phase 7: Verification
- [ ] Run targeted suites first, then full hygiene. — Validate crash semantics, upload contracts, and startup ordering end-to-end.
  - Repository: `ploy`
  - Component: nodeagent, step runtime, server handlers, docs/api
  - Scope:
    - Run focused tests for reconciliation and completion idempotency.
    - Then run full unit + hygiene targets.
  - Snippets:
    - `go test ./internal/nodeagent ./internal/workflow/step ./internal/server/handlers`
    - `make test`
    - `make vet`
    - `make staticcheck`
  - Tests: All updated suites should pass; no change to public completion endpoint shape/status.

## Open Questions
- None.
