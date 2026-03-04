# Build Gate Contract

Scope
- Minimal, stable contract to validate a repository after each Mods stage.
- Works in Mods and standalone CI.

## Overview: Unified Jobs Pipeline

Build Gate validation runs as part of the **unified jobs pipeline**. Gate jobs are
stored in the `jobs` table alongside mig and healing jobs, and nodes claim work
from a single queue with chain progression driven by `next_id`. There is no dedicated Build Gate
queue or separate worker mode—all nodes pull from the same jobs queue.

**Key characteristics:**
- **Single queue:** Gate jobs (`pre-gate`, `post-gate`, `re-gate`) are stored in the
  `jobs` table with the same schema as mig jobs.
- **Docker-based execution:** Gates execute locally on the claiming node via Docker
  containers. There is no remote HTTP Build Gate mode.
- **Chain progression:** Jobs advance through `next_id` successor links, ensuring
  sequential execution of pre-gate → mig → post-gate flows.
- **Workspace semantics:** Gate validation runs against the local workspace on the
  node. For re-gates after healing, the workspace already contains accumulated changes.

**Removed components (historical):**
- HTTP Build Gate API (`POST /v1/buildgate/validate`, `/v1/buildgate/jobs/{id}`)
- Dedicated `buildgate_jobs` table
- `PLOY_BUILDGATE_MODE` and `PLOY_BUILDGATE_WORKER_ENABLED` environment variables
- Remote HTTP gate executor and Build Gate worker node designation

## Execution Flow

Gate validation is orchestrated by the node agent as part of the Mods run lifecycle:

```
┌─────────────────────┐     ┌────────────────────┐     ┌───────────────────────┐
│ Control Plane       │     │ Jobs Queue         │     │ Node Agent            │
│ (creates jobs)      │────▶│ (jobs table)       │────▶│ (claims & executes)   │
└─────────────────────┘     └────────────────────┘     └───────────────────────┘
                                                                │
                                                                ▼
                                                       ┌───────────────────────┐
                                                       │ Docker Gate Executor  │
                                                       │ (local container)     │
                                                       └───────────────────────┘
                                                                │
                                                                ▼
                                                       ┌───────────────────────┐
                                                       │ BuildGateStage        │
                                                       │ Metadata (pass/fail)  │
                                                       └───────────────────────┘
```

**Flow:**
1. Control plane creates gate jobs (`pre-gate`, `post-gate`) in the `jobs` table.
2. Node agent claims the next queued job via `/v1/nodes/{id}/claim`.
3. For gate jobs, the node executes validation using the Docker gate executor.
4. Gate runs inside a Docker container with the workspace mounted at `/workspace`.
5. Results are captured as `BuildGateStageMetadata` (passed/failed, duration, logs).
6. Node reports completion via `/v1/jobs/{job_id}/complete`.

## Gate Executor

The `GateExecutor` interface (`internal/workflow/step`) provides a unified
abstraction for gate validation. The only implementation is the Docker-based executor:

**Code path:** `internal/workflow/step/gate_docker.go`

**Characteristics:**
- Workspace is local to the node; no network transfer of code.
- Gate execution runs in a Docker container with the working tree mounted.
- Build tools have direct access to the workspace.
- Gate results are captured and returned as `BuildGateStageMetadata`.

### Internal Planning Flow

Gate execution planning inside `internal/workflow/step/gate_docker.go` and
`internal/workflow/step/gate_docker_stack_gate.go` is intentionally flattened:

1. `stackdetect.Detect` runs once per gate execution.
2. `resolveGateExecutionPlan` produces either:
   - an executable plan (`image`, `cmd`, detected language/tool), or
   - a terminal gate result with prebuilt metadata/error for mismatch/unknown cases.
3. If a plan is returned, Docker execution runs once and `BuildGateStageMetadata`
   is built from container result + logs.
4. Metadata now includes `detected_stack` (`language`, `tool`, optional `release`)
   as the canonical detected gate identity used by recovery candidate validation.

The stack-gate and detected-stack branches share focused terminal-metadata builders,
so error codes/messages and `RuntimeImage` reporting stay consistent without duplicating
gate terminal-state wrappers.

See `docs/migs-lifecycle.md` section 1.1 for gate sequence diagrams and healing
flow details.

## Gate Configuration

Gates are configured via the mig spec and environment variables on worker nodes.

