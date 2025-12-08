# Build Gate Contract

Scope
- Minimal, stable contract to validate a repository after each Mods stage.
- Works in Mods and standalone CI.

Status
- HTTP Build Gate mode and the dedicated `buildgate_jobs` table have been removed.
- This document describes the historical HTTP Build Gate design; see ROADMAP.md for the current jobs-based gate execution model.

## Overview: Remote Execution Architecture

Build Gate follows a **repo+diff remote execution model**: callers submit validation
jobs to the control plane, and dedicated Build Gate worker nodes claim, execute, and
report results. This architecture decouples gate validation from Mods execution,
enabling horizontal scaling and separation of concerns.

**Primary execution path:**
1. Mods pre-gate and re-gate (after healing) call the **HTTP Build Gate API**.
2. The `GateExecutor` adapter in `internal/workflow/runtime/step` abstracts the HTTP
   call, presenting a unified interface to the node orchestrator.
3. Build Gate workers claim jobs via `/v1/nodes/{id}/buildgate/claim` and execute
   validation inside Docker containers.
4. Workers report results via `/v1/nodes/{id}/buildgate/{job_id}/complete`; the
   orchestrator receives `BuildGateStageMetadata` through the API response.

**Key benefits:**
- **Horizontal scaling:** Add Build Gate workers independently of Mods nodes.
- **Separation of concerns:** Heavy build validation runs on dedicated nodes.
- **Consistent workspace semantics:** Repo+diff reconstruction ensures reproducible builds.
- **Multi-VPS support:** Gate and Mods can run on different nodes without shared state.

## Remote Execution Flow

The **HTTP Build Gate API** is the canonical execution path for all gate validation.
Callers submit validation jobs via HTTP; Build Gate worker nodes claim and execute them.

**Code paths (historical HTTP mode):**
- Server handlers: `internal/server/handlers/buildgate.go`
- Job storage: unified `jobs` queue in `internal/store/queries/jobs.sql` (the former `buildgate_jobs` table has been removed)
- Gate executor adapter: `internal/workflow/runtime/step/gate_http.go` (HTTP) and
  `internal/workflow/runtime/step/gate_factory.go` (mode selection)

**Endpoints:**
- `POST /v1/buildgate/validate` — Submit a validation job (repo_url + ref + optional diff_patch).
- `GET /v1/buildgate/jobs/{id}` — Poll job status until completed/failed.
- `POST /v1/nodes/{id}/buildgate/claim` — Worker nodes claim pending jobs.
- `POST /v1/nodes/{id}/buildgate/{job_id}/ack` — Acknowledge job start (transition to running).
- `POST /v1/nodes/{id}/buildgate/{job_id}/complete` — Report job completion with result.

**Flow:**
1. Caller (Mods orchestrator or healing flow) submits `BuildGateValidateRequest` with
   `repo_url`, `ref`, and optional `diff_patch` (gzipped unified diff for healing flows).
2. Control plane creates a job row in the unified `jobs` table with status `pending` (historically this used a dedicated `buildgate_jobs` table).
3. If job completes within `syncWaitTimeout` (30s), result returns synchronously.
4. Otherwise, caller receives `job_id` with status `pending` for async polling.
5. Build Gate worker node claims job via `/v1/nodes/{id}/buildgate/claim`.
6. Worker clones repo at ref, applies diff_patch if present, runs build validation
   inside a Docker container.
7. Worker reports completion via `/v1/nodes/{id}/buildgate/{job_id}/complete`.

**Characteristics:**
- Workspace is reconstructed from repo+diff; no local state dependency.
- Gate execution can run on any eligible Build Gate worker node.
- Supports multi-VPS deployments where gate and Mods run on different nodes.
- Job queue enables load distribution across workers.
- Full gate history (pre-gate + all re-gates) is captured in run stats (`gate.pre_gate`,
  `gate.re_gates`, `gate.final_gate`) built from `BuildGateStageMetadata` results.

## GateExecutor Adapter

The `GateExecutor` interface (in `internal/workflow/runtime/step`) abstracts gate
execution for the node orchestrator. The HTTP-backed implementation calls the Build
Gate API and waits for results:

