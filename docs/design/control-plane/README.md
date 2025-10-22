# Control Plane Scheduler

- **Identifier**: `roadmap-control-plane`
- **Status**: Completed — 2025-10-21
- **Upstream Docs**: `../../v2/README.md`, `../../v2/queue.md`, `../../v2/etcd.md`, `../../v2/job.md`

## Why

- Eliminate the Grid dependency chain by scheduling Mods steps directly through Ploy.
- Provide a durable job record (queue + status) that survives node failures and powers CLI status
  queries.
- Ensure every worker claim happens at most once by relying on etcd transactions and leases.

## What to do

- Expose `/v2/jobs` HTTP APIs for submission, claim, heartbeat, completion, and listing.
- Persist job state under `mods/<ticket>/jobs/<job-id>` and queue entries under
  `queue/mods/<priority>/<job-id>` with optimistic concurrency.
- Attach leases to claimed jobs so expired leases automatically re-queue the work.
- Emit GC markers to support the retention controller described in `docs/v2/gc.md`.

## Where to change

- `internal/controlplane/scheduler` — job lifecycle, queue transactions, lease plumbing.
- `internal/controlplane/httpapi` — REST handlers, validation, SSE log piping.
- `internal/workflow/runtime` — default to the new scheduler adapter and remove Grid wiring.
- `cmd/ploy` — surface control plane endpoints for CLI commands (`ploy status`, `ploy jobs`).

## How to test

- `go test ./internal/controlplane/...` — unit coverage for scheduler and HTTP handlers.
- `go test -tags integration ./tests/integration/controlplane` — embedded etcd race coverage for
  claims and lease expiry.
- `staticcheck ./internal/controlplane/...` — lint guardrail.
- CLI smoke: `make build && dist/ploy status` against a dev cluster to verify job listing and health.