**Spec configuration:**
```yaml
build_gate:
  enabled: true
  images: [] # optional stack→image overrides
  pre:
    stack:
      enabled: true
      language: java
      release: "11"
      default: true
  post:
    stack:
      enabled: true
      language: java
      release: "17"
      default: true
```

- `build_gate.pre.stack` applies to the `pre_gate` job.
- `build_gate.post.stack` applies to the `post_gate` job.
- `build_gate.pre.gate_profile` applies to the `pre_gate` command/env override.
- `build_gate.post.gate_profile` applies to `post_gate` and `re_gate` command/env overrides.
- `build_gate.pre.target` / `build_gate.post.target` pin gate execution target (`build|unit|all_tests`).
- `build_gate.pre.always` / `build_gate.post.always` force gate execution even when a prior exact profile would allow skip.
- When `stack.enabled: true`, Build Gate rejects a detected stack mismatch (e.g. configured `release: "11"` but detected `"17"`).
- When `default: true`, if stack detection cannot determine tool or release, Build Gate falls back to the configured stack. If `tool` is omitted, a detected tool is used when available.
- When `default: false`, stack detection failures cancel execution for the repo (job status `Cancelled`), and remaining jobs are cancelled.
- For Gradle, Java `release` detection reads `sourceCompatibility` / `targetCompatibility` (and falls back to Kotlin `kotlinOptions.jvmTarget`); unrelated build logic (tasks, repositories, `ext[...]`, etc.) does not block detection.

`re_gate` always re-runs Build Gate using the stack detected from the workspace (via `stackdetect`) to select the gate runtime image/tool.

### Prep Override Precedence

Gate command resolution uses the following precedence (highest wins):
1. Explicit run spec override: `build_gate.<phase>.gate_profile`
2. For `re_gate`: validated infra recovery candidate override
3. Claim-time resolved gate profile from `gate_profiles` (exact `(repo_id, repo_sha_in, stack_id)` row)
4. Detected-tool fallback command (`buildCommandForTool`)

Claim-time gate profile lookup honors strict phase stack requirements:
- When `build_gate.<phase>.stack` is strict (`enabled: true` and `default: false`),
  profile resolution filters by required stack (`language`, `release`, optional `tool`)
  before image/repo-sha/any-stack fallback paths.
- If no stack row matches those required parameters, no gate profile is resolved for that gate claim.

Successful gate profile persistence:
- On successful `pre_gate`, `post_gate`, or `re_gate`, the server persists an exact
  profile row keyed by `(repo_id, repo_sha_out, stack_id)` (falling back to
  `repo_sha_in` when `repo_sha_out` is unavailable).
- The persisted payload marks the active gate target as `passed` and updates
  `gates(job_id, profile_id)` for auditability.

Target pin behavior:
- If `build_gate.<phase>.target` is set and differs from a prep override target, Build Gate ignores that prep override and runs the pinned target command.
- Prep stack validation (`prep stack mismatch`) is evaluated only when the prep override is actually selected for execution.

Default gate profiles are seeded from `gates/stacks.yaml` + `gates/profiles/*.yaml`
at server startup. Pre-gate runtime auto-bootstrap is removed.
Seeded Gradle profiles are wrapper-aware for runnable targets (`build`, `unit`,
`all_tests`): they use `./gradlew` when `/workspace/gradlew` is executable and
`/workspace/gradle/wrapper/gradle-wrapper.properties` exists; otherwise they use `gradle`.

Gate profile mapping for simple mode:
- Gate phase still selects destination override slot (`build_gate.pre.gate_profile` for `pre_gate`; `build_gate.post.gate_profile` for `post_gate` and `re_gate`).
- Runtime command/env source is always `gate_profile.targets.<targets.active>`.
- Runtime mapping is status-agnostic: `status`/`failure_code` do not control command/env selection.
- Runtime does not auto-fallback across targets; only `targets.active` decides command/env source.
- `targets.active=unsupported` is terminal for gate mapping and injects no runnable override.
- Terminal unsupported contract requires:
  - `targets.build.status=failed`
  - `targets.build.failure_code=infra_support`
