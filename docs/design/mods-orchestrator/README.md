# Mods Orchestrator Control-Plane Service (Roadmap 1.2)

## Status
- Stage: Implemented
- Target Roadmap Item: `1.2 Build a Mods orchestrator in the control plane`

## Context
The current control plane persists job-level metadata (`mods/<ticket>/jobs/<job-id>`) and queue state, but lacks a cohesive service for Mod lifecycle orchestration. Roadmap item 1.2 introduces an orchestrator responsible for:
- Persisting Mod tickets and stage graphs under `mods/<ticket>/**`.
- Translating Mod submissions into scheduler-ready job records and queue entries.
- Coordinating optimistic concurrency so a stage is claimed exactly once.
- Surfacing Mod-level metadata to the HTTP API (`/v1/mods/...`) for the CLI.

This proposal builds on the etcd schema defined in `docs/next/etcd.md` and the API contract in `docs/next/api.md`, aligning with the watcher and retention changes from item 1.1.

## Goals
- Create an `internal/controlplane/mods` package that owns Mod lifecycle orchestration, providing a clean interface for submission, status reads, stage transitions, and cancellation.
- Model Mod tickets in etcd using a normalized structure that captures status, stage graph, artifact references, and optimistic concurrency counters.
- Implement a transactional stage claim path that prevents double execution while allowing retries.
- Integrate with the scheduler to enqueue stage-level jobs and watch for completions, updating Mod state accordingly.
- Provide service hooks for the HTTP handlers in `internal/api/httpserver/controlplane.go` that will be expanded in roadmap item 1.3.
- Document testing strategy (unit, integration readiness) and observability requirements so the implementation can follow RED→GREEN→REFACTOR.

## Non-Goals
- Implementing the HTTP routes themselves (covered by roadmap item 1.3).
- Wiring SHIFT/runtime execution (roadmap section 2).
- Building artifact/registry backends (item 1.4), beyond referencing integration points.
- Finalizing CLI UX or documentation updates beyond orchestrator-specific notes.

## Proposed Architecture

### Package Layout
- `internal/controlplane/mods/`
  - `service.go`: Public interface (`Submit`, `Resume`, `Cancel`, `StageStatus`, `TicketStatus`).
  - `store.go`: etcd CRUD helpers and transactional primitives.
  - `graph.go`: Stage graph validation, dependency resolution, and optimistic concurrency helpers.
  - `scheduler_bridge.go`: Adapter that enqueues jobs via `internal/controlplane/scheduler`.
  - `watchers.go`: Watches for job completion (`mods/<ticket>/jobs/**`) and lease expiry to drive state machines.
  - `errors.go`: Sentinels for concurrency conflicts (`ErrStageAlreadyClaimed`) to back off and retry.

### Alignment with Existing Packages
- `internal/workflow/mods`: Hosts the current planner, stage metadata, and advisor interfaces consumed by the legacy workflow runner. To maintain clean architecture, extract the pure domain types and planner construction into a neutral module (e.g., `internal/mods/plan`). Keep `internal/workflow/mods` as a thin shim re-exporting the planner until roadmap item 3.5 retires the runner. This ensures the control-plane orchestrator and CLI share a single source of truth for stage names, metadata, and advisor integration.
- `internal/cli/mods`: Currently implements log streaming against SSE endpoints. As `/v1/mods` endpoints land, move shared response structs (ticket summaries, stage DTOs, retention hints) into a CLI-friendly subpackage under `internal/mods/api` so HTTP handlers and CLI commands consume identical types without forming a circular dependency on control-plane packages.
- `internal/workflow/runner`: Remains in place during transition. Provide an adapter that lets the runner optionally submit tickets through the orchestrator (feature flag) while gradually deprecating direct Grid integrations. This avoids duplicate scheduling logic and gives a path to validate the orchestrator before fully removing `runner`.

### etcd Data Model Extensions
- `mods/<ticket>/meta`: Top-level ticket metadata (status, submitter, repository, created/updated timestamps).
- `mods/<ticket>/graph`: Serialized stage graph (list of nodes, edges, retry policy, concurrency caps).
- `mods/<ticket>/stages/<stage-id>`: Stage execution record with fields:
  - `state`: `pending|queued|running|succeeded|failed|cancelling|cancelled`.
  - `attempts`: Attempt counter.
  - `current_job_id`: etcd key to active job (`mods/<ticket>/jobs/<job-id>`).
  - `artifacts`: Diff/asset CID references, log bundle keys.
  - `version`: Monotonic revision used for optimistic concurrency (etcd `mod_revision` mirrored in payload).
- `mods/<ticket>/events/<uuid>` (optional extension for audit trail once SSE is wired).

### Submission Flow (`Submit`)
1. Validate stage graph and metadata.
2. Create transaction:
   - Put `mods/<ticket>/meta` with initial status `pending`.
   - Put `mods/<ticket>/graph`.
   - Put each `mods/<ticket>/stages/<stage-id>` with `state=pending`, `attempts=0`.
   - Enqueue root stages as jobs via scheduler transaction helper that writes to `queue/mods` and `mods/<ticket>/jobs/<job-id>`.
3. Emit scheduler enqueue events (existing watchers pick up queue updates).

### Stage Claim Flow
- Workers claim jobs through scheduler (item 1.1). Before a job is marked `running`, `store.go` will:
  1. Read stage record.
  2. Submit an etcd transaction conditioned on `version` matching the stored value.
  3. Update `state=running`, bump `attempts`, set `current_job_id`.
  4. Attach lease metadata for audit.
