# `ployd` Daemon Design

## Overview

We need a long-running node agent that unifies bootstrap, worker runtime, and node management APIs
into a single executable (`ployd`). Today we have scattered scripts and libraries under
`internal/deploy` and `internal/node` that rely on ad-hoc SSH execution, etcd manipulation, and
bespoke HTTP handlers. `ployd` will encapsulate those responsibilities, exposing well-defined
interfaces for the control plane and CLI while orchestrating local services on the node.

## Goals

- Provide a daemonized process (systemd service) installed during bootstrap that keeps the node in
  sync with the control plane.
- Serve the documented node APIs (e.g., `/node/v2/...`) using the forthcoming Fiber-based HTTP
  server, handling log streaming, health checks, artifact access, and command execution.
- Manage certificate renewal, worker registration, and metadata updates without requiring the
  workstation CLI to reach etcd directly.
- Coordinate bootstrap tasks (install dependencies, configure Docker/IPFS, prepare PKI materials)
  through internal modules exposed via a minimal gRPC/HTTP control surface.
- Support both beacon (bootstrap) mode and worker mode via configuration flags.

## Non-goals

- Replacing the control-plane scheduler or Mod execution runtime; `ployd` focuses on node-side
  orchestration.
- Overhauling the CLI; instead, the CLI will communicate with `ployd` (locally during bootstrap or
  remotely via control-plane APIs).
- Cross-node coordination beyond reporting state and fetching assignments.

## Architecture

### Process Model

- `ployd` runs as root (or privileged user) under systemd with templated units
  (e.g., `/etc/systemd/system/ployd.service`).
- On startup it loads configuration from `/etc/ploy/ployd.yaml` (generated during bootstrap) and the
  cluster descriptor cached by the CLI.
- Components:
  - **HTTP API** (Fiber) on configurable port for `/node/v2` endpoints, log streaming, health checks.
  - **Control Plane Client** maintaining a persistent connection (mTLS) to beacon/control plane,
    polling assignments and pushing status.
  - **PKI Manager** watching certificate expiry, requesting renewals, installing bundles into
    `/etc/ploy/pki`.
  - **Bootstrap Runner** (optional) executing the host preparation script and reporting progress back
    to the CLI during initial install.
  - **Task Scheduler** for background jobs (dependency checks, disk pruning, artifact cleanup).

### Configuration & Modes

- `ployd --mode=bootstrap` runs the initial install flow, prepares the host, writes configuration, and
  transitions to `--mode=worker` once complete.
- `ployd --mode=worker` focuses on steady-state orchestration: joining the cluster, maintaining
  heartbeat, executing tasks, serving APIs.
- Beacon mode (`ployd --mode=beacon`) extends worker mode with beacon-specific tasks (DNS records,
  control-plane proxy, CA publication).

### Communication

- Upstream: gRPC/HTTP client to control plane using cluster CA + node certificate.
  - Poll `/v2/assignments`, `/v2/config`, `/v2/nodes/{id}` for updates.
  - Publish health/status via `/v2/nodes/{id}` PATCH.
- Local CLI/Operators: expose Unix socket or localhost port for administrative commands (log
  retrieval, diagnostics, manual maintenance actions).
- Workload Execution: integrate with existing runtime adapters (`internal/workflow/runtime`) through
  well-defined interfaces.

### Data Flow

1. Bootstrap CLI deploys binaries and systemd unit.
2. `ployd --mode=bootstrap` runs, installs dependencies, issues worker certificate via control-plane
   API, stores descriptors.
3. After bootstrap success, service reconfigures itself to worker/beacon mode and restarts.
4. Worker mode loops: fetch tasks → execute (e.g., Mods) → stream logs → report status.
5. Control plane uses `/v2/nodes` to manage registrations; CLI no longer touches etcd.

## Security

- Mutual TLS for all outbound control-plane communication using node certificate with short TTLs.
- Local APIs protected via Unix socket permissions or mTLS for remote management.
- Secrets (API tokens, private keys) stored under `/etc/ploy/pki` with 0600 permissions; rotate via
  existing PKI flows.

## Deployment Plan

1. **Scaffold** `ployd` binary under `cmd/ployd/main.go`, wiring existing modules (logstream hub,
   executor, bootstrap runner).
2. **Systemd Packaging**: add unit files to deployment assets (`internal/deploy/assets/bootstrap.sh`).
3. **API Layer**: reuse the Fiber migration work to expose `/node/v2/*` routes inside `ployd`.
4. **Control Plane Integration**: implement node heartbeat/assignment polling using the upcoming
   control-plane APIs.
5. **CLI Adjustments**: bootstrap installs `ployd` and coordinates via local admin API instead of
   raw SSH commands. `ploy node add` becomes a remote call to `/v2/nodes` handled by `ployd` on the
   target.
6. **Rolling Adoption**: start with lab nodes, validate mod execution + log streaming, expand to
   production clusters.

## Open Questions

- How to structure the local admin API to avoid privilege escalation (policy? RBAC?).

## Decisions

- **Configuration reload**: `ployd` must support graceful config reload on `SIGHUP`, in addition to
  the ability to restart via systemd. The daemon monitors config files and reinitializes components
  (HTTP server, control-plane client) without dropping active workloads whenever possible.
- **Runtime plugins**: `ployd` must provide a plugin interface for additional execution backends
  (Nomad, Kubernetes, custom runtimes). The initial implementation will load adapters via Go modules
  behind feature flags, with a stable interface for registering new runtimes without rebuilding the
  core daemon.
- **Metrics endpoint**: `ployd` will expose Prometheus metrics on the standard agent port `:9100`
  (configurable), following best practices for node services. TLS support and access control will be
  provided via the same configuration subsystem as the main HTTP API.
