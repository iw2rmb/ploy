# Unified Mods Log Streaming

Scope: Unify Mods log streaming across server and CLI so `ploy mod run --follow`, `ploy mods logs`, and `ploy runs follow` consume the same enriched log payload (timestamp, stream, line, node_id, job_id, mod_type, step_index) for better diagnostics.

Documentation: docs/mods-lifecycle.md, docs/api/components/schemas/controlplane.yaml, docs/api/paths/mods_id_events.yaml, cmd/ploy/README.md.

Legend: [ ] todo, [x] done.

## Data model and SSE payload
- [x] Extend log payload model for Mods streams — define a single enriched log record shape used by the hub and SSE clients.
  - Repository: ploy
  - Component: internal/stream, internal/server/events
  - Scope: Update `internal/stream/hub.go` (`LogRecord`) to add `node_id`, `job_id`, `mod_type`, and `step_index` fields; ensure `internal/server/events/service.go` (`publishLogToHub`, `publishEventToHub`) continues to marshal the updated struct without changing event types.
  - Snippets: `type LogRecord struct { Timestamp, Stream, Line, NodeID, JobID, ModType string; StepIndex int }`
  - Tests: `go test ./internal/stream ./internal/server/events` — hub tests still pass and new fields are present in marshaled JSON.

- [x] Enrich log fanout with execution context — attach node_id and mod_type to each log frame using existing jobs metadata.
  - Repository: ploy
  - Component: internal/server/events, internal/store
  - Scope: In `internal/server/events/service.go`, look up job metadata (node_id, mod_type, step_index) using `store.Job` (e.g., via `GetJob` or a cached map keyed by `log.JobID`); populate the new fields on `LogRecord` before calling `Hub().PublishLog`.
  - Snippets: `job := loadJobForLog(ctx, s.store, log.RunID, log.JobID); record.NodeID = uuidToString(job.NodeID); record.ModType = job.ModType; record.StepIndex = int(job.StepIndex)`
  - Tests: Extend `internal/server/events/service_test.go` and `internal/server/handlers/handlers_events_http_test.go` to assert that SSE `event: log` frames include expected `node_id` and `mod_type` JSON fields for known jobs.

## CLI consumers
- [ ] Introduce a shared CLI log printer — ensure Mods and Runs commands print logs using the same formatting rules.
  - Repository: ploy
  - Component: internal/cli/mods, internal/cli/runs
  - Scope: Extract a common printer (e.g., `internal/cli/logs` or shared helper) that consumes the enriched log JSON (including node_id/mod_type) and implements `structured`/`raw` formats; refactor `internal/cli/mods/logs.go` and `internal/cli/runs/follow.go` to delegate to it.
  - Snippets: Structured line: `2025-10-22T10:00:00Z stdout node=<node_id> mod=<mod_type> step=<step_index> job=<job_id> Step started`
  - Tests: Update `cmd/ploy/mods_logs_test.go` and `internal/cli/runs/follow_test.go` golden expectations to match the unified structured format, keeping `--format raw` as message-only.

- [ ] Wire unified logs into `ploy mod run --follow` — provide a consistent, informative view when following a Mods ticket directly.
  - Repository: ploy
  - Component: cmd/ploy, internal/cli/mods
  - Scope: Keep `internal/cli/mods.EventsCommand` responsible for ticket/stage events but add an option in `cmd/ploy/mod_run_exec.go` (or a follow helper) to also stream enriched log records via the same SSE endpoint, reusing the shared printer so `mod run --follow` looks like `mods logs` plus ticket/state headers.
  - Snippets: Header lines such as `Ticket mods-abc123: running (node=<node_id> base=main target=feature)` followed by unified log lines.
  - Tests: Extend `cmd/ploy/mod_run_follow_test.go` to assert that followed runs print the new header plus enriched log lines, and still handle caps/cancellation correctly.

## API and documentation
- [ ] Document enriched `event: log` payload — keep API, server, and CLI aligned on log semantics.
  - Repository: ploy
  - Component: docs/api, docs
  - Scope: Update `docs/mods-lifecycle.md` SSE section for `/v1/mods/{id}/events` to describe `event: log` as `LogRecord {timestamp, stream, line, node_id, job_id, mod_type, step_index}`; add a short note in `cmd/ploy/README.md` explaining the structured log format used by `mods logs` and `runs follow`.
  - Snippets: `event: log` / `data: {"timestamp":"...","stream":"stdout","line":"...","node_id":"...","job_id":"...","mod_type":"mod","step_index":2000}`
  - Tests: Run `go test ./docs/api/...` (including `docs/api/verify_openapi_test.go`) to ensure schema references remain valid; manually verify rendered docs if applicable.

- [ ] Validate performance and resilience with enriched logs — confirm no regressions for long-running or chatty Mods tickets.
  - Repository: ploy
  - Component: internal/cli/stream, internal/server/events, tests/e2e
  - Scope: Exercise `mods logs` and `runs follow` against a high-volume ticket in the lab (multi-node Mods scenarios) to confirm backoff behavior, idle timeouts, and reconnection semantics remain stable with the larger log frames.
  - Snippets: Commands like `dist/ploy mods logs <ticket-id> --format structured` and `dist/ploy runs follow <ticket-id>` against existing E2E scenarios.
  - Tests: Use `tests/e2e/mods` scenarios to smoke-test the new log shape end-to-end; ensure `make test` (including SSE-related tests) passes with coverage thresholds unchanged.

