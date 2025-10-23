# Ploy v2 API Reference

This document catalogs the service endpoints introduced in Ploy v2. Routes are grouped by surface:
Ploy control-plane APIs, node-local endpoints, artifact/registry interfaces, and the beacon
exposure. The `ployd` daemon serves both the control-plane and node APIs described below, exposing
them over mutual TLS behind the beacon DNS entry.

## Ploy Control Plane

All routes below are served by the control-plane API (fronted by the beacon DNS), protected by
mutual TLS and token-based authorization. The cluster's `ployd` control-plane deployment owns this
surface.

### Mods

- `POST /v2/mods` — Submit a new Mod run (repository reference, step graph, metadata).
  Returns the ticket ID and initial status.
- `GET /v2/mods/{ticket}` — Retrieve Mod status, step outcomes, artifacts produced, build gate metadata,
  and the node currently executing the Mod.
- `POST /v2/mods/{ticket}/resume` — Resume an interrupted Mod using stored artifacts and logs.
- `POST /v2/mods/{ticket}/cancel` — Request cancellation of an in-flight Mod.
- `GET /v2/mods/{ticket}/logs` — Fetch aggregated logs (per-step and build gate) from archive storage.
- `GET /v2/mods/{ticket}/logs/stream` — SSE stream of combined Mod logs for real-time tailing.

### Artifacts & Snapshotting

- `POST /v2/artifacts/upload` — Stage a repository snapshot or diff bundle; returns the CID published
  to IPFS Cluster.
- `GET /v2/artifacts/{cid}` — Fetch artifact metadata, download URL, and pin status.
- `DELETE /v2/artifacts/{cid}` — Request unpin/garbage collection of an artifact (subject to
  retention policy).
- `GET /v2/artifacts` — List artifacts with optional filters (`repo`, `type`, `age`).

The upload endpoint is primarily for workstation CLI interactions; runtime nodes typically stream
artifacts directly to IPFS Cluster via their local daemon and only record the resulting CIDs in etcd.

### OCI Registry

- `PUT /v2/registry/{repo}/manifests/{reference}` — Store an OCI manifest (schema 2).
  Publishes pin events to IPFS Cluster.
- `GET /v2/registry/{repo}/manifests/{reference}` — Retrieve a manifest and associated metadata.
- `DELETE /v2/registry/{repo}/manifests/{reference}` — Remove a manifest and release pins.
- `POST /v2/registry/{repo}/blobs/uploads/` — Start a blob upload session (standard Docker Registry v2 semantics).
- `PATCH /v2/registry/{repo}/blobs/uploads/{uuid}` — Append chunk data to an upload session.
- `PUT /v2/registry/{repo}/blobs/uploads/{uuid}?digest=sha256:...` — Complete an upload, committing
  the blob and pinning it.
- `GET /v2/registry/{repo}/blobs/{digest}` — Stream a blob.
- `DELETE /v2/registry/{repo}/blobs/{digest}` — Remove a blob (subject to reference tracking).
- `GET /v2/registry/{repo}/tags/list` — List tags for a repository.

### Node Management & Observability

- `POST /v2/nodes` — Register a node, returning credentials, CA bundles, and initial workload assignments.
- `GET /v2/nodes` — List nodes with health, capabilities, and workload counters.
- `GET /v2/nodes/{node}` — Inspect a specific node’s status, IPFS pin queue depth, running jobs, and cluster version tag.
- `DELETE /v2/nodes/{node}` — Deregister a node after drain acknowledgement.
- `POST /v2/nodes/{node}/heal` — Trigger automated remediation routines (restart services, resync pins).
- `POST /v2/nodes/{node}/promote` — Promote a node to beacon role.
- `GET /v2/nodes/{node}/logs` — Fetch historical daemon logs.
- `GET /v2/nodes/{node}/logs/stream` — SSE stream for real-time node logs.

### Jobs

- `GET /v2/jobs/{id}` — Retrieve job status, container metadata, exit code, resource usage, executing node ID,
  and artifact references (diff/build-gate CIDs).
- `GET /v2/jobs/{id}/logs` — Fetch archived stdout/stderr output.
- `GET /v2/jobs/{id}/logs/stream` — SSE log stream for live job output.
- `GET /v2/jobs/{id}/events` — List lifecycle events (submitted, started, completed, failed, retried).

Jobs cover both Mod steps and build gate executions; callers can use the `type` field in responses to
differentiate them.

### Configuration & Cluster Metadata

- `GET /v2/config` — Retrieve effective cluster configuration (IPFS endpoints, node selection policies,
  feature flags, cluster version tag).
- `PUT /v2/config` — Update configuration values (requires admin scope).
- `GET /v2/status` — Cluster summary: node health, Mods throughput, build gate pass rate.
- `GET /v2/logs/{component}` — Fetch aggregated logs for control-plane components (scheduler, artifact service).
- `GET /v2/version` — Return the current cluster version tag used for drift detection and CLI caching.

## Node API

Each worker node exposes a subset of APIs (mutual TLS, restricted to control-plane callers)
through its local `ployd` instance:

- `POST /node/v2/jobs` — Accept a step execution request (OCI image, command, environment).
- `GET /node/v2/jobs/{id}` — Inspect job state (queued, running, succeeded, failed) with timestamps and exit codes.
- `POST /node/v2/jobs/{id}/cancel` — Stop a running job (if local).
- `GET /node/v2/jobs/{id}/logs` — Fetch archived stdout/stderr.
- `GET /node/v2/jobs/{id}/logs/stream` — SSE log stream direct from the node runtime.
- `GET /node/v2/status` — Health summary: Docker status, SHIFT availability, IPFS connectivity, resource usage.
- `GET /node/v2/artifacts/{cid}` — Provide local access to a pinned artifact (used during step execution).

## Beacon API

The beacon node (running in beacon mode) exposes DNS-compatible discovery plus a small HTTPS API:

- `GET /v2/beacon/nodes` — Signed list of healthy nodes, advertised addresses, and capabilities.
- `GET /v2/beacon/ca` — Current cluster CA bundle for clients.
- `POST /v2/beacon/rotate-ca` — Initiate CA rotation (admin only); returns new bundle metadata.
- `GET /v2/beacon/config` — Discovery endpoints (etcd cluster addresses, IPFS Cluster peers,
  registry endpoints).
- `POST /v2/beacon/promote` — Record the canonical beacon node (used during failover).

## Authentication & Security

- All APIs require mutual TLS certificates issued by the cluster CA.
- Control-plane routes additionally accept bearer tokens (JWT) minted during node registration or CLI login.
- Beacon responses are signed with the beacon key so clients can verify node lists and configuration payloads.

## Eventing & Streaming

- Job and Mod updates stream over server-sent events (`GET /v2/mods/{ticket}/events`) for CLI consumption.
- Artifact pin/unpin notifications publish to etcd watch paths (`/ploy/pins/**`), replacing the
  JetStream event bus.

This layout keeps Ploy APIs modular while mirroring the familiar registry and Mods flows, allowing
gradual upgrades from Grid-based deployments.
