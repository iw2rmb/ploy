# Garbage Collection & Retention

## Why

- Ploy v2 introduces retention and garbage collection workflows for artifacts,
  logs, and job metadata (`docs/v2/gc.md`).
- Proper cleanup is required to keep IPFS storage and etcd keyspace healthy
  without relying on Grid GC jobs.

## Required Changes

- Implement GC controllers that respect retention policies for logs, artifacts,
  and job records, coordinating with IPFS unpin operations.
- Provide CLI commands to preview GC actions, schedule manual runs, and override
  retention for investigations.
- Integrate GC telemetry with the observability stack to report reclaimed
  storage and surfaced errors.
- Document operational safeguards (rate limits, dry runs) and default retention
  profiles per environment.

## Definition of Done

- GC runs automatically on configurable intervals, producing auditable logs and
  metrics.
- Operators can trigger targeted cleanups via CLI without manual data store
  manipulation.
- Retention defaults are documented, communicated, and validated through smoke
  tests.

## Tests

- Unit tests for retention policy evaluation, selection of deletion candidates,
  and IPFS unpin logic.
- Integration tests simulating artifacts aging out, ensuring GC removes data
  safely and updates etcd.
- Regression tests verifying GC skips in-progress jobs and preserves inspection
  windows.
