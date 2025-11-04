# PIPES — Stage‑Aware Multi‑Mod Pipelines (Outline)

This document outlines the Pipes feature: run a pipe — an ordered set (or small DAG) of Mods — with stage‑aware ingestion using `stage_id`. A pipe produces a single Mods ticket (the run) with one Stage per step. Nodes tag logs, diffs, and artifact bundles with the Stage ID so the control plane can group results per step.

Status: outline for implementation planning. No CLI/API surface is committed yet.

## Goals
- Compose multiple Mods into a single, stage‑aware ticket.
- Make `stage_id` first‑class for uploads and events so artifacts map to steps.
- Re‑use the Mods facade (`/v1/mods/*`) and existing ingest limits/caps.

Non‑goals (for this slice):
- Full DAG scheduler, retries, or parallel fan‑out (serialize by default).
- New persistence tables; we reuse `runs`, `stages`, `logs`, `diffs`, `artifact_bundles`.

## Key Concepts
- Pipe: a named set of Mods steps (ordered; limited DAG later).
- Stage: a single step within the pipe; persisted in `stages` and identified by a UUID `stage_id`.
- Ticket: the Mods run (one per pipe execution). Ticket ID == `run_id`.
- Build ID: optional sub‑identifier for fine‑grained uploads under a stage.

## Minimal Flow (MVP)
1) Submit a pipe as one Mods ticket. Server creates the run and N stages (one per step).
2) Node claims the run; the server merges the active `stage_id` into the run `spec` for that step.
3) Node executes the step and uploads:
   - Logs: `POST /v1/mods/{ticket}/logs` (annotate `stage_id`, optional `build_id`).
   - Diff: `POST /v1/mods/{ticket}/diffs` (require `stage_id` when produced by a step).
   - Artifacts: `POST /v1/mods/{ticket}/artifact_bundles` (require `stage_id` when produced by a step).
4) Server streams step updates on `GET /v1/mods/{ticket}/events` and finalizes when done.

Notes on today’s code:
- The server already enforces `stage_id` ownership for diffs and artifact bundles (see “Server Enforcement”).
- Logs and structured events accept `stage_id` annotations and validate UUID format but do not enforce membership.

## Pipe Descriptor (proposed)
YAML or TOML payload embedded under `spec` at submission time. Minimal shape:

```yaml
pipe:
  name: build-and-test
  steps:
    - id: plan
      mod: mods-plan
      depends_on: []
    - id: exec
      mod: mods-openrewrite
      depends_on: [plan]
```

Rules:
- `steps[].id` becomes the human label; the server assigns a UUID `stage_id` per step.
- `depends_on` is optional in MVP; if provided, only linear dependencies are honored.
- The server records one Stage row per step and includes the mapping in ticket status.

## Stage ID Semantics
- Generation: server creates a Stage per step at submit time and returns a UUID for each.
- Propagation: when a node claims work for a step, the handler merges `stage_id` into `spec` so the agent can echo it in uploads.
- Upload discipline:
  - Logs/events: `stage_id` optional, best effort association.
  - Diffs/artifacts: `stage_id` required for pipe steps; uploads without it are rejected for Pipes.

## Server Enforcement (existing code)
- Diff upload — validates `stage_id` exists and belongs to the run; rejects otherwise:
  - File: `internal/server/handlers/handlers_runs_ingest.go`
  - Func: `createRunDiffHandler` → checks UUID, `GetStage`, and run ownership; errors:
    - `400 Bad Request` → invalid `stage_id`, or stage does not belong to run.
    - `404 Not Found` → stage not found, or run not found.
    - `413 Payload Too Large` → `patch` > 1 MiB.
- Artifact bundle upload — same validation as diffs:
  - File: `internal/server/handlers/handlers_runs_ingest.go`
  - Func: `createRunArtifactBundleHandler`; same error semantics; 1 MiB bundle cap.
- Logs — UUID format validated, no ownership check (annotation only):
  - File: `internal/server/handlers/handlers_runs_ingest.go`
  - Func: `createRunLogHandler`; enforces 1 MiB gzipped chunk cap.
- Spec merge for nodes — injects the server’s `stage_id` into the JSON `spec` before dispatch:
  - File: `internal/server/handlers/handlers_worker.go`
  - Func: `mergeStageIDIntoSpec`.

Implication for Pipes: with the above checks in place, a mis‑tagged diff/artifact cannot be attached to the wrong run or stage.

## CLI Shape (proposed)
- `ploy pipes run -f pipe.yaml [--follow]` — submit and optionally follow a pipe.
- `ploy pipes status <ticket>` — print ticket state with stage table (state, attempts, last error).
- `ploy pipes artifacts <ticket>` — list artifacts grouped by stage, download by name.

For MVP these commands can wrap existing Mods commands (`ploy mods run/status/artifacts`) and only change the submit payload (`spec.pipe`).

## API Surfaces
- Reuse Mods facade under `/v1/mods`:
  - `POST /v1/mods` — include `spec.pipe` with steps; server creates stages.
  - `GET /v1/mods/{id}` — return `stages` map including `{ stage_id, state, artifacts }`.
  - `GET /v1/mods/{id}/events` — stream ticket + per‑stage updates + `done`.
  - Upload endpoints as in “Minimal Flow”.

OpenAPI TODOs (when implemented):
- Extend `TicketSubmitRequest` with an optional `pipe` object.
- Extend `TicketStatus` to return the step→`stage_id` mapping and per‑stage artifacts.

## Error Model & Limits
- Invalid or foreign `stage_id` on diff/artifact → `400`/`404` as noted above.
- Payload caps: 1 MiB for gzipped log chunk, diff patch, artifact bundle body.
- Oversized request body → `413` with a clear message (handlers clamp sizes today).

## Testing (TDD Plan)
- Unit (RED → GREEN → REFACTOR):
  - Submit with `spec.pipe` creates N stages with stable labels; returns mapping.
  - Diff/artifact with wrong `stage_id` rejected; with correct `stage_id` accepted.
  - Logs accept `stage_id` and persist association (no ownership enforcement).
- CLI unit:
  - `pipes run` builds submit payload; `--follow` renders per‑stage table using existing Mods printer.
- Integration (later): pipe with two steps (`plan` → `exec`) produces a diff in `plan` and a bundle in `exec`; CLI lists both under their stages.

## Environment
- Control plane and auth: see `docs/envs/README.md` (e.g., `PLOY_CONTROL_PLANE_URL`).
- No additional env vars required for MVP; descriptor is provided inline via `spec` or `-f` file.

## Open Questions
- Should logs without `stage_id` be rejected for Pipes (strict mode)?
- Do we support parallel steps in MVP, or serialize strictly by `depends_on`?
- Download convenience: `GET /v1/mods/{id}/artifacts` index by logical name?

## Related
- Mods happy path: `docs/mod-simple-happy-path.md`.
- API spec: `docs/api/OpenAPI.yaml` and components under `docs/api/components/`.

