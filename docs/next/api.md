# Ploy Next API Reference

This document catalogs the service endpoints introduced in Ploy Next. Routes are grouped by surface:
Ploy control-plane APIs, node-local endpoints, and artifact/registry interfaces. The `ployd` daemon
serves both the control-plane and node APIs described below, and the CLI reaches them by tunnelling
HTTP over SSH using the cached cluster descriptors (`pkg/sshtransport`). No separate beacon or CA
distribution surface remains.

## Ploy Control Plane

All routes below are served by the control-plane API and are typically accessed through the SSH
tunnels managed by the CLI. Token-based authorization still applies, but the SSH transport handles
end-to-end encryption instead of a standalone beacon TLS proxy.

### Mods

- `POST /v1/mods` — Submit a new Mod run (repository reference, step graph, metadata).
  Returns the ticket ID and initial status.
- `GET /v1/mods/{ticket}` — Retrieve Mod status, step outcomes, artifacts produced, build gate metadata,
  and the node currently executing the Mod.
- `POST /v1/mods/{ticket}/resume` — Resume an interrupted Mod using stored artifacts and logs.
- `POST /v1/mods/{ticket}/cancel` — Request cancellation of an in-flight Mod.
- `GET /v1/mods/{ticket}/logs` — Fetch aggregated logs (per-step and build gate) from archive storage.
- `GET /v1/mods/{ticket}/logs/stream` — SSE stream of combined Mod logs for real-time tailing.

Example `GET /v1/mods/{ticket}` response:

```json
{
  "ticket": {
    "ticket_id": "mod-1234",
    "state": "running",
    "submitter": "ci-bot",
    "repository": "git@gitlab.com:ploy/example.git",
    "metadata": {
      "sha": "3b28cf5"
    },
    "created_at": "2025-10-24T10:14:09Z",
    "updated_at": "2025-10-24T10:15:42Z",
    "stages": {
      "plan": {
        "stage_id": "plan",
        "state": "queued",
        "attempts": 1,
        "max_attempts": 3,
        "current_job_id": "job-9h2d5",
        "artifacts": {},
        "last_error": ""
      }
    }
  }
}
```

`GET /v1/mods/{ticket}/logs` returns the buffered SSE history (log, retention, done frames) as JSON:

```json
{
  "events": [
    {
      "id": 1,
      "type": "log",
      "data": {
        "timestamp": "2025-10-24T10:15:10Z",
        "stream": "stdout",
        "line": "starting plan stage"
      }
    }
  ]
}
```

### Artifacts & Snapshotting

- `POST /v1/artifacts/upload` — Stage a repository snapshot or diff bundle; returns the CID published
  to IPFS Cluster.
- `GET /v1/artifacts/{cid}` — Fetch artifact metadata, download URL, and pin status.
- `DELETE /v1/artifacts/{cid}` — Request unpin/garbage collection of an artifact (subject to
  retention policy).
- `GET /v1/artifacts` — List artifacts with optional filters (`repo`, `type`, `age`).

The upload endpoint is primarily for workstation CLI interactions; runtime nodes typically stream
artifacts directly to IPFS Cluster via their local daemon and only record the resulting CIDs in etcd.
Artifacts routes require the `artifact.read` and/or `artifact.write` scopes. Until roadmap item 1.4
lands persistence, uploads and deletions return a structured `501 Not Implemented` payload with
`error_code` hints, and listings surface an empty collection.

### OCI Registry

- `PUT /v1/registry/{repo}/manifests/{reference}` — Store an OCI manifest (schema 2).
  Publishes pin events to IPFS Cluster.
- `GET /v1/registry/{repo}/manifests/{reference}` — Retrieve a manifest and associated metadata.
- `DELETE /v1/registry/{repo}/manifests/{reference}` — Remove a manifest and release pins.
- `POST /v1/registry/{repo}/blobs/uploads/` — Start a blob upload session (standard Docker Registry v2 semantics).
- `PATCH /v1/registry/{repo}/blobs/uploads/{uuid}` — Append chunk data to an upload session.
- `PUT /v1/registry/{repo}/blobs/uploads/{uuid}?digest=sha256:...` — Complete an upload, committing
  the blob and pinning it.
- `GET /v1/registry/{repo}/blobs/{digest}` — Stream a blob.
- `DELETE /v1/registry/{repo}/blobs/{digest}` — Remove a blob (subject to reference tracking).
- `GET /v1/registry/{repo}/tags/list` — List tags for a repository.

Registry endpoints enforce `registry.pull` for read paths and `registry.push` for write/delete
operations. While the backing registry store is still underway, all write paths and blob/manifest
reads return `501 Not Implemented` responses carrying `error_code` markers so the CLI can gate on the
contract.

### Node Management & Observability

