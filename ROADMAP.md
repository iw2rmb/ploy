# Mods Minimal E2E — Node Claim Loop + Ticket SSE

Scope: Wire the simplest end-to-end execution path so `dist/ploy mod run … --follow` transitions from queued → running → succeeded/failed using the pull‑based node claim loop and ticket SSE events. No scheduler, lane priorities, or full stage graph yet; logs/diffs/artifacts continue to use existing node ingest endpoints.

Documentation:
- Architecture and API overview: README.md
- Event streaming hub: internal/stream/*, internal/server/events/service.go
- Server handlers: internal/server/handlers/* (runs/nodes/mods)
- Node agent: internal/nodeagent/* (agent, heartbeat, logstreamer, status/diff/artifact uploaders)
- CLI follow: internal/cli/mods/events.go
- Template used: ../auto/ROADMAP.md

Legend: [ ] todo, [x] done.

## Phase 1 — Node Claim→Ack→Complete loop (pull‑based)
- [x] Add claim loop to node agent — Poll POST /v1/nodes/{id}/claim; on 200, ack, execute, complete. — Enables execution without server‑push
  - Change: internal/nodeagent/agent.go (start background `claimLoop` alongside heartbeat); add new file internal/nodeagent/claimer.go with backoff, 204 handling, and JSON decode of claim response (fields from handlers_worker.go).
  - Change: internal/nodeagent/agent.go — After ack, invoke controller.StartRun with StartRunRequest mapped from claim response (repo_url/base_ref/target_ref/commit_sha). On return, use StatusUploader to POST terminal status.
  - Test: unit test with httptest server stubbing /claim→/ack→/complete; verify loop posts in order and transitions once per claim.

- [x] Implement ack on node — POST /v1/nodes/{id}/ack with run_id before execution. — Moves run to running per server contract
  - Change: internal/nodeagent/claimer.go — call ack before StartRun;
  - Test: claim loop test asserts ack call made prior to StartRun invocation.

- [x] Implement complete on node — POST /v1/nodes/{id}/complete with status, optional reason/stats. — Terminates run on server
  - Change: reuse internal/nodeagent/statusuploader.go (UploadStatus) from controller outcome; map success/failure.
  - Test: verify 200 paths and retry on transient 5xx (basic backoff).

## Phase 2 — Ticket SSE events (submit/ack/complete)
- [x] Add PublishTicket API to events service — Fan‑out `event: ticket` with modsapi.TicketSummary/Status payload. — Lets CLI follow see lifecycle
  - Change: internal/server/events/service.go — add `PublishTicket(ctx, runID, payload)` that emits type "ticket" on stream `<run-uuid>`.
  - Test: unit test builds TicketSummary and asserts hub snapshot contains `event:ticket` frames.

- [x] Emit queued event on submit — Immediately after run creation. — Follow shows "queued"
  - Change: internal/server/handlers/handlers_mods_ticket.go — after success, build minimal TicketSummary and call events.PublishTicket.
  - Test: handler unit test with a fake service; ensure one `ticket` event emitted.

- [ ] Emit running event on ack — After Update status to running. — Follow shows “running”
  - Change: internal/server/handlers/handlers_worker.go (ackRunStartHandler) — publish TicketSummary(status=running).
  - Test: handler test verifies PublishTicket called.

- [ ] Emit terminal + done on complete — After UpdateRunCompletion. — Follow stops (final ticket state) and consumers see `done`
  - Change: handlers_worker.go (completeRunHandler) — publish TicketSummary with final status; also events.Hub().PublishStatus(streamID, done).
  - Test: handler test checks both calls.

## Phase 3 — Glue and guards
- [ ] Map claim response → StartRunRequest — Ensure StartRun uses repo + base/target refs. — Execution succeeds with current runner
  - Change: internal/nodeagent/agent.go buildManifestFromRequest inputs
  - Test: agent test validates manifest URL/refs.

- [ ] Idle/health checks — Backoff when no work; keep heartbeats. — Reduce noisy logs
  - Change: claimer loop backoff (250ms→5s exponential with cap).
  - Test: loop test observes sleep growth and reset on success.

## Acceptance Criteria
- Node claims runs and posts ack/complete; server DB reflects status transitions.
- Events stream `/v1/mods/{id}/events` shows `ticket` events (queued→running→{succeeded|failed|cancelled}) and ends with `done`.
- `dist/ploy mod run … --follow` exits with the final ticket state.
