# Heartbeat-Based Stale Job Recovery

Scope: Implement end-to-end recovery for jobs stuck in `Running` when a node stops heartbeating. The server must detect stale nodes, cancel orphaned active jobs for affected repo attempts, recompute `run_repos` status, and finalize `runs` status when all repos are terminal.

Documentation: `AGENTS.md`; `docs/testing-workflow.md`; `docs/migs-lifecycle.md`; `internal/server/handlers/jobs_complete.go`; `internal/server/handlers/nodes_complete_run.go`; `internal/store/queries/jobs.sql`; `internal/store/queries/nodes.sql`; `cmd/ployd/server.go`

Legend: [ ] todo, [x] done.

## Phase 0: Behavior Contract and RED
- Note: Phase 0 tests are scaffolded and intentionally skipped until the Phase 2 recovery worker is wired.
- [x] Add failing tests that reproduce stale `Running` jobs after node loss — Locks target behavior before implementation.
  - Repository: `ploy`
  - Component: `internal/server/handlers`, `internal/store`, `internal/store/*scheduler*`
  - Scope: Add tests for: node heartbeat becomes stale, job remains `Running`, recovery cycle cancels active chain and unblocks terminal run/repo status transitions.
  - Snippets: `go test ./internal/server/handlers -run 'Stale|Recovery'`
  - Tests: `make test` — New stale-recovery tests fail first.

- [x] Add run/repo status reconciliation regression tests for cancellation path — Guarantees parity with existing completion semantics.
  - Repository: `ploy`
  - Component: `internal/server/handlers`
  - Scope: Cover these outcomes after stale cancellation: `run_repos` -> `Cancelled`, `runs` -> `Finished` when all repos terminal, and no transition when other repos are still non-terminal.
  - Snippets: `go test ./internal/server/handlers -run 'RepoStatus|RunCompletion|Cancelled'`
  - Tests: Expected failures on missing stale-recovery implementation.

## Phase 1: Store Primitives for Stale Detection and Cancellation
- [x] Add SQL query to list stale running jobs by node heartbeat age — Provides deterministic stale candidate selection.
  - Repository: `ploy`
  - Component: `internal/store/queries/jobs.sql`, generated sqlc files
  - Scope: Add query joining `jobs` + `nodes` filtering `jobs.status='Running'` and stale heartbeat (`last_heartbeat` older than cutoff or NULL), returning run/repo/attempt keyed rows.
  - Snippets: `ListStaleRunningJobs(cutoff timestamptz)`
  - Tests: `go test ./internal/store -run 'StaleRunningJobs'` — Correct rows only.

- [x] Add SQL query to bulk-cancel active jobs in a repo attempt — Ensures orphaned chains become terminal.
  - Repository: `ploy`
  - Component: `internal/store/queries/jobs.sql`, generated sqlc files
  - Scope: Add query updating `Created|Queued|Running -> Cancelled` for `(run_id, repo_id, attempt)` with proper `finished_at`/`duration_ms` handling.
  - Snippets: `CancelActiveJobsByRunRepoAttempt(run_id, repo_id, attempt)`
  - Tests: `go test ./internal/store -run 'CancelActiveJobsByRunRepoAttempt'` — Only active rows updated.

## Phase 2: Server Recovery Worker
- [x] Implement a scheduler task that performs stale-job recovery cycles — Moves recovery out of request path and makes it self-healing.
  - Repository: `ploy`
  - Component: `internal/server/handlers` (or dedicated recovery package), `internal/server/scheduler`
  - Scope: Add task with cycle: list stale running jobs -> group by repo attempt -> cancel active jobs in attempt -> recompute `run_repos` and `runs` terminal state via existing status derivation logic.
  - Snippets: `type StaleJobRecoveryTask struct { ... }`
  - Tests: `go test ./internal/server/... -run 'StaleJobRecoveryTask'` — One cycle resolves stuck attempts.

- [x] Reuse one canonical repo/run terminal reconciliation path — Prevents divergence from job-complete behavior.
  - Repository: `ploy`
  - Component: `internal/server/handlers/nodes_complete_run.go`, recovery task code
  - Scope: Extract/reuse shared helpers for `run_repos` derivation and `runs` completion checks; avoid duplicate logic forks.
  - Snippets: `maybeUpdateRunRepoStatus(...)`, `maybeCompleteRunIfAllReposTerminal(...)`
  - Tests: Existing completion tests + recovery tests remain green with identical status semantics.

## Phase 3: Config and Wiring
- [x] Add explicit recovery configuration with safe defaults — Prevents false positives from heartbeat jitter.
  - Repository: `ploy`
  - Component: `internal/server/config/types.go`, `internal/server/config/defaults.go`, `internal/server/config/config_test.go`
  - Scope: Add scheduler config keys for stale threshold and recovery interval; default threshold must exceed normal node heartbeat cadence used in local/dev.
  - Snippets: `scheduler.stale_job_recovery_interval`, `scheduler.node_stale_after`
  - Tests: `go test ./internal/server/config` — Defaults and custom values validated.

- [x] Register and start the recovery task in server bootstrap — Activates feature in production path.
  - Repository: `ploy`
  - Component: `cmd/ployd/server.go`
  - Scope: Build task from config and store, add to scheduler, ensure graceful shutdown path remains unchanged.
  - Snippets: `sched.AddTask(staleRecoveryTask)`
  - Tests: `go test ./cmd/ployd ./internal/server/scheduler` — Boot wiring and lifecycle pass.

## Phase 4: Observability and API Surface Consistency
- [x] Add structured logs and counters for stale recovery actions — Makes operator diagnosis clear.
  - Repository: `ploy`
  - Component: recovery task package
  - Scope: Log stale node count, affected attempts, canceled jobs, and finalized runs; include run/repo IDs in structured fields.
  - Snippets: `slog.Info("stale-job-recovery cycle", ...)`
  - Tests: `go test ./internal/server/...` — verify behavior; log assertions where already used.

- [x] Ensure SSE/run status consistency after recovery finalization — Keeps CLI follow/status behavior coherent.
  - Repository: `ploy`
  - Component: `internal/server/events_service.go`, reconciliation helper usage
  - Scope: Publish run terminal snapshot/done status when recovery causes finalization, matching existing completion path semantics.
  - Snippets: `eventsService.PublishRun(...)`, `Hub().PublishStatus(... done ...)`
  - Tests: `go test ./internal/server/handlers -run 'Events|RunCompletion'` — terminal events emitted once.

## Phase 5: GREEN and Documentation
- [x] Run full verification for touched packages — Confirms no regressions in queueing and completion.
  - Repository: `ploy`
  - Component: `internal/store/**`, `internal/server/**`, `cmd/ployd`
  - Scope: Execute unit tests, vet, staticcheck, and coverage checks for affected paths.
  - Snippets: `make test`; `make vet`; `make staticcheck`; `make coverage`
  - Tests: All pass with project thresholds maintained.

- [x] Update docs to reflect recovery behavior and new config knobs — Keeps runtime docs aligned with implementation.
  - Repository: `ploy`
  - Component: `docs/migs-lifecycle.md`, `docs/envs/README.md`, optionally `docs/testing-workflow.md`
  - Scope: Document stale-node detection semantics, recovery lifecycle, config defaults, and troubleshooting commands.
  - Snippets: `GET /v1/runs/{id}/status` examples showing recovery-terminalized repos.
  - Tests: `npx --yes markdownlint --config .markdownlint.yaml docs/**/*.md` — docs lint clean.
