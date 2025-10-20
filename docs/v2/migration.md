# Ploy v2 Development Plan

This document outlines the implementation path for evolving the existing Ploy codebase into the
Ploy v2 architecture (beacon-mode nodes, etcd coordination, IPFS Cluster artifacts, v2 CLI).
Backward compatibility with the Grid-based runtime is **not** required; the roadmap replaces the
previous workflow stack entirely.

## Goals

- Replace Grid RPC + JetStream dependencies with native Ploy node APIs, etcd, and IPFS Cluster.
- Deliver a unified job model with durable metadata, artifact CIDs, and streamed logs.
- Reshape the CLI and API surfaces to match the v2 documentation.
- Provide deployment tooling (bootstrap + node onboarding) using the embedded shell script workflow.

## Pre-work

1. **Repository hygiene**
   - Remove or archive Grid-specific adapters, feature flags, and environment variables.
   - Annotate remaining cross-repo references (grid SDK imports, JetStream helpers) for later deletion.
2. **Docs alignment**
   - Keep `docs/v2/*` in sync with ongoing changes; treat these documents as the contract.

## Phase 1 — Foundations

1. **Runtime scaffolding**
   - Introduce `internal/workflow/runtime` v2 registry that defaults to the new Ploy adapter.
   - Port minimum viable job service (`internal/jobs` clone) backed by etcd key/value stores.
2. **Beacon node**
   - Adapt `gridbeacon` service into a Ploy-focused binary (etcd store, no Cloudflare).
   - Expose discovery + DNS endpoints documented in `docs/v2/api.md`.

## Phase 2 — Job Metadata & Artifacts

1. **Job outcome schema**
   - Implement etcd storage layout `mods/<ticket>/jobs/<job-id>` (status, node, timestamps).
   - Add artifact reference fields (diff CID, build gate report CID, log digest).
2. **Workspace hydration & diff publishing**
   - Teach `ploynode` to hydrate containers with the original repo plus cumulative diffs before each
     job launches (shared volume mount).
   - After completion, compute diff bundles on the node, upload them through the IPFS Cluster client
     (replacing JetStream attachments), and record CIDs in etcd.
   - Add build gate report publisher (structured JSON) with CID references recorded alongside job
     outcomes.
3. **SSE log streaming**
   - Implement `/v2/jobs/{id}/logs/stream` and `/node/v2/jobs/{id}/logs/stream`.

## Phase 3 — CLI & API Surface

1. **Command updates**
   - `ploy mod` subcommands with `--mod-env` and log streaming.
   - `ploy node` namespace (add/remove/list/heal/logs).
   - `ploy deploy bootstrap`, `ploy cluster connect`, `ploy cluster list`.
2. **API handlers**
   - Control plane endpoints (`/v2/mods`, `/v2/jobs`, `/v2/nodes`, `/v2/config`).
   - Node endpoints (`/node/v2/jobs`, `/node/v2/status`, `/node/v2/artifacts`).

## Phase 4 — Mods & Build Gate Integration

1. **Workflow runner**
   - Replace Grid client wiring with the new runtime adapter.
   - Ensure stage execution stores job metadata + artifact CIDs after every step.
2. **SHIFT integration**
   - Keep existing build gate packages but adapt output to IPFS & job metadata.
   - Update failure handling to emit JSON report CIDs.

## Phase 5 — Deployment & Ops Tooling

1. **Bootstrap script**
   - Finalise embedded shell script used by `ploy deploy bootstrap` and `ploy node add`.
   - Handle CA generation, DNS stub installation, cluster descriptor caching (with version tag).
2. **Ops commands**
   - Implement `ploy beacon rotate-ca`, `ploy beacon sync`, `ploy node logs`.
   - Surface `ploy status` using the new endpoints.

## Phase 6 — Cleanup & Validation

1. **Remove legacy code paths**
   - Delete Grid RPC clients, JetStream helpers, and unused env vars once v2 paths are green.
2. **Testing**
   - Add integration tests covering job submission, diff publishing, build gate failure loops, CLI end-to-end flows.
   - Update CI to run only the new pipelines.
3. **Documentation pass**
   - Verify `docs/v2` remains accurate, and update root README when v2 becomes default.

## Tracking & Delivery

- Use a milestone (e.g., `v2-migration`) in issue tracker to group tasks.
- Prioritise phases sequentially; avoid partial implementations that keep Grid dependencies alive.
- Coordinate with release/ops teams to schedule cutover once phases 1–5 are complete.