- Runtime hint mapping:
  - `runtime.docker.mode=host_socket` injects `DOCKER_HOST=unix:///var/run/docker.sock`
  - `runtime.docker.mode=tcp` injects `DOCKER_HOST=<runtime.docker.host>`
  - `runtime.docker.api_version` injects `DOCKER_API_VERSION=<value>` when set
  - when `DOCKER_HOST` is `unix://...`, Build Gate mounts that socket path into the gate container automatically.

Environment precedence for gate execution:
1. Gate env from run/spec (`spec.env` + global env injection)
2. `build_gate.<phase>.gate_profile.env` (override wins on key conflicts)

### Stack Gate: Build Gate Image Mapping

When Stack Gate is enabled for a gate phase (a gate job carries `gate.stack_gate.expect`),
Build Gate resolves its runtime image from an explicit stack→image mapping.

**Resolution sources and precedence (highest wins):**
1. Mod spec: `build_gate.images[]` (per-stack overrides)
2. Default catalog: `gates/stacks.yaml` (installed at `/etc/ploy/gates/stacks.yaml` in Docker images)

**Default catalog shipping:**
- The repository default lives at `gates/stacks.yaml`.
- The `ploy-node` and `ploy-server` Docker images include it at `/etc/ploy/gates/stacks.yaml` by default.

**Rule format:**
```yaml
stacks:
  - lang: java
    release: "17"
    tool: maven
    image: ${PLOY_CONTAINER_REGISTRY}/maven:3-eclipse-temurin-17
    profile: gates/profiles/java-17-maven.yaml
```

**Validation:**
- `lang`, `release`, `image`, and `profile` are required; `tool` is optional.
- Duplicate selectors in the catalog are rejected.
- Referenced profile files must exist.

**Environment variables (on worker nodes):**
- Resource limits: `PLOY_BUILDGATE_LIMIT_MEMORY_BYTES`, `PLOY_BUILDGATE_LIMIT_CPU_MILLIS`,
  `PLOY_BUILDGATE_LIMIT_DISK_SPACE`.

See `docs/envs/README.md` for the complete environment variable reference.

## Inputs

- **Workspace mount:** `/workspace` (required, read-write). Contains the Git checkout.
- **Output mount:** `/out` (read-write), backed by a workspace-local host directory for gate artifacts.
- **Resource limits:** Memory, CPU, and disk limits are optional and configurable via
  environment variables.

## Behavior

Gate validation behavior depends on the detected tool (from stack detection):

| Tool    | Command                                                                                   |
|---------|-------------------------------------------------------------------------------------------|
| `maven` | `mvn --ff -B -q -e -DskipTests=false -Dstyle.color=never -f /workspace/pom.xml clean install` |
| `gradle`| `./gradlew -q --stacktrace --build-cache test -p /workspace` when `gradle/wrapper/gradle-wrapper.properties` exists; otherwise `gradle -q --stacktrace --build-cache test -p /workspace` |
| `go`    | `go test ./...`                                                                           |
| `cargo` | `cargo test`                                                                              |
| `pip` / `poetry` | `python -m compileall -q /workspace`                                               |

The gate does not modify the repository; it validates the current working tree.

## Outputs

- **Exit code:** `0` = success; non-zero = failure.
- **Logs:** Combined stdout/stderr captured and truncated to ≤10 MiB; uploaded as
  `build-gate.log` artifact.
- **Summary:** Pass/fail flag, duration, optional resource usage.
- **Detected stack identity:** `BuildGateStageMetadata.detected_stack` captures
  normalized detected stack (`language`, `tool`, optional `release`) for gate and
  healing/re-gate stack-aware decisions.
- **Gradle test reports:** Gate Gradle images force test reports into `/out/gradle-test-results` (JUnit XML)
  and `/out/gradle-test-report` (HTML) via image init script. Node uploads these bundles to object storage
  and records artifact links in `BuildGateStageMetadata.report_links`.
- **Gradle SBOM (when CycloneDX task exists):** Gate Gradle init script wires `cyclonedxBom` to run after
  `build`/`test`, forces JSON output, and writes it under `/out` (for example `sbom.cyclonedx.json`), so
  `build -x test` gates can still produce SBOM evidence.
- **API exposure:** Gate status is surfaced via `GET /v1/runs/{id}/status` and `Metadata["gate_summary"]` on the run.
  - Format: `Gate: passed duration=1234ms` or `Gate: failed pre-gate duration=567ms`.
  - Accessible via `Metadata["gate_summary"]` in `GET /v1/runs/{id}/status` responses.

