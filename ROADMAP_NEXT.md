# Mods E2E — Full Wire (Scheduler, Stages, Streaming, Retention)

Scope: Complete the end-to-end Mods orchestration beyond the minimal claim + ticket SSE loop. Introduce a simple scheduler with lanes/priorities, a stage graph with retries/healing, consistent streaming of ticket/stage/log events, artifact/retention UX, and robust node lifecycle controls.

Documentation:
- Architecture: README.md (keep as canonical)
- APIs: docs/api/OpenAPI.yaml (update where new endpoints/fields appear)
- Node/server packages: internal/nodeagent/*, internal/server/*, internal/workflow/*, internal/store/*
- Template used: ../auto/ROADMAP.md

Legend: [ ] todo, [x] done.

## Scheduler + Queueing (lanes, priority, concurrency)
- [ ] Add lane/priority columns to runs — Simple enums with defaults; per-lane concurrency. — Enables basic fairness
  - Change: internal/store/migrations/XXX_queue_lanes.sql; sqlc updates; internal/store/queries/runs.sql for ClaimRun (ORDER BY created_at, priority)
  - Test: unit tests for ClaimRun ordering and per-lane caps.

- [ ] Concurrency guard per lane — Track in‑flight counts in SQL or memory. — Prevents overload
  - Change: server runtime gate before assigning; optional advisory lock key per lane
  - Test: concurrent claims respect caps.

## Stage Graph + Lifecycle
- [ ] Introduce stage rows on submit — seed stages (plan, orw-apply, orw-gen, llm-plan, llm-exec, human, gates). — Persist for status
  - Change: internal/store/queries/stages.sql and submit handler to create rows; keep minimal graph
  - Test: submit creates expected stage names with pending status.

- [ ] Stage events — Emit `event: stage` frames on start/finish with attempts/error. — CLI follow shows progress
  - Change: server events service adds PublishStage; handlers ack/complete and node uploads (events endpoint) translate to stage updates
  - Test: SSE snapshot shows stage transitions.

- [ ] Healing loop — On build gate failure, append healing stages (#healN) and retry. — Demonstrate auto‑recovery
  - Change: internal/workflow/runner/healing.go; update completion logic to enqueue new stages when failure qualifies
  - Test: failing scenario spawns #heal1 stages and later succeeds.

## Streaming: Logs + Ticket Done
- [ ] Log fan‑out during ingest — Publish `event: log` frames on `/v1/runs/{id}/logs` ingests. — Live logs in `mods logs`
  - Change: internal/server/handlers/handlers_runs_ingest.go — call events.CreateAndPublishLog after storing
  - Test: SSE stream receives structured log frames (timestamp, stream, line).

- [ ] Retention hint on terminal — Publish `event: retention` with TTL/bundle CID at end. — Improves UX
  - Change: events service adds PublishRetention helper; called by completion path once artifacts stored; compute TTL from config
  - Test: CLI prints retention summary line.

## Control and Intents
- [ ] Cancel/resume endpoints under /v1/mods/{id}/* — Wire CLI `mod cancel|resume`. — Lifecycle control
  - Change: internal/server/handlers (new handlers); docs/api paths; CLI already has commands wired to /v1/mods/{id}/cancel/resume
  - Test: unit tests and CLI tests.

- [ ] Drain/undrain integration — Scheduler ignores drained nodes and drains before rollout. — Safer upgrades
  - Change: reuse existing /v1/nodes/{id}/drain|undrain; ensure scheduler consults `drained`
  - Test: node rollout tests verify drained nodes not assigned.

## Artifacts and Downloads
- [ ] Ticket artifacts index — `/v1/mods/{id}/artifacts` lists logical names ↔ CIDs. — Easier CLI download
  - Change: add path + handler; or populate in TicketStatus
  - Test: CLI `mod artifacts` lists items.

- [ ] Retention policy — TTL worker marks bundles for retention; publishes events. — Predictable cleanup
  - Change: internal/store/ttlworker/…; emit events.PublishRetention on cutoff
  - Test: retention event appears near expiry.

## Observability and Hardening
- [ ] mTLS for all node → server uploads in nodeagent — unify clients. — Security
  - Change: refactor logstreamer/diff/artifact/status uploaders to share mTLS http.Client
  - Test: integration tests with certs.

- [ ] Backoff/retry policies for network calls — bounded retries with jitter. — Robustness
  - Change: node uploaders and claim loop; server SSE reconnect hints
  - Test: simulated failures with retry verification.

- [ ] Coverage and fuzz on parsers — SSE, YAML, JSON payloads. — Confidence
  - Change: add fuzz tests for log/event decoding and config loaders
  - Test: `go test -fuzz=…` minimal targets.

## Acceptance Criteria (Full Wire)
- Scheduler assigns work respecting lane/priority and drains; stage graph executes with retries/healing.
- `/v1/mods/{id}/events` streams ticket, stage, log, retention, and done events in order; CLI follow and logs work end‑to‑end.
- Cancel/resume honored; artifacts discoverable and downloadable from ticket context.
- Unit/integration tests green; coverage targets met per AGENTS.md.