```
┌─────────────────┐      ┌─────────────────────┐      ┌─────────────────────┐
│ Node Orchestr.  │      │ GateExecutor (HTTP) │      │ Control Plane       │
│ (pre-gate/      │─────▶│                     │─────▶│ POST /buildgate/    │
│  re-gate)       │      │                     │      │ validate            │
└─────────────────┘      └─────────────────────┘      └─────────────────────┘
                                                               │
                                                               ▼
                         ┌─────────────────────┐      ┌─────────────────────┐
                         │ Build Gate Worker   │◀─────│ Job Queue           │
                         │ (Docker execution)  │      │ (jobs table)        │
                         └─────────────────────┘      └─────────────────────┘
                                   │
                                   ▼
                         ┌─────────────────────┐
                         │ BuildGateStage      │
                         │ Metadata returned   │
                         └─────────────────────┘
```

This adapter pattern allows:
- Mods orchestration code to remain agnostic of execution location.
- Future execution backends (e.g., Kubernetes jobs) to plug in via the same interface.
- CLI-visible gate summaries (`ploy mod inspect`) to work unchanged.

## Local Docker Gate (Legacy/Fallback)

For local development or single-node deployments without Build Gate workers, the
system can fall back to **local docker gate** execution. This runs build validation
directly on the node agent using a mounted workspace.

**Code path:** `internal/workflow/runtime/step/gate_docker.go`

**Characteristics:**
- Workspace is local to the node; no network transfer of code.
- Gate execution is coupled to the node running the Mods step.
- Build tools have direct access to the working tree.

**Note:** Local docker gate is not the recommended production path. Use Build Gate
workers with the HTTP API for scalable, decoupled gate validation.

See `docs/mods-lifecycle.md` section 1.1 for gate sequence diagrams and healing
flow details.

### Designating Build Gate Worker Nodes

Not all nodes need to execute Build Gate jobs. The `buildgate_worker_enabled`
configuration flag controls whether a node participates in Build Gate job claiming.

**Configuration:**
- YAML config (`/etc/ploy/ployd-node.yaml`):
  ```yaml
  buildgate_worker_enabled: true
  ```
- Environment variable (takes precedence over YAML):
  ```bash
  PLOY_BUILDGATE_WORKER_ENABLED=true
  ```

**Behavior:**
- When `buildgate_worker_enabled=true`: The node claims and executes Build Gate
  jobs via the HTTP Build Gate API endpoints.
- When `buildgate_worker_enabled=false` (default): The node skips Build Gate
  job claiming entirely and only processes regular Mods runs.

**Multi-node deployments:**
In a multi-VPS setup, you can designate specific nodes as Build Gate workers
while others handle only Mods runs. This enables:
- Separation of concerns: heavy build validation on dedicated nodes.
- Resource isolation: Build Gate jobs don't compete with Mods execution.
- Horizontal scaling: add more Build Gate workers as validation load increases.

Example two-node lab:
- Node A: `buildgate_worker_enabled=true` — claims Build Gate jobs.
- Node B: `buildgate_worker_enabled=false` — claims only Mods runs.

Submit a Build Gate job; only Node A logs `claimed buildgate job`.
Submit a Mods run; either node can claim it.

See `docs/envs/README.md` for the complete environment variable reference.

HTTP Build Gate API

The Build Gate uses a repo+diff validation model: callers provide a Git repository URL and ref as baseline,
with an optional unified diff patch for healing flows. This avoids transmitting large workspace archives
over HTTP by leveraging Git for the baseline and sending only the delta.

Endpoint: `POST /v1/buildgate/validate`

Contract: Submit a build validation job using a Git repo+ref baseline, with an optional diff patch for healing flows.

Required fields:
- `repo_url` — Git repository URL to clone (e.g., `https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git`).
- `ref` — Git ref (branch, tag, or commit SHA) to validate (e.g., `e2e/fail-missing-symbol`).

Optional fields:
- `diff_patch` — Gzipped unified diff (base64-encoded) to apply on top of the cloned repo_url+ref baseline. Enables healing mods to replay changes without shipping full workspace archives.
- `profile` — Build profile (`auto`, `java`, `java-maven`, `java-gradle`). Defaults to auto-detection.
- `timeout` — Duration string (e.g., `5m`). Defaults to server-side limit.
- `limit_memory_bytes`, `limit_cpu_millis`, `limit_disk_space` — Resource limits for the validation job.

