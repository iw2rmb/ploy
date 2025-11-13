# Mods Simple Happy Path — Minimal Claim Loop + Ticket SSE

This document describes the end‑to‑end flow after implementing the “minimal” roadmap
(Node claim→ack→complete loop and ticket SSE events). It shows what the CLI sends,
what the server records/streams, and what the node executes/returns.

Assumptions:
- Server is running with `/v1/mods` facade and `/v1/nodes/*` endpoints.
- At least one node is provisioned and heartbeating (`/v1/nodes/{id}/heartbeat`).
- CLI has a control‑plane descriptor or `PLOY_CONTROL_PLANE_URL` set.

## 1) Submit Ticket (CLI → Server)
- Request: `POST /v1/mods`
  - Body: `{ repo_url, base_ref, target_ref, commit_sha? (optional), spec? (optional), created_by? }`
- Server:
  - Insert run row: `status=queued`, `created_at=now()`
  - Publish SSE “ticket” event: `{ ticket_id, state: "queued", repo_url, base_ref, target_ref }`
- Response: `201 Created { ticket_id, status, repo_url, base_ref, target_ref }`

## 2) Start Following (CLI ⇄ Server)
- Request: `GET /v1/mods/{ticket_id}/events` (SSE)
- Stream: receives the “queued” ticket event from step 1; uses `Last-Event-ID` for resume.

## 3) Claim Work (Node → Server, loop)
- Request: `POST /v1/nodes/{node_id}/claim`
- Response:
  - `200 OK` when a run is assigned:
    - `{ id (run_id), repo_url, status: "assigned", node_id, base_ref, target_ref, commit_sha?, started_at, created_at }`
  - `204 No Content` when no work is available
- Server: atomically transitions the earliest queued run to `assigned`, sets `node_id`, `started_at=now()`.

## 4) Acknowledge Start (Node → Server)
- Request: `POST /v1/nodes/{node_id}/ack`
  - Body: `{ run_id }`
- Server:
  - Update run: `status=running`
  - Publish SSE “ticket” event: `{ ticket_id, state: "running" }`
- Response: `204 No Content`

## 5) Execute on Node (Node local work)
- Inputs (from claim): `repo_url`, `base_ref`, `target_ref`, `commit_sha?`
- Node does:
  - Materialize repo (shallow clone) per refs
  - Run the current single‑step executor (build gate placeholder for this slice)
  - Stream logs and upload diffs/artifacts as produced

## 6) Stream Logs (Node → Server)
- Request: `POST /v1/nodes/{node_id}/logs`
  - Body: `{ run_id, stage_id?, build_id?, chunk_no, data (gzipped ≤1 MiB) }`
- Server: persist log chunk (no SSE fan‑out required in “minimal”).
- Response: `201 Created { id, chunk_no }`

## 7) Upload Diff (Node → Server)
- Request: `POST /v1/nodes/{node_id}/stage/{stage_id}/diff`
  - Body: `{ run_id, patch (gzipped ≤1 MiB), summary (JSON) }`
- Server: persist diff row.
- Response: `201 Created { diff_id }`

## 8) Upload Artifact Bundle (Node → Server)
- Request: `POST /v1/nodes/{node_id}/stage/{stage_id}/artifact`
  - Body: `{ run_id, build_id?, name?, bundle (gzipped tar ≤1 MiB) }`
- Server: persist artifact, compute `{ cid, digest }` for addressability.
- Response: `201 Created { artifact_bundle_id }`

## 9) Complete Run (Node → Server)
- Request: `POST /v1/nodes/{node_id}/complete`
  - Body: `{ run_id, status: "succeeded"|"failed"|"cancelled", reason?, stats? (JSON) }`
- Server:
  - Update run: `status=<terminal>`, `finished_at=now()`, `stats=<payload>`
  - Publish SSE “ticket” event with final state
  - Publish SSE “done” status to terminate the stream cleanly
- Response: `204 No Content`

## 10) CLI Follow Terminates (CLI ⇄ Server)
- SSE stream receives the final “ticket” event, then the “done” status.
- CLI exits with the final ticket state.

## 11) Optional: Artifact Download (CLI → Server)
- List by CID: `GET /v1/artifacts?cid=<cid>` → choose artifact `id`
- Download: `GET /v1/artifacts/{id}?download=true` → bytes
- Note: For the minimal slice, ticket status does not need to enumerate stage artifacts; CID lookup suffices for ad‑hoc fetches.

## Summary of Exchanges
- CLI → Server: submit, follow (SSE), optional artifact download
- Node → Server: claim → ack → logs/diffs/artifacts → complete
- Server → CLI (SSE): ticket events (queued, running, terminal) + done
