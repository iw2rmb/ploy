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
Artifacts routes require the `artifact.read` and/or `artifact.write` scopes.

#### Listing artifacts

`GET /v1/artifacts` accepts `job_id`, `stage`, `kind`, `cid`, `limit` (default 50, max 200), and
`cursor` query parameters. The `stage` filter is only valid when a `job_id` is supplied. Responses are
cache-busted (`Cache-Control: no-store`) and include an opaque `next_cursor` when additional pages are
available:

```json
{
  "artifacts": [
    {
      "id": "artifact-lwt8z8b4m6kqv1vr",
      "job_id": "mod-1234-plan",
      "stage": "plan",
      "kind": "diff",
      "node_id": "node-a",
      "cid": "bafybeihdwd...",
      "digest": "sha256:d9043d...",
      "size": 7340032,
      "name": "plan-diff.tar.gz",
      "source": "ssh-slot",
      "replication_factor_min": 2,
      "replication_factor_max": 3,
      "pin_state": "pinning",
      "pin_replicas": 1,
      "pin_retry_count": 0,
      "created_at": "2025-10-26T12:07:08.123456Z",
      "updated_at": "2025-10-26T12:07:08.123456Z"
    }
  ]
}
```

#### Uploading via HTTP

`POST /v1/artifacts/upload` expects the artifact payload in the request body with metadata supplied via
query parameters:

- `job_id` (required) associates the artifact with a Mod/job record.
- `stage`, `kind`, and `node_id` (optional) label the artifact.
- `name` overrides the stored filename (defaults to `artifact-<job_id>`).
- `digest` enforces an expected `sha256:` hash before the payload is accepted.
- `replication_min` / `replication_max` override IPFS Cluster pin targets.
- `ttl` hints at retention windows understood by the GC controllers.

Example request:

```bash
curl -X POST "https://cp.example.com/v1/artifacts/upload?job_id=mod-1234-plan&stage=plan&kind=diff&name=plan-diff.tar.gz" \
  -H "Authorization: Bearer <token>" \
  --data-binary @plan-diff.tar.gz
```

Successful uploads return `201 Created` and the stored metadata:

```json
{
  "artifact": {
    "id": "artifact-4i4lwtwaz0z9qhgm",
    "job_id": "mod-1234-plan",
    "stage": "plan",
    "kind": "diff",
    "cid": "bafybeigd4...",
    "digest": "sha256:2ee6e0...",
    "size": 7340032,
    "name": "plan-diff.tar.gz",
    "source": "http-upload",
    "created_at": "2025-10-26T12:09:11.000000Z",
    "updated_at": "2025-10-26T12:09:11.000000Z"
  }
}
```

#### Inspecting, downloading, or deleting artifacts

`GET /v1/artifacts/{id}` returns the metadata above. Appending `?download=true` streams the binary
payload with `Content-Type: application/octet-stream` and preserves the stored `size` header when
available. `DELETE /v1/artifacts/{id}` unpins the artifact (subject to retention policy) and returns
the final metadata snapshot so operators can audit who removed it.

Pin health surfaces through `pin_state`, `pin_replicas`, `pin_retry_count`, `pin_error`, and
`pin_next_attempt_at`. These fields are also emitted by `ploy artifact status` so workstation runs can
confirm whether IPFS Cluster finished replicating the upload.

### Transfer Slots (SSH uploads & downloads)

Bulk transfers ride over the persistent SSH tunnels managed by the CLI. Slots are short-lived
reservations that define which node, directory, and size budget a transfer may target. The control
plane exposes three endpoints guarded by the `artifact.read`/`artifact.write` scopes:

- `POST /v1/transfers/upload` — Reserve an upload slot for a job (`kind` defaults to `repo`).
- `POST /v1/transfers/download` — Reserve a download slot bound to an existing artifact ID or kind.
- `POST /v1/transfers/{slot}/commit` / `.../abort` — Finalise or release the slot once the SSH copy
  completes.

Each slot response includes the node that should be contacted over SSH, the absolute `remote_path`
under `/var/lib/ploy/ssh-artifacts/slots/<slot>/payload`, a `max_size` ceiling (default 10 GiB), and a
30‑minute `expires_at` deadline. The CLI uses those values when running `ploy upload` and `ploy report`:

```json
{
  "slot_id": "slot-y7bn0oec8k4q",
  "kind": "repo",
  "job_id": "mod-1234-plan",
  "node_id": "cp-1",
  "remote_path": "/var/lib/ploy/ssh-artifacts/slots/slot-y7bn0oec8k4q/payload",
  "max_size": 10737418240,
  "expires_at": "2025-10-26T12:40:02.000000Z"
}
```

Upload clients stream data to `remote_path` via the existing ControlMaster socket; on success they call
`POST /v1/transfers/{slot}/commit` with the final `size` and `sha256:` digest. Downloads mirror the
flow: the control plane maps the requested artifact to the node that produced it, the CLI uses `CopyFrom`
to pull the staged file, and a commit call confirms the checksum before recording an access log.

The control plane keeps slot state in-memory (`pending`, `committed`, `aborted`). Aborts can be issued
explicitly via `POST /v1/transfers/{slot}/abort` (for example, when the SSH session fails) and the CLI
invokes it automatically before surfacing errors. Operators can delete orphaned slot directories safely
after the TTL if a node restarts mid-transfer.

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

