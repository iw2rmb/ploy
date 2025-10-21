# Job Observability & Logs

## Why
- Every Mod step and SHIFT run must persist stdout/stderr, expose SSE tails, and keep containers available for inspection (`docs/v2/job.md` and `docs/v2/logs.md`).
- Operators need first-class observability that no longer depends on Grid logging pipelines.

## Required Changes
- Build a log aggregation pipeline that streams container output to etcd metadata and IPFS payload storage, exposing SSE endpoints through the control plane.
- Implement retention policies and garbage collection hooks aligned with `docs/v2/gc.md`, configurable per Mod or organization.
- Add tracing and metrics instrumentation (timings, exit codes, retry counts) exported to the observability stack.
- Provide CLI tooling for real-time tails, historical log pulls, and inspection of retained containers.

## Definition of Done
- SSE log streaming works for active jobs with documented reconnect semantics and back-pressure handling.
- Historical logs and artifacts can be fetched after job completion, respecting retention windows and ACLs.
- Observability dashboards surface run-level metrics and alert on failures or stalled jobs.

## Tests
- Unit tests for log persistence adapters, SSE streaming handlers, and retention policy evaluation.
- Integration tests capturing live container output, verifying IPFS archival, and replaying logs via CLI.
- Load tests simulating high-volume log streams to validate back-pressure and resource usage.
