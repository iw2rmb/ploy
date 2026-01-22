# Build Gate Contract

Scope
- Minimal, stable contract to validate a repository after each Mods stage.
- Works in Mods and standalone CI.

## Overview: Unified Jobs Pipeline

Build Gate validation runs as part of the **unified jobs pipeline**. Gate jobs are
stored in the `jobs` table alongside mod and healing jobs, and nodes claim work
from a single FIFO queue ordered by `step_index`. There is no dedicated Build Gate
queue or separate worker mode—all nodes pull from the same jobs queue.

**Key characteristics:**
- **Single queue:** Gate jobs (`pre-gate`, `post-gate`, `re-gate`) are stored in the
  `jobs` table with the same schema as mod jobs.
- **Docker-based execution:** Gates execute locally on the claiming node via Docker
  containers. There is no remote HTTP Build Gate mode.
- **FIFO ordering:** Jobs are claimed in `step_index` order, ensuring sequential
  execution of pre-gate → mod → post-gate flows.
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

The `GateExecutor` interface (`internal/workflow/runtime/step`) provides a unified
abstraction for gate validation. The only implementation is the Docker-based executor:

**Code path:** `internal/workflow/runtime/step/gate_docker.go`

**Characteristics:**
- Workspace is local to the node; no network transfer of code.
- Gate execution runs in a Docker container with the working tree mounted.
- Build tools have direct access to the workspace.
- Gate results are captured and returned as `BuildGateStageMetadata`.

See `docs/mods-lifecycle.md` section 1.1 for gate sequence diagrams and healing
flow details.

## Gate Configuration

Gates are configured via the mod spec and environment variables on worker nodes.

**Spec configuration:**
```yaml
build_gate:
  enabled: true
  profile: auto  # auto, java, java-maven, java-gradle
```

### Stack Gate: Build Gate Image Mapping

When Stack Gate is enabled for a gate phase (a gate job carries `gate.stack_gate.expect`),
Build Gate can resolve its runtime image from an explicit stack→image mapping instead of
profile-based defaults.

**Resolution sources and precedence (highest wins):**
1. Mod spec: `build_gate.images[]` (mod-level overrides)
2. Node config: `gates.build_gate.images[]` (cluster/global inline)
3. Default file: `/etc/ploy/gates/build-gate-images.yaml`

**Rule format:**
```yaml
images:
  - stack: { language: java, tool: maven, release: "17" }
    image: docker.io/org/stack-gate-java-maven:17
  # tool-agnostic fallback (used only when Stack Gate expectation omits tool)
  - stack: { language: java, release: "17" }
    image: docker.io/org/stack-gate-java:17
```

**Validation:**
- `stack.language`, `stack.release`, and `image` are required; `stack.tool` is optional.
- Duplicate selectors within the same source are rejected.
- When Stack Gate mode is active and `PLOY_BUILDGATE_IMAGE` is not set, missing
  `/etc/ploy/gates/build-gate-images.yaml` is an error.

**Profile detection:**
- `auto` (default): Detects Maven if `pom.xml` exists; Gradle if `build.gradle(.kts)`
  exists; otherwise plain `java`.
- Explicit profiles: `java-maven`, `java-gradle`, `java`.

**Environment variables (on worker nodes):**
- `PLOY_BUILDGATE_IMAGE` — Unified override for the gate container image.
- `PLOY_BUILDGATE_PROFILE` — Override profile detection.
- `PLOY_BUILDGATE_JAVA_IMAGE` — (Deprecated) Maven image override.
- `PLOY_BUILDGATE_GRADLE_IMAGE` — (Deprecated) Gradle image override.
- Resource limits: `PLOY_BUILDGATE_LIMIT_MEMORY_BYTES`, `PLOY_BUILDGATE_LIMIT_CPU_MILLIS`,
  `PLOY_BUILDGATE_LIMIT_DISK_SPACE`.

See `docs/envs/README.md` for the complete environment variable reference.

## Inputs

- **Workspace mount:** `/workspace` (required, read-write). Contains the Git checkout.
- **Profile:** Configurable via spec or auto-detected from workspace markers.
- **Optional image override:** `PLOY_BUILDGATE_IMAGE` takes precedence over profile defaults.
- **Resource limits:** Memory, CPU, and disk limits are optional and configurable via
  environment variables.

## Behavior

Gate validation behavior depends on the detected or configured profile:

| Profile       | Command                                                                                   |
|---------------|-------------------------------------------------------------------------------------------|
| `java-maven`  | `mvn --ff -B -q -e -DskipTests=false -Dstyle.color=never -f /workspace/pom.xml clean install` |
| `java-gradle` | `gradle -q --stacktrace test -p /workspace`                                               |
| `java`        | `javac --release 17` on all `*.java` files (succeeds when no sources present)             |

The gate does not modify the repository; it validates the current working tree.

## Outputs

- **Exit code:** `0` = success; non-zero = failure.
- **Logs:** Combined stdout/stderr captured and truncated to ≤1 MiB; uploaded as
  `build-gate.log` artifact.
- **Summary:** Pass/fail flag, duration, optional resource usage.
- **API exposure:** Gate status is surfaced via `GET /v1/runs/{id}/status` and `Metadata["gate_summary"]` on the run.
  - Format: `Gate: passed duration=1234ms` or `Gate: failed pre-gate duration=567ms`.
  - Accessible via `Metadata["gate_summary"]` in `GET /v1/runs/{id}/status` responses.

## Notes

- When the container runtime is unavailable, the gate is skipped (no-op) and metadata
  is empty.
- Disk limit is driver dependent; if unsupported, container creation may fail early.

## Healing Container Environment

Healing containers receive environment variables from the node agent to support
Build Gate verification. Since gate execution is local (no HTTP API), these variables
provide repository metadata for healing mods that need Git baseline information.

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
- `/in/prompt.txt` — Optional prompt file when provided in spec.

See `docs/envs/README.md` for the complete environment variable reference.

## Implementation References

- Gate executor: `internal/workflow/runtime/step/gate_docker.go`
- Gate+healing orchestration: `internal/nodeagent/execution_healing.go`
- Run orchestration: `internal/nodeagent/execution_orchestrator.go`
- Job claiming: `internal/store/queries/jobs.sql` (`ClaimJob` query)
- Contracts: `internal/workflow/contracts/build_gate_metadata.go`

## Historical Note

Prior to the unified jobs pipeline, Build Gate supported an HTTP remote execution
mode with dedicated `buildgate_jobs` table and worker designation via
`PLOY_BUILDGATE_WORKER_ENABLED`. This mode has been removed. All gate execution
now runs locally on the node claiming the gate job from the unified queue.

See git history for the migration rationale for collapsing gate execution into the jobs pipeline.
