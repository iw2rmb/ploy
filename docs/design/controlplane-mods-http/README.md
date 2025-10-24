# Control Plane Mods & Jobs HTTP Surface (Roadmap 1.3B)

- Status: Draft
- Owner: Codex
- Created: 2025-10-24
- Sequence: 2 of 4 for `docs/next/roadmap.md` item 1.3
- Depends On: [`docs/design/controlplane-auth-surface/README.md`](../controlplane-auth-surface/README.md)

## Summary
Expose the Mods orchestration and job event APIs documented in `docs/next/api.md` through the control-plane server. Provide REST handlers for submitting, resuming, cancelling, and inspecting Mods, alongside streaming job and Mod events.

## Goals
- Implement `/v1/mods` (submit), `/v1/mods/{ticket}` (GET), `/v1/mods/{ticket}/resume`, `/v1/mods/{ticket}/cancel`.
- Serve `/v1/mods/{ticket}/logs` and `/v1/mods/{ticket}/logs/stream` using `internal/node/logstream`.
- Add `/v1/mods/{ticket}/events` SSE endpoint backed by `controlplanemods.Service`.
- Provide `/v1/jobs/{id}/events` for job lifecycle streaming.
- Ensure all handlers enforce scopes via the auth middleware and emit structured errors.

## Non-Goals
- Artifact persistence or registry endpoints (covered in roadmap 1.3C).
- Worker node job execution changes (roadmap section 2).
- CLI tooling updates (roadmap section 3).

## Current State
- `controlplanemods.Service` supports ticket persistence but only `/v1/mods/tickets` (legacy) is exposed.
- SSE hubs exist (`logstream.Hub`, `events.RotationHub`) but not wired to new Mod endpoints.
- Job handlers return basic JSON but lack event streaming.

## Proposed Changes
- Replace legacy `/v1/mods/tickets` endpoints with the new REST surface while keeping backward-compatible aliases until CLI migrates.
- Add handler methods for Mods operations, delegating to `controlplanemods.Service`.
- Implement SSE writers that stream updates from logstream and scheduler event channels with heartbeat/ping support.
- Ensure `/v1/jobs/{id}/events` queries scheduler for job timeline and subscribes to updates.

## Work Plan
1. Model request/response DTOs for Mods operations; include validation.
2. Implement HTTP handlers under new routes using auth helper from 1.3A.
3. Wire SSE streaming for Mods logs/events and job events with tests for connection lifecycle.
4. Update documentation and deprecate `/v1/mods/tickets` in code comments/tests.

## Testing Strategy
- Unit tests for each handler covering success, validation failures, missing tickets, auth errors.
- SSE tests verifying event framing and client disconnect handling.
- Snapshot tests for JSON payloads to guard contract shape.

## Documentation
- Update `docs/next/api.md` examples to match new payloads (ticket JSON, event schema).
- Add CLI usage notes to `docs/next/devops.md` once endpoints considered GA.
- Mention in roadmap tracking that item 1.3 is checkmarked only after all four design docs ship.

## Dependencies
- Requires auth/middleware foundation (1.3A).
- Relies on `controlplanemods.Service` persistence completed in roadmap item 1.2.

## COSMIC Sizing

| Functional process | E | X | R | W | CFP |
|--------------------|---|---|---|---|-----|
| Mods and job events HTTP surface | 1 | 1 | 1 | 1 | 4 |
| TOTAL | 1 | 1 | 1 | 1 | 4 |
