# Run Events Stream And Job Logs Stream

## Summary
Split streaming responsibilities into two explicit channels:
- Run lifecycle SSE (`/v1/runs/{run_id}/logs`) carries run/repo/stage progress events only.
- Job log SSE (`/v1/jobs/{job_id}/logs`) carries container log lines for one job only.

Expected outcome:
- `ploy run --follow` and `ploy run logs` remain available for run-level progress visibility.
- Job log fanout no longer broadcasts all log lines to every run subscriber.
- `ploy job --follow <job-id>` can stream the exact container logs for one job without client-side drop filtering.

## Scope
In scope:
- API contract changes for run/job SSE endpoints and job-level log ingestion endpoint.
- Server event fanout split by stream domain (run-state vs job-logs).
- CLI behavior alignment for `run --follow`, `run logs`, and new `job --follow`.
- Tests and docs updates for the new contracts.

Out of scope:
- Reworking workflow orchestration (`claim`, `complete`, `next_id`, repo/run status transitions).
- Replacing blob/log persistence model.
- Cross-cluster/event-bus redesign.

## Why This Is Needed
Current fanout is run-keyed and includes container logs:
- Node logs are ingested via [internal/server/handlers/nodes_logs.go](../internal/server/handlers/nodes_logs.go).
- Logs are fanned out through a run-keyed hub in [internal/server/events_service.go](../internal/server/events_service.go) and [internal/stream/hub.go](../internal/stream/hub.go).
- SSE consumers subscribe via [internal/server/handlers/events.go](../internal/server/handlers/events.go) (`GET /v1/runs/{id}/logs`).

With multiple nodes and repos in parallel, run streams become high-volume mixed log feeds and force downstream filtering pressure on transport and clients.

## Goals
- Keep run progress streaming lightweight and stable under parallel execution.
- Provide first-class job-scoped log streaming without run-wide log merge.
- Preserve current run-follow UX and run lifecycle visibility.
- Keep implementation bounded and incremental.

## Non-goals
- No backward compatibility requirement for removed undocumented stream payload behavior.
- No new persistence backend.
- No broad redefinition of event schemas beyond stream ownership split.

## Current Baseline (Observed)
- Run SSE endpoint (`GET /v1/runs/{id}/logs`) currently streams `run`, `log`, `retention`, `done` ([docs/api/paths/runs_id_logs.yaml](../docs/api/paths/runs_id_logs.yaml)).
- Repo SSE endpoint (`GET /v1/runs/{run_id}/repos/{repo_id}/logs`) filters run stream events by allowed job IDs ([internal/server/handlers/events.go](../internal/server/handlers/events.go)).
- Node log ingestion path is node-scoped and run/job-attributed (`POST /v1/nodes/{id}/logs`) ([internal/server/handlers/nodes_logs.go](../internal/server/handlers/nodes_logs.go)).
- Run follow in CLI is hybrid:
  - SSE acts as refresh signal.
  - Polling (`GetRunReportCommand`) is authoritative for rendered state ([internal/cli/runs/follow_tui.go](../internal/cli/runs/follow_tui.go), [internal/cli/runs/report_builder.go](../internal/cli/runs/report_builder.go)).
- Store already supports job-specific log reads (`ListLogsByRunAndJob`) ([internal/store/queries/logs.sql](../internal/store/queries/logs.sql)).

## Target Contract or Target Architecture
### 1. Stream ownership
- `GET /v1/runs/{run_id}/logs`:
  - Allowed events: `run`, `stage` (if present), `done`.
  - Disallowed events: `log`, `retention`.
  - Purpose: run lifecycle and orchestration visibility only.
- `GET /v1/jobs/{job_id}/logs`:
  - Allowed events: `log`, `retention`, `done`.
  - Disallowed events: `run`, `stage`.
  - Purpose: container/job log streaming for one job.

### 2. Ingestion ownership
- Introduce `POST /v1/jobs/{job_id}/logs` as canonical worker log upload endpoint.
- Request payload keeps chunk semantics (`chunk_no`, `data`) and size limits.
- Server resolves `run_id` from `job_id` and persists using existing logs storage model.
- Existing node-scoped ingestion may be retained temporarily as compatibility shim, but new implementation targets job-scoped ingestion as canonical.

### 3. Fanout architecture
- `EventsService` maintains separate streaming domains:
  - run-state publisher path for lifecycle snapshots/status completion.
  - job-log publisher path for per-job log lines and retention hints.
- Job completion publishes job-stream `done` for that `job_id`.
- Run completion publishes run-stream `done` for that `run_id`.

### 4. CLI contract
- `ploy run --follow`:
  - unchanged rendering behavior (polling + optional SSE trigger).
  - consumes run-state stream only.