### `/out` Artifact Contract

- Gate-generated files must be written under `/out/*` (no repository writes required for artifacts).
- Node artifact upload treats gate `/out/*` as first-class output and preserves deterministic `out/` archive-relative paths.
- SBOM files found in uploaded gate artifacts are parsed into normalized package rows (`lib`, `ver`) keyed by producing `job_id` and `repo_id`.
- SBOM rows are persisted only for successful gate jobs (`pre_gate`, `post_gate`, `re_gate`).
- Compatibility lookup contract for `deps` healing:
  - `GET /v1/sboms/compat?lang=<lang>&release=<release>&tool=<tool>&libs=<name>:<ver>,<name>`
  - Returns per-lib minimum successful version evidence for the resolved stack.
  - Returns `null` when no successful SBOM evidence exists for the requested stack.

## Notes

- When the container runtime is unavailable, gate execution fails immediately and
  the run does not proceed to mig execution.
- Disk limit is driver dependent; if unsupported, container creation may fail early.

## Healing Container Environment

Healing containers receive environment variables from the node agent to support
Build Gate verification. Since gate execution is local (no HTTP API), these variables
provide repository metadata for healing migs that need Git baseline information.

**Repo metadata (injected from StartRunRequest):**
- `PLOY_REPO_URL` — Git repository URL for the Mods run.
- `PLOY_BASE_REF` — Base Git reference (branch or tag).
- `PLOY_TARGET_REF` — Target Git reference for the run.
- `PLOY_COMMIT_SHA` — Pinned commit SHA when available.

**Server connection details:**
- `PLOY_SERVER_URL` — Control plane base URL.
- `PLOY_HOST_WORKSPACE` — Host filesystem path to workspace.

**Cross-phase inputs (mounted at `/in`):**
- `/in/build-gate.log` — First Build Gate failure log (read-only).
- `/in/gate_profile.json` — Gate profile used by the failed gate when available (provided for `infra` healing).
- `/in/gate_profile.schema.json` — Gate profile schema contract for infra healing (`title: Ploy Build Gate Profile`, includes `$comment` guidance for agent-facing fields).
- `/in/deps-compat-url.txt` — Stack-prefilled SBOM compatibility endpoint (provided for `deps` healing).
- `/in/deps-bumps.json` — Prior cumulative dependency bump state (provided for `deps` healing).

Primary source for these inputs is the typed `recovery_context` returned by
`POST /v1/nodes/{id}/claim` for `heal`/`re_gate` jobs. Node-local run cache files
remain an optional fallback optimization when claim context fields are absent.
- For `selected_error_kind=deps`, claim `recovery_context` carries:
  - `deps_compat_endpoint`
  - `deps_bumps`
- `/in/prompt.txt` — Optional prompt file when provided in spec.

**Healing workspace policy:**
- `build_gate.healing.selected_error_kind=infra`: healing is output-only and must not modify `/workspace`; any workspace diff fails the heal job with `healing_warning=unexpected_workspace_changes`.
- `build_gate.healing.selected_error_kind!=infra`: healing must modify `/workspace`; no workspace diff fails the heal job with `healing_warning=no_workspace_changes`.

See `docs/envs/README.md` for the complete environment variable reference.

## Implementation References

- Gate executor: `internal/workflow/step/gate_docker.go`
- Gate job execution: `internal/nodeagent/execution_orchestrator_gate.go`
- Router runtime: `internal/nodeagent/execution_orchestrator_router_runtime.go`
- Healing job execution: `internal/nodeagent/execution_orchestrator_jobs.go`
- Healing runtime helpers: `internal/nodeagent/execution_orchestrator_healing_runtime.go`
- Run orchestration: `internal/nodeagent/execution_orchestrator.go`
- Job claiming: `internal/store/queries/jobs.sql` (`ClaimJob` query)
- Contracts: `internal/workflow/contracts/build_gate_metadata.go`

## Historical Note

Prior to the unified jobs pipeline, Build Gate supported an HTTP remote execution
mode with dedicated `buildgate_jobs` table and worker designation via
`PLOY_BUILDGATE_WORKER_ENABLED`. This mode has been removed. All gate execution
now runs locally on the node claiming the gate job from the unified queue.

See git history for the migration rationale for collapsing gate execution into the jobs pipeline.
