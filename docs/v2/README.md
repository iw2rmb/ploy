# Ploy v2 Overview

Ploy v2 consolidates Mods execution, build validation, and artifact handling into a single
workstation-first stack. The goal is to replace the Grid dependency chain with lightweight
Ploy nodes that coordinate Mods runs, build gates (SHIFT), and artifact storage (IPFS Cluster)
while keeping the CLI experience familiar.

## Core Mods Feature

- Mods remain the primary unit of work: each Mod starts from a repository, expands into a plan,
  and executes a sequence of typed steps (plan, LLM, rewrite generation, rewrite apply, validation).
- Every step runs inside an OCI image, receives repository plus cumulative diffs as inputs, executes
  via the node-local Docker runtime, produces a diff tarball, and passes through the SHIFT sandbox
  before continuing.
- The control plane may assign independent steps to different nodes when a Mod’s dependency graph
  allows for concurrency. Each step is claimed once via the job records in etcd, so no additional
  leader election or distributed locks are required.
- Artifacts (diffs, archives, logs) are published to IPFS Cluster so any Ploy node can hydrate the
  same state deterministically.

## Components & Responsibilities

- **Ploy CLI** — Operator interface for submitting Mods, managing artifacts, administering nodes,
  and bootstrapping a cluster.
- **Ploy Nodes** — Worker daemons hosting Docker, SHIFT, IPFS Cluster client, and etcd connectivity.
  They execute Mod steps, persist job state, and stream logs back to the CLI.
- **Control Plane Service** — Exposes `/v2/jobs` APIs for submission, worker claims, heartbeats,
  status queries, and completion. Wraps the etcd-backed scheduler and enforces optimistic
  concurrency with leases.
- **Ploy Node (beacon mode)** — A standard node operating in discovery mode: serves DNS bootstrap,
  distributes API endpoints and trust bundles, and still participates in job execution when capacity
  allows.
- **SHIFT Build Gate** — Runs unit tests and static analysis per step; reused from the existing
  integration without embedding its CLI.
- **IPFS Cluster** — Externalized artifact store for repositories, step diffs, logs, and OCI layers.
  Cluster pinning replaces embedded IPFS nodes.
- **etcd** — Control-plane backing store for node membership, Mods metadata, job scheduling,
  and coordination.
- **GitLab Integration** — GitLab API keys and project metadata live in etcd so nodes can
  authenticate when cloning repositories or opening merge requests across all Mods.

## Ploy CLI Overview

The CLI remains the operator’s primary touchpoint for Mods execution, artifact management, and
cluster administration. Commands cover Mods lifecycle actions, artifact uploads/downloads, node
maintenance, bootstrap flows, and observability tooling. See [docs/v2/cli.md](cli.md) for the
complete reference.

## GitLab Integration

Ploy v2 treats GitLab as the canonical source and destination for Mods. Operators store a single
GitLab API key (or key set) in etcd via `ploy config set gitlab.api_key=...`, letting nodes
authenticate when cloning repositories, creating branches, or publishing merge requests. Credentials
replicate through the control plane over mutual TLS, so nodes never rely on local static secrets.

## API Surfaces

Ploy v2 introduces dedicated APIs for the control plane, nodes, and beacon. Refer to
[docs/v2/api.md](api.md) for route details and security expectations.

## Job Execution

Every Mod step and build gate run executes as a durable job. Containers remain available after exit
for inspection, stdout/stderr are persisted, and logs stream over SSE so operators can tail
progress. See [docs/v2/job.md](job.md) for the full execution model.

## Grid Component Reuse

Ploy v2 deliberately reuses proven Grid components (beacon, job runtime, build gate) to reduce risk
and keep behaviour aligned. See [docs/v2/reuse.md](reuse.md) for a detailed inventory of recommended
modules and code paths to port from `../grid`.

## Deploy

Operators can bootstrap a Ploy cluster (beacon mode, etcd, IPFS Cluster, CA generation) and add
worker nodes with the same workflow. See [docs/v2/devops.md](devops.md) for step-by-step deployment
and operational guidance, including prerequisites (SSH, Linux builds, user provisioning) and
post-install checks.
For IPFS-specific topology and replication details, refer to [docs/v2/ipfs.md](ipfs.md).

## Further Reading

- [docs/v2/cli.md](cli.md) — Command-line reference.
- [docs/v2/api.md](api.md) — REST route catalog for control plane, nodes, and beacon.
- [docs/v2/job.md](job.md) — Job abstraction, log streaming, and retention guarantees.
- [docs/v2/mod.md](mod.md) — Example Mods workflow (Java 11 → Java 17 upgrade) illustrating
  end-to-end orchestration.
- [docs/v2/reuse.md](reuse.md) — Grid components worth reusing in Ploy v2.
- [docs/v2/devops.md](devops.md) — Deployment, bootstrap, and node operations playbook.
- [docs/v2/migration.md](migration.md) — Development roadmap for landing Ploy v2 features (no backward compatibility).
- [docs/v2/ipfs.md](ipfs.md) — IPFS Cluster topology, replication, and operational guidance.
- [docs/v2/gc.md](gc.md) — Retention and garbage collection workflow (controller + `ploy gc`).
- [docs/v2/logs.md](logs.md) — Log streaming and archival strategy (metadata in etcd, payloads in IPFS).
- [docs/v2/queue.md](queue.md) — etcd-backed queue, capacity tracking, and scheduling behaviour.
- [docs/v2/etcd.md](etcd.md) — etcd keyspace layout and contracts.
- [docs/v2/testing.md](testing.md) — Testing requirements (unit/integration coverage, timeouts).
- [docs/v2/vps-lab.md](vps-lab.md) — Shared VPS lab environment for integration and E2E testing.
- [docs/v2/shift.md](shift.md) — Simplifying the SHIFT build gate for Ploy v2.
