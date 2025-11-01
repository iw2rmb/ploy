# Ploy — Mods Orchestrator and Control Plane

Ploy is a workstation‑first orchestration stack for code‑mod (Mods) workflows. It consists of:

- `ploy` — a CLI for submitting Mods, following logs, managing artifacts, and administering nodes.
- `ployd` — a daemon that runs both the HTTPS control‑plane APIs and the worker execution loop.

The v2 architecture removes external orchestrators and build‑gate dependencies. Ploy embeds its own
scheduler and step runtime, integrates a language‑agnostic Build Gate, and persists artifacts in
IPFS Cluster.

See docs/next/README.md for the broader “Ploy Next” overview.

For a proposed simplified architecture that replaces etcd with PostgreSQL and removes IPFS in favor of ephemeral repo clones, see `SIMPLE.md`.

**What Changed (2025‑10)**
- Not CLI‑only anymore: the repository ships a control plane (`cmd/ployd`) alongside the CLI.
- Grid orchestrator removed: scheduling and queueing live in `internal/controlplane/scheduler`.
- SHIFT build gate removed: the integrated Build Gate lives under `internal/workflow/buildgate`.

**Core Components**
- CLI entrypoint: `cmd/ploy` (top‑level commands: `mod`, `mods`, `jobs`, `artifact`, `cluster`, `manifest`, `environment`, `knowledge-base`).
- Daemon: `cmd/ployd` with defaults in `internal/api/config/*` and wiring in `internal/api/daemon/*`.
- Control‑plane HTTP/SSE: handlers in `internal/api/httpserver/*` and OpenAPI in docs/api/OpenAPI.yaml.
- Scheduler and state: `internal/controlplane/scheduler/*` with etcd integration in `internal/api/daemon/default.go`.
- Worker runtime: `internal/node/worker/step/*` and container adapter in `internal/workflow/runtime/step/*`.
- Build Gate: `internal/workflow/buildgate/*` (sandbox runner, static checks, Java executor, log ingestion).
- Artifacts and transfers: control‑plane store/reconciler in `internal/controlplane/artifacts/*`, workflow publishers in `internal/workflow/artifacts/*`, SSH transfer guard in `internal/controlplane/transfers/*`.
- Control-plane connectivity: CLI uses direct HTTPS (mTLS) to reach the control plane; SSH tunnels have been removed.

**Docs You’ll Want**
- Architecture and concepts: docs/next/README.md
- CLI reference: docs/next/cli.md
- Control‑plane APIs: docs/next/api.md and docs/api/OpenAPI.yaml
- Job and Mods model: docs/next/job.md and docs/next/mod.md
- Artifact handling: docs/next/ipfs.md and docs/workflow/README.md
- Deploy/operate a cluster: docs/how-to/deploy-a-cluster.md and docs/next/observability.md

**Build**
- Requirements: Go 1.25+, Docker 28.x for local step execution.
- Build binaries into `dist/`:
  
  ```bash
  make build
  ```

This produces `dist/ploy` and `dist/ployd` (plus a Linux `ployd` for remote installs).

**Quick Start**
- Bootstrap a first control‑plane node over SSH (see the how‑to for prerequisites):

  ```bash
  dist/ploy cluster add --address <host-or-ip>
  ```

- Add more workers to the same cluster:

  ```bash
  dist/ploy cluster add --address <host-or-ip> --cluster-id <cluster-id>
  ```

- Submit a Mods run and follow events:

  ```bash
  dist/ploy mod run --repo-url https://example.com/repo.git \
    --repo-base-ref main --repo-target-ref mods-upgrade-java17 \
    --follow --artifact-dir /tmp/mods-artifacts
  ```

- Follow an individual job’s logs:

  ```bash
  dist/ploy jobs follow <job-id>
  ```

- Plan an integration environment (no cache hydration in dry‑run):

  ```bash
  dist/ploy environment materialize <commit-sha> --app commit-app --dry-run
  ```

**Environment Variables**
- Full reference: docs/envs/README.md
- Common examples:
  - `PLOY_CONTROL_PLANE_URL` — override when no cached cluster descriptor exists.
  - `PLOY_IPFS_CLUSTER_API` — required on worker nodes for artifact publishing.
  - `PLOY_BUILDGATE_JAVA_IMAGE` — optional Java image for the Build Gate.

**Contributing**
- Follow AGENTS.md (RED→GREEN→REFACTOR cadence; `make test` runs `go test -cover ./...`).
- Keep docs in sync; prefer docs/next/* and remove or update stale references in the same PR.

**Notes on Removed/Legacy Items**
- References to `configs/lanes/*`, legacy “lanes describe”, or docs/design/* are obsolete and have been removed here.
- External GRID/SHIFT integrations are no longer part of this codebase.

License: see `LICENSE` when present.
