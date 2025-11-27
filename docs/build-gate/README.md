Build Gate Contract

Scope
- Minimal, stable contract to validate a repository after each Mods stage.
- Works in Mods and standalone CI.

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
- For `java-maven`: run `mvn -B -q -DskipTests=false -Dstyle.color=never -f /workspace/pom.xml test`.
- For `java-gradle`: run `gradle -q test -p /workspace`.
- For `java`: compile all `*.java` under `/workspace` with `javac --release 17` (succeeds when no Java sources are present).
- The gate does not modify the repo; it validates the current working tree.

Outputs
- Exit code: `0` = success; non‑zero = error.
- Logs: combined stdout/stderr captured and truncated to ≤256 KiB; uploaded as `build-gate.log` artifact.
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
