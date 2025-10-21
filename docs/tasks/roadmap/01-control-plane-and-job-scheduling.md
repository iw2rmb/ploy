# Control Plane & Job Scheduling

## Why
- Consolidate Mods coordination into a workstation-first control plane that no longer relies on Grid services.
- Ensure each Mod step is scheduled once, supports concurrency, and persists state within etcd as the single source of truth (per `docs/v2/README.md`).

## Required Changes
- Design an etcd keyspace that captures node membership, Mods metadata, and job lifecycle events without Grid fallbacks.
- Implement optimistic concurrency and lease-based TTLs for job claims so only one node executes a step at a time, following current etcd scheduling best practices.citeturn1search9
- Define durable job status records (queued, running, succeeded, failed, inspection-ready) with retention windows and garbage collection hooks.
- Remove all Grid-specific leader election or dependency code paths from the scheduler packages.

## Definition of Done
- Control-plane service exposes APIs for job submission, status query, and worker claims backed solely by the new etcd layout.
- Nodes acquire and release work through documented leases with automatic expiry when a worker disappears.
- Operational runbooks document how to recover stuck jobs by manipulating the new keyspace rather than Grid tools.

## Tests
- Unit tests covering optimistic locking, lease renewal, and failure recovery paths on the scheduler.
- Integration tests that spin up ephemeral etcd instances and multiple worker processes to validate single-claim behaviour and retry semantics.
- Coverage reports confirm ≥90% coverage on scheduler packages, in line with the repo standards.
