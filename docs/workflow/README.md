# Workflow Runtime Overview

Ploy Next executes Mods locally on each workstation node. This document summarises how step manifests
are hydrated, executed, and staged for follow-on tasks.

## Runtime Selection

- The CLI defaults to the `local-step` runtime adapter.
- Local execution uses Docker via `github.com/docker/docker` (API negotiation enabled). Containers
  are created with `auto-remove` disabled so retention TTLs from the manifest can be honoured.
- When the Docker daemon is unavailable the adapter surfaces an error immediately instead of
  attempting any legacy fallbacks.

## Workspace Hydration

- Step manifests may reference repository materialization plus optional artifact bundles. The
  workspace hydrator extracts those tarballs from `${PLOY_ARTIFACT_ROOT:-$XDG_CACHE_HOME/ploy/artifacts}`
  into mount-specific directories.
- The first read/write input defines the default working directory inside the container. Read-only
  inputs remain isolated so diffs can be reapplied deterministically.

## Diff Capture

- After container exit the runtime archives the writable mount into a deterministic tarball and
  streams the payload to the configured IPFS Cluster client. The returned CID is recorded on the
  step result so downstream tasks can hydrate the diff from any node.
- Log bundles are captured in-memory and pushed through the same publisher, ensuring both diff and
  log artifacts share replication and verification behaviour.

## Artifact Publishing

- The IPFS Cluster publisher computes a SHA-256 digest for every artifact before upload and stores
  the digest alongside the CID. Workflow checkpoints now reference both values so the CLI can verify
  downloads.
- Replication factors default to the workstation configuration (`PLOY_IPFS_CLUSTER_REPL_MIN` /
  `PLOY_IPFS_CLUSTER_REPL_MAX`) but can be overridden per upload. Operators can use `ploy artifact
  status` to inspect peer health and `ploy artifact rm` to unpin stale artifacts when debugging.
- Stage checkpoints now include diff/log artifacts plus retention metadata (`retain_container` and
  TTL). The CLI surfaces this in the **Stage Artifacts** summary so operators can pull bundles or
  plan inspections without querying the control plane.

## Build Gate

- The runtime adapts the Build Gate sandbox runner via `step.NewBuildGateClient` using the
  embedded Java executor (Gradle/Maven wrappers, with a Dockerised Maven fallback). Static-check
  adapters are temporarily disabled; the sandbox result is recorded in stage metadata and
  failures block downstream stages.
- Once artifact publishing is wired, static-check findings and log digests will be attached to the
  staged report artifact. Java Error Prone integration is implemented but kept disabled by default
  pending artifact plumbing.
