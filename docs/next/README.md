# Ploy Next Overview

Ploy Next unifies Mods execution, SHIFT validation, and artifact handling into a
workstation-first stack. Lightweight `ployd` nodes coordinate Mods locally,
while an etcd-backed control plane assigns work, enforces optimistic
concurrency, and
publishes durable artifacts in IPFS Cluster. The CLI remains the primary
operator surface, providing familiar workflows without requiring the legacy
Grid stack.

## Goals

- Deliver workstation-first Mods orchestration without Grid or JetStream.
- Keep Mods steps deterministic by replaying repository snapshots plus ordered
  diffs on every node.
- Streamline artifact publishing so diffs, logs, and SHIFT reports replicate
  across the cluster through IPFS Cluster.
- Provide observability from the first release: job logs, retention metadata,
  and health endpoints behave the same on laptops and shared clusters.

## Non-Goals

- Multi-cluster federation or dedicated discovery nodes (future work).
- Hybrid support for the Grid workflow runner; v2 replaces Grid entirely.
- Rewriting SHIFT itself—Ploy consumes the existing SHIFT build gate APIs.

## Architecture Summary

### Mods Lifecycle

Mods stay the primary unit of work. Each Mod expands into a typed plan and runs
as an ordered set of jobs (plan, LLM, rewrite generation, rewrite apply,
validation). Every job executes in an OCI image, sees repository plus
cumulative diffs, and produces a diff tarball plus logs that move through the
SHIFT sandbox before the next step.

### Cluster Roles

- Control plane schedules jobs, stores job metadata in etcd, and exposes the
  `/v1/jobs` APIs.
- Nodes hydrate workspaces, launch containers, run SHIFT, and publish artifacts
  to IPFS Cluster.
- Beacon mode distributes discovery data (DNS bootstrap, trust bundles) while
  still participating in job execution when capacity allows.
- The CLI submits Mods, manages artifacts, administers nodes, and bootstraps
  clusters.

## Component Responsibilities

- **Ploy CLI** — Operator interface for Mods submission, artifact management,
  and node lifecycle. See [docs/next/cli.md](cli.md).
- **Control Plane Service (ployd)** — `/v1/jobs` HTTP APIs for submission,
  worker claims, heartbeats, status queries, and job completion. The ployd
  daemon fronts these routes, wrapping the etcd-backed scheduler with
  optimistic concurrency.
- **Ploy Nodes (ployd workers)** — `ployd` worker daemons hosting Docker,
  SHIFT, IPFS Cluster client, and etcd connectivity. Execute Mod steps, persist
  job state, and stream logs back to the CLI.
- **SSH Tunnel Manager** — `pkg/sshtransport` plus cached descriptors keep
  persistent SSH tunnels alive so the CLI can reach control-plane HTTP APIs
  without provisioning separate beacons or TLS bundles.
- **SHIFT Build Gate** — Executes unit tests and static analysis per step;
  reused from the existing integration without embedding its CLI.
- **IPFS Cluster** — Artifact store for snapshots, diff bundles, logs, and OCI
  layers. Cluster pinning replaces embedded IPFS nodes.
- **etcd** — Backing store for node membership, Mods metadata, queue state, and
  coordination leases.
- **GitLab Integration** — Control plane stores GitLab API keys in etcd so
  nodes can authenticate when cloning repositories or opening merge requests.

## Execution Pipeline

1. Operator submits a Mod via the CLI, including target repository, manifest,
   and optional overrides (e.g., build gate profile, plan heuristics).
2. Control plane records the job (`mods/<ticket>/jobs/<job-id>`), enqueues the
   work (`queue/mods/<priority>/<job-id>`), and exposes status over `/v1/jobs`.
3. A node claims the job through `/v1/jobs/claim`, hydrates the workspace from
   snapshot and diff CIDs, and launches the specified container with retention
   enabled for inspection.
4. On exit, the node captures stdout/stderr, diff tarball, and metadata before
   invoking SHIFT to run tests and static analysis.
5. Artifacts (diffs, logs, SHIFT report) publish to IPFS Cluster; the node
   records resulting CIDs, digests, and retention windows back in etcd.