- If transaction fails (stage already claimed), scheduler retries or requeues job.

### Completion & Retry Flow
- Watcher consumes job completion events:
  - On success: mark stage `succeeded`, clear `current_job_id`, update artifacts, enqueue dependent stages.
  - On failure: increment attempts, evaluate retry policy, either re-enqueue or fail the Mod.
- The orchestrator updates `mods/<ticket>/meta.status` (`running`, `failed`, `succeeded`, `cancelled`) via conditional transactions to guard against concurrent cancels/resumes.

### Cancellation & Resume
- `Cancel(ticket)`:
  - Mark ticket meta as `cancelling`.
  - Iterate stages to set terminal states or flag active stages for cancellation (future worker support via `/v1/jobs/{id}/cancel` once available).
- `Resume(ticket)`:
  - Rehydrate graph, identify failed/cancelled stages with retry budget remaining, enqueue them following dependency rules.

### Integration Points
- Scheduler dependency: reuse transactional helpers introduced in item 1.1 for queue writes and job record creation so retention metadata remains consistent.
- HTTP API: expose orchestrator via dependency injection in `internal/api/httpserver/controlplane.go`.
- Logging: use structured logging (`internal/log`) to record stage transitions and transaction conflicts.
- Metrics: add counters/histograms (submitted mods, stage retries, conflicts) once observability slice (4.2) is active; instrument upfront but keep registration minimal to avoid duplication.

## Testing Strategy (Plan Before Coding)
- **Unit Tests (RED/GREEN)**:
  - Stage graph validation (`graph_test.go`).
  - Concurrent stage claims using in-memory etcd (embed server or fake client).
  - Submission flow ensuring atomic creation of meta, graph, stages, and queue entries.
  - Completion handler transitions for success/failure paths.
- **Table-Driven Tests** verifying retry policy evaluation and dependency fan-out.
- **Fakes/Mocks**: Wrap etcd client interactions to simulate `Compare-And-Swap` failures.
- **Coverage Targets**: Maintain ≥90% coverage within `internal/controlplane/mods` package, contributing to ≥60% overall (per repo rules).
- **Future Integration Hooks**:
  - Document how to extend tests under `tests/e2e` once Grid harness is ready (REFACTOR phase).

## Observability & Operations
- Emit debug logs when transactions conflict to aid tuning.
- Record Mod lifecycle events under `mods/<ticket>/events` for future SSE streaming (item 4.1).
- Provide admin tooling via `ploy mods inspect` (future work) to fetch orchestrator state.
- Consider adding a `healthz` probe for orchestrator background loops (watchers) so control plane reports readiness.

## Rollout Strategy
1. Implement `internal/controlplane/mods` with feature flags to gate new flows.
2. Backfill existing Mod submissions (if any) by migrating data into the new schema.
3. Enable orchestrator for new tickets while CLI still uses legacy runner; ensure compatibility by keeping job schema stable.
4. Coordinate with roadmap item 1.3 to expose HTTP routes, followed by CLI updates (section 3).
5. Monitor etcd transaction metrics to confirm optimistic concurrency behaves under load.

## Risks & Mitigations
- **Concurrent stage claims**: Mitigated via etcd compare revisions; add exponential backoff on retries.
- **Schema drift**: Keep schema definitions centralized in `store.go` and mirrored in tests; document updates in `docs/next/job.md` if fields change.
- **Watcher lag**: Ensure watchers use resumable revision tokens and handle compaction by performing range refresh on `ErrCompacted`.
- **Partial submissions**: Transactions guarantee atomic creation; fallback cleanup job can sweep orphaned `mods/<ticket>` entries if transaction aborted mid-flight.
- **Domain duplication**: Without the planned module extraction, planner types could diverge between control plane, CLI, and legacy runner. Treat the shared module as the single source of truth and enforce imports via static checks.

## Implementation Checklist
- [x] Scaffold `internal/controlplane/mods` package interfaces and dependency injection.
- [x] Implement etcd transaction helpers for ticket, stage, and job orchestration.
- [x] Integrate scheduler bridge for enqueue/dequeue flows with optimistic concurrency.
- [x] Add background watchers for job completion and lease expiry.
- [x] Extract planner/stage domain types into a neutral module shared by control plane, CLI, and legacy runner.
- [ ] Add feature-flagged adapter so `internal/workflow/runner` can delegate submissions to the orchestrator during migration.
- [ ] Publish shared API DTOs for Mods ticket/stage responses to keep `internal/cli/mods` aligned with control-plane handlers.
- [x] Write unit tests covering submission, claim, completion, retry, cancel, and resume flows.
- [x] Document schema updates in `docs/next/job.md` or related docs if fields change.
- [x] When all above tasks pass tests, update `docs/next/roadmap.md` item 1.2 to `[x]` and commit the implementation.

## Completion Criteria
- Mod submissions persist deterministic state in etcd and drive scheduler job creation without double claims.
- Stage transitions are transactional, observable, and retry-aware.
- CLI/API layers can rely on orchestrator service to serve `GET /v1/mods/{ticket}` once handlers land.
- Unit tests reach the stated coverage thresholds.
- **Post-implementation reminder**: After satisfying the checklist, mark roadmap item 1.2 as complete (set the checkbox to `[x]`) and commit all resulting changes together.

## References
- `docs/next/roadmap.md`
- `docs/next/etcd.md`
- `docs/next/api.md`
