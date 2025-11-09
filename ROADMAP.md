# Mods: Cancel Ticket (Full Wire)

Scope: Implement server/API support for cancelling a Mods ticket to match the existing CLI (`ploy mod cancel`). Idempotent endpoint transitions a run and any in‑flight/pending stages to `canceled`, persists optional reason, and publishes a terminal ticket event over SSE so `--follow` exits cleanly.

Documentation: docs/api/OpenAPI.yaml; docs/api/paths/mods_id.yaml; new docs/api/paths/mods_id_cancel.yaml; internal/server/handlers/*; internal/store/*; internal/cli/mods/cancel.go (already implemented).

Legend: [ ] todo, [x] done.

## API Definition
- [x] Define `POST /v1/mods/{id}/cancel` — Control-plane cancel request
  - Component: ploy (server, docs)
  - Change: add `docs/api/paths/mods_id_cancel.yaml` with `id` (uuid) and optional JSON body `{ reason?: string }`; responses `202 Accepted` on state transition, `200 OK` when already terminal, `404` when not found, `400` invalid id
  - Test: docs compile/validation (existing docs/api package), route exercised by handler tests — verify status codes

## Route Registration
- [x] Register cancel endpoint — Wire HTTP handler and auth
  - Component: internal/server/handlers
  - Change: add `cancelTicketHandler(st, eventsService)`; mount in `register.go` as `POST /v1/mods/{id}/cancel` with `auth.RoleControlPlane`
  - Test: unit test mounts handler on `httptest` server and asserts 202/200/404 paths

## Handler Logic
- [x] Implement cancel handler — Persist state, publish SSE
  - Component: internal/server/handlers
  - Change: in `cancelTicketHandler`: parse `{id}`, decode optional `{reason}`; load run; if terminal → `200 OK`; else `UpdateRunStatus(id, 'canceled', reason, now())`; for each stage with `pending|running` call `UpdateStageStatus(stage_id, 'canceled', started_at?, finished_at=now(), duration)`; publish `events.PublishTicket(ctx, runID, TicketSummary{state=cancelled, metadata.reason})`
  - Test: verify DB mutations, `TicketStateCancelled` event present via `eventsService.Hub().Snapshot(runID)`

## Store Usage (Minimal)
- [x] Use existing queries — Avoid migrations
  - Component: internal/store
  - Change: reuse `UpdateRunStatus`, `ListStagesByRun`, `UpdateStageStatus`; no schema changes; optional TODO for a bulk `CancelStagesByRun` query if needed for performance
  - Test: handler test asserts affected stages are updated to `canceled`

## CLI (No-op)
- [x] CLI command exists — Accepts `--ticket` and optional `--reason`
  - Component: cmd/ploy, internal/cli/mods
  - Change: none; already posts to `/v1/mods/{ticket}/cancel`; help/completions present
  - Test: existing `cmd/ploy/mod_cancel_test.go` and `internal/cli/mods/commands_test.go`

## SSE Integration
- [x] Ensure terminal event visible to followers — Client exits
  - Component: internal/server/events
  - Change: use `PublishTicket(... state=cancelled ...)` on success; optional: also emit `PublishStatus(done)` for stream completion
  - Test: `mods events` client test observes `cancelled` and returns terminal state

## OpenAPI Wiring
- [x] Reference path in spec — Keep docs in sync
  - Component: docs/api/OpenAPI.yaml
  - Change: add `/v1/mods/{id}/cancel` under paths; link to `paths/mods_id_cancel.yaml`
  - Test: PR review gate runs `go test ./docs/api` (no code), manual lint if configured

## TDD/Quality
- [ ] RED → GREEN → REFACTOR — Cover handler and edge cases
  - Component: internal/server/handlers
  - Change: add tests: (1) happy path 202; (2) idempotent 200 when already terminal; (3) 404 missing run; (4) bad id 400; (5) stages transitioned
  - Test: aim ≥60% overall and ≥90% on handler package slices per AGENTS.md

## Rollout & Risks
- [ ] Rollout notes
  - Component: docs
  - Change: note idempotency, no DB migrations, symmetric `resume` tracked separately; ensure mTLS RoleControlPlane
  - Test: smoke with `ploy mod run --follow --cap 1s --cancel-on-cap` against lab; expect immediate cancellation