6. Control plane updates job state, surfaces observability (SSE log streams,
   status poll APIs), and triggers GC markers for later retention enforcement.

## Data & Artifact Management

The control plane persists canonical job records and queue entries in etcd,
using transactions and leases to guarantee single-worker claims. IPFS Cluster
stores snapshots, diffs, logs, and SHIFT reports so any node can hydrate the
same state deterministically. Artifact CIDs live alongside job metadata,
allowing the CLI to pull specific bundles or hydrate new Mods with cached data.

## Interfaces & Access

- CLI command reference — [docs/next/cli.md](cli.md)
- Control plane APIs — [docs/next/api.md](api.md)
- Job execution model — [docs/next/job.md](job.md)
- Mods workflow example — [docs/next/mod.md](mod.md)
- IPFS artifact handling — [docs/next/ipfs.md](ipfs.md)
- SHIFT integration — [docs/next/shift.md](shift.md)

## Operations & Observability

- Nodes stream container stdout/stderr over SSE so operators can tail progress
  live, even on workstation runs.
- Containers remain available after job completion for inspection; retention
  policies govern when GC prunes them.
- Metrics capture queue depth, claim latency, lease expirations, retries, and
  SHIFT duration. Health endpoints report etcd connectivity and backlog size.
- Prometheus scraping and alerting guidance — [docs/next/observability.md](observability.md)
- Garbage collection controllers respect retention windows defined in
  [docs/next/gc.md](gc.md).

## Operational Baseline

| Dependency      | Minimum version (2025-10-22) | Notes |
|-----------------|------------------------------|-------|
| etcd cluster    | 3.6.x (recommend 3.6.5)      | Leverages the 3.6 feature set (livez/readyz, downgrade RPC) and security fixes released through September 2025. |
| IPFS Cluster    | 1.1.4                        | Provides the pin tracker improvements and metrics emitted in May 2025, matching the artifact replication strategy. |
| Docker Engine   | 28.x                         | Required for the BuildKit and container retention defaults used by the step runner. |
| Go toolchain    | 1.25                          | Matches the module target (`go 1.25`) and unlocks Go 1.25 runtime improvements relevant to the control plane. |

## Adoption Path

Ploy Next rolls out sequentially: control plane scheduler, step runtime, artifact
publisher, CLI refresh, and deployment tooling. Follow
[`docs/next/migration.md`](migration.md) for the phase-by-phase plan, the
dependencies between components, and cleanup guidance for retiring Grid.

## Further Reading

- [docs/next/cli.md](cli.md) — Command-line reference.
- [docs/next/api.md](api.md) — REST route catalog for control plane and node APIs exposed over SSH tunnels.
- [docs/next/job.md](job.md) — Job abstraction, log streaming, and retention guarantees.
- [docs/next/mod.md](mod.md) — Example Mods workflow (Java 11 → Java 17 upgrade) illustrating
  end-to-end orchestration.
- [docs/next/reuse.md](reuse.md) — Grid components worth reusing in Ploy Next.
- [docs/next/devops.md](devops.md) — Deployment, bootstrap, and node operations playbook.
- [docs/next/ipfs.md](ipfs.md) — IPFS Cluster topology, replication, and operational guidance.
- [docs/next/ssh-transfer-migration.md](ssh-transfer-migration.md) — Migration plan for the SSH slot
  workflow and CLI transfer commands.
- [docs/next/gc.md](gc.md) — Retention and garbage collection workflow (controller + `ploy gc`).
- [docs/next/logs.md](logs.md) — Log streaming and archival strategy (metadata in etcd, payloads in IPFS).
- [docs/next/queue.md](queue.md) — etcd-backed queue, capacity tracking, and scheduling behaviour.
- [docs/next/etcd.md](etcd.md) — etcd keyspace layout and contracts.
- [docs/next/testing.md](testing.md) — Testing requirements (unit/integration coverage, timeouts).
- [docs/next/vps-lab.md](vps-lab.md) — Shared VPS lab environment for integration and E2E testing.
- [docs/next/shift.md](shift.md) — Simplifying the SHIFT build gate for Ploy Next.