## Node API

### Jobs

- `GET /v1/node/jobs` — List recent jobs executed on this node. Newest first.
- `GET /v1/node/jobs/{id}` — Inspect a single job record.
- `GET /v1/node/jobs/{id}/logs/stream` — Server‑sent events stream of live logs for the job.

Example list response:

```json
[
  {
    "id": "job-xyz",
    "state": "running",
    "started_at": "2025-10-29T20:20:00Z",
    "completed_at": "",
    "log_stream": "job-xyz"
  }
]
```

Example detail response:

```json
{
  "id": "job-abc",
  "state": "failed",
  "started_at": "2025-10-29T20:18:00Z",
  "completed_at": "2025-10-29T20:19:01Z",
  "log_stream": "job-abc",
  "error": "exit status 1"
}
```

Registry endpoints enforce `registry.pull` for read paths and `registry.push` for write/delete
operations.

#### Blob uploads

`POST /v1/registry/<repo>/blobs/uploads` allocates an upload slot backed by the same SSH transfer
manager. The JSON payload may include `node_id`, `size`, and an optional `media_type`; the response
returns `upload_id`, `slot_id`, `remote_path`, and a `Location` header pointing at
`/v1/registry/<repo>/blobs/uploads/<upload_id>`.

Registry writers then:

1. Copy the blob to `remote_path` over SSH (typically from the same host running the CLI or build).
2. Optionally `PATCH /v1/registry/<repo>/blobs/uploads/<upload_id>` with `{"size": <bytes_sent>}` to
   record progress counters.
3. `PUT /v1/registry/<repo>/blobs/uploads/<upload_id>?digest=sha256:...` with a JSON body describing the
   `media_type` and `size`. The control plane verifies the digest, publishes the blob to IPFS Cluster,
   updates the registry store, deletes the slot, and responds with:

```json
{
  "digest": "sha256:8843d7f92416211de9ebb963ff4ce281",
  "cid": "bafybeiemblobs...",
  "location": "/v1/registry/acme/web/blobs/sha256:8843d7..."
}
```

`Docker-Content-Digest` mirrors the stored digest and allows Docker/OCI clients to validate the commit.

`GET /v1/registry/<repo>/blobs/<digest>` streams the blob (`Content-Type` defaults to
`application/octet-stream` when unspecified). `DELETE` marks the blob as deleted and returns
`{"digest": "sha256:...", "state": "deleted"}` with `202 Accepted` once persistence succeeds.

#### Manifests and tags

`GET /v1/registry/<repo>/manifests/<reference>` resolves either a digest or tag, returning the raw
manifest payload and setting `Docker-Content-Digest`. `PUT` accepts a full OCI manifest document; the
control plane ensures every referenced blob already exists and optionally associates the provided tag.
Successful writes respond with `201 Created`, a `Location` header for the canonical digest, and a JSON
body containing `{ "digest": "sha256:..." }`.

`DELETE /v1/registry/<repo>/manifests/<digest>` removes the immutable record, while deleting a tag via
`DELETE /v1/registry/<repo>/manifests/<tag>` detaches only that mutable pointer.

Tags can also be listed through `GET /v1/registry/<repo>/tags/list`, which returns:

```json
{
  "name": "acme/web",
  "tags": ["latest", "v1.2.3"]
}
```

All registry routes emit metrics (`ploy_registry_http_requests_total` and
`ploy_registry_payload_bytes_total`) so operators can alert on unexpected 4xx/5xx spikes or large blob
flows.

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
  timestamps (`updated_at`, `updated_by`), and an optional `discovery` block that advertises the
  control-plane descriptors (`default_descriptor` plus a list of `{cluster_id,address,api_endpoint,ca_bundle}` entries).
  The response is returned with `Cache-Control: no-store` and an `ETag` header whose value matches the etcd revision so callers can cache locally.
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
- `GET /v1/node/status` — Returns the latest lifecycle snapshot published by the worker:
  - `state` aggregates component health (`ok`, `degraded`, `error`, `unknown`) across Docker, SHIFT, and IPFS probes.
  - `resources.cpu|memory|disk` now include host totals/free plus nested disk I/O metrics: `resources.disk.io.read_mb_per_sec`, `write_mb_per_sec`, `read_iops`, and `write_iops`, with `details.initial_sample=true` when the first sample lacks a baseline.
  - `resources.network` tracks aggregate `rx_bytes_per_sec`, `tx_bytes_per_sec`, `rx_packets_per_sec`, `tx_packets_per_sec`, and an `interfaces` map keyed by device (`eth0`, `bond0`, etc.) so operators can spot per-NIC saturation. Interfaces listed in `PLOY_LIFECYCLE_NET_IGNORE` (glob support) are omitted, and the section exposes `details.initial_sample=true` until the second sample lands.
  - `components.docker|shift|ipfs` carry `state`, `message`, `version`, and probe timestamps so the control plane can surface detailed diagnostics.
  - `heartbeat` mirrors the timestamp written to etcd (`nodes/<node-id>/capacity`), letting the scheduler correlate status and capacity updates.
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