- `POST /v1/nodes` — Register a node, returning the join metadata and any initial workload assignments.
- `GET /v1/nodes` — List nodes with health, capabilities, and workload counters.
- `GET /v1/nodes/{node}` — Inspect a specific node’s status, IPFS pin queue depth, running jobs, and cluster version tag.
- `DELETE /v1/nodes/{node}` — Deregister a node after drain acknowledgement.
- `POST /v1/nodes/{node}/heal` — Trigger automated remediation routines (restart services, resync pins).
- `GET /v1/nodes/{node}/logs` — Fetch historical daemon logs.
- `GET /v1/nodes/{node}/logs/stream` — SSE stream for real-time node logs.

### Jobs

- `GET /v1/jobs/{id}` — Retrieve job status, container metadata, exit code, resource usage, executing node ID,
  and artifact references (diff/build-gate CIDs).
- `GET /v1/jobs/{id}/logs` — Fetch archived stdout/stderr output.
- `GET /v1/jobs/{id}/logs/stream` — SSE log stream for live job output.
- `GET /v1/jobs/{id}/events` — List lifecycle events (submitted, started, completed, failed, retried).

Jobs cover both Mod steps and build gate executions; callers can use the `type` field in responses to
differentiate them.

### Configuration & Cluster Metadata

- `GET /v1/config` — Retrieve effective cluster configuration (IPFS endpoints, node selection policies,
  feature flags, cluster version tag). Responses include the current `revision`, `version_tag`,
  and timestamps (`updated_at`, `updated_by`). The response is returned with `Cache-Control: no-store`
  and an `ETag` header whose value matches the etcd revision so callers can cache locally.
- `PUT /v1/config` — Update configuration values (requires `admin` scope). Callers must supply an
  `If-Match` header: use `0` to create the document, the last seen revision to update in place, or `*`
  to force an unconditional write. On success the control plane returns the sanitized document,
  refreshes the revision/ETag, and increments `ploy_config_updates_total`.
- `GET /v1/status` — Cluster summary covering queue depth and worker readiness. The payload includes:

  ```json
  {
    "cluster_id": "cluster-alpha",
    "timestamp": "2025-10-24T17:42:13.123456Z",
    "queue": {
      "total_depth": 3,
      "priorities": [
        {"priority": "default", "depth": 2},
        {"priority": "urgent", "depth": 1}
      ]
    },
    "workers": {
      "total": 5,
      "phases": {"ready": 4, "registering": 1, "error": 0, "unknown": 0}
    }
  }
  ```

  The endpoint always returns `Cache-Control: no-store` so operators poll without stale data.
- `GET /v1/logs/{component}` — Fetch aggregated logs for control-plane components (scheduler, artifact service).
- `GET /v1/version` — Return the current build metadata used for drift detection and CLI caching.
  Responses contain the semantic version, git commit, and build timestamp and are cacheable for one
  minute (`Cache-Control: public, max-age=60`).

## Node API

Each worker node exposes a subset of APIs (mutual TLS, restricted to control-plane callers)
through its local `ployd` instance:

- `POST /v1/node/jobs` — Accept a step execution request (OCI image, command, environment).
- `GET /v1/node/jobs/{id}` — Inspect job state (queued, running, succeeded, failed) with timestamps and exit codes.
- `POST /v1/node/jobs/{id}/cancel` — Stop a running job (if local).
- `GET /v1/node/jobs/{id}/logs` — Fetch archived stdout/stderr.
- `GET /v1/node/jobs/{id}/logs/stream` — SSE log stream direct from the node runtime.
- `GET /v1/node/status` — Health summary: Docker status, SHIFT availability, IPFS connectivity, resource usage.
- `GET /v1/node/artifacts/{cid}` — Provide local access to a pinned artifact (used during step execution).

## Authentication & Security

- All APIs are reached through SSH tunnels stood up by `pkg/sshtransport`. Within the tunnel the
  control plane still enforces mutual TLS using the cluster-internal PKI.
- Control-plane routes additionally accept bearer tokens minted by the GitLab signer. Tokens embed the issuing secret identifier (`sid`) and token id (`tid`) so the control plane can validate them without scanning every secret.
- Administrative operations (GitLab signer management, configuration updates) require bearer tokens that include the `admin` scope.

## Eventing & Streaming

- Job and Mod updates stream over server-sent events (`GET /v1/mods/{ticket}/events`) for CLI consumption.
- Artifact pin/unpin notifications publish to etcd watch paths (`/ploy/pins/**`), replacing the
  JetStream event bus.

Mod event streams emit:

```text
event: ticket
data: {"ticket_id":"mod-1234","state":"running",...}

event: stage
data: {"ticket_id":"mod-1234","stage":{"stage_id":"plan","state":"queued","attempts":1,...}}
```

Job event streams (`GET /v1/jobs/{id}/events`) emit the canonical job DTO on every update:

```text
event: job
data: {"id":"job-9h2d5","ticket":"mod-1234","state":"succeeded",...}
```

This layout keeps Ploy APIs modular while mirroring the familiar registry and Mods flows, allowing
gradual upgrades from Grid-based deployments.