- `ploy run logs <run-id>`:
  - remains supported as run-event stream consumer.
  - no longer expected to emit container log lines.
- `ploy job --follow <job-id>`:
  - streams only that job’s container logs from `/v1/jobs/{job_id}/logs`.
- `ploy run ...` text report/link behavior:
  - remove `Logs` reference from the `Artefacts` column.
  - render `<job-id>` in the `Job` column as an OSC8/browser-capable link to `/v1/jobs/{job_id}/logs`.
  - include `auth_token` query parameter when token-aware links are enabled, consistent with existing link behavior.

## Implementation Notes
- Server handlers:
  - Add job logs SSE handler and route registration in [internal/server/handlers/register.go](../internal/server/handlers/register.go).
  - Add job logs ingest handler (canonical path) alongside existing node ingest path.
  - Refactor [internal/server/handlers/events.go](../internal/server/handlers/events.go) to remove log emission from run SSE path.
- Events service/hub:
  - Extend [internal/server/events_service.go](../internal/server/events_service.go) to publish job-log events by `job_id`.
  - Keep run-state publication API isolated from job-log publication.
- Store/backfill:
  - Reuse existing log queries from [internal/store/queries/logs.sql](../internal/store/queries/logs.sql), especially `ListLogsByRunAndJob`, for job SSE backfill.
- CLI:
  - Keep `run follow` and `run logs` run-oriented.
  - Add job command surface in `cmd/ploy` and job streaming command module under `internal/cli`.
  - Update run report rendering to:
    - stop showing `Logs` in `Artefacts`.
    - emit job-id log links pointing to `/v1/jobs/{job_id}/logs` (tokenized when enabled).
- API docs:
  - Update [docs/api/OpenAPI.yaml](../docs/api/OpenAPI.yaml) and `docs/api/paths/*` to reflect event type separation and new job logs endpoints.

## Milestones
### Milestone 1: Contracts and endpoints
Scope:
- Define and register `GET /v1/jobs/{job_id}/logs` and `POST /v1/jobs/{job_id}/logs`.
- Document run stream as run-state only.
Expected Results:
- API surface clearly separates run events from job logs.
Testable outcome:
- Endpoint tests validate event type constraints and authorization.

### Milestone 2: Server fanout split
Scope:
- Refactor event publication so job logs do not enter run SSE stream.
- Emit `done` for job stream on job completion.
Expected Results:
- Run subscribers stop receiving container logs.
- Job subscribers receive only their job logs + terminal sentinel.
Testable outcome:
- Stream tests assert run stream excludes `log` and job stream excludes `run`.

### Milestone 3: CLI alignment
Scope:
- Keep `ploy run logs` for run events.
- Add `ploy job --follow <job-id>` for job logs.
Expected Results:
- Operator can inspect orchestration at run scope and container output at job scope.
Testable outcome:
- CLI tests cover `run logs` event-only behavior and `job --follow` output behavior.

## Acceptance Criteria
- `GET /v1/runs/{run_id}/logs` never emits job log frames.
- `GET /v1/jobs/{job_id}/logs` never emits run lifecycle frames.
- Job stream backfill + live updates are ordered and resumable via `Last-Event-ID`.
- `ploy run --follow` behavior remains functionally unchanged for progress tracking.
- `ploy job --follow <job-id>` provides targeted container logs without client-side run-stream filtering.
- `ploy run ...` output no longer shows `Logs` in `Artefacts`, and job IDs link to `/v1/jobs/{job_id}/logs` with auth token when link tokenization is enabled.

## Risks
- Incorrect done signaling can leave job streams open after job terminal state.
- Temporary dual-ingest period can duplicate publication unless dedup rules are explicit.
- Documentation drift if CLI and API event contracts are changed in separate commits.
- Load may shift to many job streams; connection limits and history sizes must be tuned deliberately.

## References
- [internal/server/handlers/events.go](../internal/server/handlers/events.go)
- [internal/server/handlers/nodes_logs.go](../internal/server/handlers/nodes_logs.go)
- [internal/server/events_service.go](../internal/server/events_service.go)
- [internal/stream/hub.go](../internal/stream/hub.go)
- [internal/cli/runs/follow_tui.go](../internal/cli/runs/follow_tui.go)
- [internal/cli/runs/report_builder.go](../internal/cli/runs/report_builder.go)
- [internal/store/queries/logs.sql](../internal/store/queries/logs.sql)
- [docs/api/OpenAPI.yaml](../docs/api/OpenAPI.yaml)
- [docs/api/paths/runs_id_logs.yaml](../docs/api/paths/runs_id_logs.yaml)
- [docs/api/paths/runs_run_id_repos_repo_id_logs.yaml](../docs/api/paths/runs_run_id_repos_repo_id_logs.yaml)