Example request payload (repo+diff model):

```json
{
  "repo_url": "https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git",
  "ref": "e2e/fail-missing-symbol",
  "profile": "java-maven",
  "timeout": "5m",
  "diff_patch": "<base64(gzip(unified-diff))>"
}
```

Semantics:
1. The executor clones `repo_url` at `ref`.
2. If `diff_patch` is non-empty, it decodes and decompresses the payload, then applies the patch via `git apply`.
3. The build runs against the resulting workspace.

This model avoids transmitting large workspace archives over HTTP, instead leveraging Git for baseline state and transmitting only the delta.

Response:
- On success (200): Returns `BuildGateValidateResponse` with `status` (`completed` or `failed`) and `result` object.
- On accepted (202): Returns `BuildGateValidateResponse` with `job_id` and `status=pending` for async polling via `GET /v1/buildgate/jobs/{id}`.
- On validation error (400): Missing required fields (`repo_url` or `ref`).

Status polling: Use `GET /v1/buildgate/jobs/{job_id}` to poll job status until `completed` or `failed`.

Inputs
- Workspace mount: `/workspace` (required, read–write). Contains a shallow Git checkout at `HEAD`.
- Profile: configurable or auto
  - Set via gate spec `profile` or env `PLOY_BUILDGATE_PROFILE`.
  - Allowed explicit values: `java`, `java-maven`, `java-gradle`.
  - Auto (when unset/unknown): Maven if `/workspace/pom.xml` exists; Gradle if `build.gradle(.kts)` exists; otherwise plain `java`.
- Optional image override: `PLOY_BUILDGATE_IMAGE`. When set, this image is used regardless of profile.
  - Deprecated: `PLOY_BUILDGATE_JAVA_IMAGE`, `PLOY_BUILDGATE_GRADLE_IMAGE` language-specific overrides.
- Limits (optional)
  - `PLOY_BUILDGATE_LIMIT_MEMORY_BYTES` — memory cap (supports `MiB`, `GiB`, etc.).
  - `PLOY_BUILDGATE_LIMIT_DISK_SPACE` — disk/quota cap for container writable layer (supports suffixes).
  - `PLOY_BUILDGATE_LIMIT_CPU_MILLIS` — CPU in millicores (e.g., `500`, `1500`).

Behavior
- For `java-maven`: run `mvn --ff -B -q -e -DskipTests=false -Dstyle.color=never -f /workspace/pom.xml clean install`.
- For `java-gradle`: run `gradle -q --stacktrace test -p /workspace`.
- For `java`: compile all `*.java` under `/workspace` with `javac --release 17` (succeeds when no Java sources are present).
- The gate does not modify the repo; it validates the current working tree.

Outputs
- Exit code: `0` = success; non‑zero = error.
- Logs: combined stdout/stderr captured and truncated to ≤1 MiB; uploaded as `build-gate.log` artifact.
- Summary: pass/fail flag, duration, optional resource usage (if available from Docker stats).
- API exposure: gate status is surfaced via `ploy mod inspect <ticket-id>`.
  - Format: `Gate: passed duration=1234ms` or `Gate: failed pre-gate duration=567ms`.
  - Accessible without inspecting raw artifacts via `Metadata["gate_summary"]` in `GET /v1/mods/{id}` responses.

Notes
- When the container runtime is unavailable, the gate is skipped (no-op) and metadata is empty.
- Disk limit is driver dependent; if unsupported, container creation may fail early.

Healing Container Environment

Healing containers that need to call the Build Gate HTTP API directly receive the following
environment variables from the node agent:

- `PLOY_REPO_URL` — Git repository URL (same as the Mods run)
- `PLOY_BASE_REF` — Base Git reference (branch or tag) for the run
- `PLOY_TARGET_REF` — Target Git reference for the run
- `PLOY_COMMIT_SHA` — Pinned commit SHA when available
- `PLOY_SERVER_URL` — Control plane URL for Build Gate HTTP API access
- `PLOY_HOST_WORKSPACE` — Host filesystem path to workspace for in-container tooling
- `PLOY_API_TOKEN` — Bearer token for Build Gate API authentication when configured. On TLS-disabled
  local stacks the node agent may derive this from the worker bearer token file to simplify testing.

See `docs/envs/README.md` for the complete environment variable reference.
