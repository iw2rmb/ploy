# Workflow Runtime Overview

Ploy v2 executes Mods locally on each workstation node. This document summarises how step manifests
are hydrated, executed, and staged for follow-on tasks.

## Runtime Selection

- The CLI defaults to the `local-step` runtime adapter. Set `PLOY_RUNTIME_ADAPTER=grid` to force the
  legacy Grid path when debugging environments that still depend on Grid RPC.
- Local execution uses Docker via `github.com/docker/docker` (API negotiation enabled). Containers
  are created with `auto-remove` disabled so retention TTLs from the manifest can be honoured.
- When the Docker daemon is unavailable the adapter surfaces an error immediately instead of
  falling back to Grid.

## Workspace Hydration

- Step manifests reference snapshot and diff CIDs. The workspace hydrator extracts those tarballs
  from `${PLOY_ARTIFACT_ROOT:-$XDG_CACHE_HOME/ploy/artifacts}` into mount-specific directories.
- The first read/write input defines the default working directory inside the container. Read-only
  inputs (snapshots) remain isolated so diffs can be reapplied deterministically.

## Diff Capture

- After container exit the runtime archives the writable mount into a deterministic tarball. The
  tarball path is returned to the runner and staged under the artifact cache using a content-hash CID.
- Future slices publish the staged tarball to IPFS Cluster; until then the CID references the cached
  file on disk so downstream tasks can inspect results locally.

## Artifact Staging

- The filesystem artifact publisher writes diff tarballs and log buffers under the artifact root with
  per-kind subdirectories (`diffs/`, `logs/`). Each file name is derived from the SHA-256 hash
  prefixed with `ipfs:`.
- Manifests and job outcomes store these CIDs so the dedicated artifact-store task
  (`docs/tasks/roadmap/03a-mod-runtime-artifacts.md`) can promote them to IPFS Cluster.

## SHIFT Enforcement

- The runtime adapts the build gate sandbox runner via `step.NewBuildGateShiftClient`. Static-check
  adapters are temporarily disabled; the sandbox result is still recorded in stage metadata and
  failures block downstream stages.
- Once artifact publishing is wired, static-check findings and log digests will be attached to the
  staged report artifact.
