Build Gate Contract

Scope
- Minimal, stable contract to validate a repository after each Mods stage.
- Works in Mods and standalone CI.

## Build Gate Execution Paths

Ploy supports two distinct Build Gate execution paths. Understanding these paths
establishes the baseline for the planned decoupling work (ROADMAP.md Phase A–F).

### Local Docker Gate (Current Default)

The **local docker gate** runs build validation directly on the node agent using
a mounted workspace. This is the CANONICAL executor for Mods pre-gate and re-gate
after healing.

**Code path:** `internal/workflow/runtime/step/gate_docker.go`

**Flow:**
1. Node agent claims a run via `POST /v1/nodes/{id}/claim`.
2. Node hydrates workspace (repo clone + optional diffs).
3. `dockerGateExecutor.Execute()` runs build commands inside a language-specific
   container image (e.g., `maven:3-eclipse-temurin-17`).
4. Workspace is mounted at `/workspace` inside the container.
5. Gate metadata (`BuildGateStageMetadata`) is captured and attached to run stats.
6. For healing flows: re-gate runs after each healing attempt using the same
   `dockerGateExecutor`, ensuring consistent validation semantics.

**Characteristics:**
- Workspace is local to the node; no network transfer of code.
- Gate execution is coupled to the node running the Mods step.
- Build tools have direct access to the working tree.
- Full gate history (pre-gate + all re-gates) is captured in `BuildGateStageMetadata`.

### HTTP Build Gate API (Remote Execution)

The **HTTP Build Gate API** provides a repo+diff validation model for remote or
decoupled gate execution. Callers submit validation jobs via HTTP; worker nodes
claim and execute them.

**Code paths:**
- Server handlers: `internal/server/handlers/handlers_buildgate.go`
- Job storage: `internal/store/buildgate_jobs.sql.go`

**Endpoints:**
- `POST /v1/buildgate/validate` — Submit a validation job (repo_url + ref + optional diff_patch).
- `GET /v1/buildgate/jobs/{id}` — Poll job status until completed/failed.
- `POST /v1/nodes/{id}/buildgate/claim` — Worker nodes claim pending jobs.
- `POST /v1/nodes/{id}/buildgate/{job_id}/ack` — Acknowledge job start (transition to running).
- `POST /v1/nodes/{id}/buildgate/{job_id}/complete` — Report job completion with result.

**Flow:**
1. Caller submits `BuildGateValidateRequest` with `repo_url`, `ref`, and optional
   `diff_patch` (gzipped unified diff for healing flows).
2. Control plane creates a `buildgate_jobs` row with status `pending`.
3. If job completes within `syncWaitTimeout` (30s), result returns synchronously.
4. Otherwise, caller receives `job_id` with status `pending` for async polling.
5. Worker node claims job via `/v1/nodes/{id}/buildgate/claim`.
6. Worker clones repo at ref, applies diff_patch if present, runs build validation.
7. Worker reports completion via `/v1/nodes/{id}/buildgate/{job_id}/complete`.

**Characteristics:**
- Workspace is reconstructed from repo+diff; no local state dependency.
- Gate execution can run on any eligible Build Gate worker node.
- Supports multi-VPS deployments where gate and Mods run on different nodes.
- Job queue enables load distribution across workers.

### Target State

The **target state** (ROADMAP.md Phases B–F) is:
- Mods and healing call the HTTP Build Gate API (repo+diff model).
- Build Gate workers encapsulate docker execution as an implementation detail.
- Gate jobs can run on any eligible worker, decoupled from the node executing
  the Mods step.

This decoupling enables:
- Horizontal scaling of gate validation.
- Separation of concerns: Mods nodes handle mod execution; gate nodes handle
  build validation.
- Consistent workspace semantics via repo+diff reconstruction.

See `docs/mods-lifecycle.md` section 1.1 for gate sequence diagrams and healing
flow details.

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
- For `java-gradle`: run `gradle -q --stacktrace --fail-fast test -p /workspace`.
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
