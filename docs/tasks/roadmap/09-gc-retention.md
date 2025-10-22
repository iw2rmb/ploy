# roadmap-gc-retention-09 — Artifact & Container Retention

- **Status**: Planned — 2025-10-22
- **Dependencies**: `docs/design/gc-retention/README.md` (to be authored), `docs/v2/gc.md`, `docs/v2/ipfs.md`

## Why

- Ploy must enforce retention policies for job metadata, containers, logs, and diff bundles so nodes
  do not exhaust storage.
- Operators need deterministic garbage collection with safe inspection windows and audit logging.

## What to do

- Implement the GC controller that scans etcd `gc/jobs/<job-id>` markers, removes expired job state,
  unpins IPFS artifacts, and instructs nodes to prune retained containers.
- Add `ploy gc` CLI support for manual sweeps, dry runs, and per-job inspection overrides.
- Record GC actions and outcomes for auditability, emitting metrics on deletions and failures.

## Where to change

- `internal/controlplane/gc` (new) — controller loop coordinating etcd markers, IPFS unpins, and node
  prune RPCs.
- `cmd/ploy/gc` — CLI entry point for manual garbage collection commands.
- `docs/v2/gc.md`, `docs/v2/logs.md` — document retention windows, override flags, and failure
  recovery.

## How to test

- `go test ./internal/controlplane/gc/...` — unit tests covering expiration selection, IPFS unpin
  retries, and node prune requests.
- Integration: run jobs with short retention windows, confirm GC removes job state and unpins
  artifacts while respecting inspection-ready jobs.
- CLI smoke: `make build && dist/ploy gc --dry-run` to validate reporting.
