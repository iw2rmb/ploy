# Mods Lifecycle and Architecture

This document is the canonical reference for how Mods runs are represented and
executed across the CLI, control plane, and node agents. It replaces the older
checkpoint notes in the repository.

## 1. Core Concepts

- **Ticket** — A Mods run submitted to the control plane. Tickets are stored as
  `runs` rows in PostgreSQL and exposed via the `/v1/mods` API.
- **Job** — A unit of work inside a ticket (for example `pre-gate`, `mod-0`,
  `post-gate`). Jobs are stored as `jobs` rows.
- **StepIndex** — Float index that orders jobs and ties them to diffs. Jobs use
  float step_index (e.g., 1000, 2000, 3000) to allow dynamic insertion of
  healing jobs between existing jobs.
- **Spec** — YAML/JSON file or inline JSON describing container image,
  command, env, Build Gate and optional `mods[]` steps. Parsed by the CLI in
  `cmd/ploy/mod_run_spec.go`.
- **Build Gate** — Validation pass run via the HTTP Build Gate API to ensure the
  workspace compiles/tests successfully. The `GateExecutor` adapter
  (`internal/workflow/runtime/step`) abstracts remote execution; Build Gate workers
  claim and execute jobs. Gates run at two distinct points in the lifecycle:
  - **Pre-mod gate** — runs once on the initial workspace before any mods execute.
  - **Post-mod gate** — runs after each mod in `mods[]` that exits with code 0.
- **Healing** — Optional corrective steps run when any Build Gate (pre or post)
  fails. The system enters a fail → heal mods → re-gate loop; if the gate still
  fails after retries, the run terminates.

## 1.1 Build Gate Sequence

This section makes the pre-/post-gate execution order explicit for both
single-mod and multi-mod runs. All gate failures follow the same healing
protocol: fail → heal mods → re-gate; if healing is exhausted, the run fails
and no further mods execute.

### Single-mod runs (no `mods[]`)

When the spec contains a single `mod` entry (or uses the legacy top-level
image/command), the execution sequence is:

```
pre-gate(+healing) → mod → post-gate(+healing)
```

1. **Pre-mod Build Gate** — Runs once on the initial hydrated workspace (step 0)
   before the mod container starts. Validates that the baseline code compiles
   and tests pass.
   - On failure with healing mods configured: enter fail → heal → re-gate loop.
   - If healing is exhausted: run exits without executing the mod.

2. **Mod execution** — The mod container runs against the validated workspace.
   - Exit code 0: proceed to post-mod gate.
   - Non-zero exit: run fails; no post-mod gate is run.

3. **Post-mod Build Gate** — Runs on the same workspace after the mod exits
   with code 0. Validates that the mod's changes do not break the build.
   - On failure with healing mods configured: enter fail → heal → re-gate loop.
   - If healing is exhausted: run fails.

### Multi-mod runs (`mods[]`)

When the spec contains a `mods[]` array with multiple entries, the execution
sequence is:

```
pre-gate(+healing) → mod[0] → post-gate[0](+healing) → mod[1] → post-gate[1](+healing) → ... → mod[N-1] → post-gate[N-1](+healing)
```

1. **Pre-mod Build Gate** — Runs once on the initial hydrated workspace before
   any mods execute.
   - On failure with healing: enter fail → heal → re-gate loop.
   - If healing exhausted: run exits without executing any mods.

2. **For each mod[k] in `mods[]` (k = 0, 1, ..., N-1)**:
   - **Mod[k] execution** — Runs against the workspace with changes from all
     prior mods applied.
   - **Post-mod gate[k]** — Runs after mod[k] exits with code 0.
     - On failure with healing: enter fail → heal → re-gate loop.
     - If healing exhausted: run fails and no further mods execute.
   - If mod[k] exits non-zero: run fails; no post-gate and no further mods.

### Remote gate execution via GateExecutor

Pre-gate and re-gate validation calls the HTTP Build Gate API through the
`GateExecutor` adapter. This decouples gate execution from the node running the
Mods step:

```
┌─────────────────────┐     ┌────────────────────┐     ┌───────────────────────┐
│ Node Orchestrator   │     │ GateExecutor       │     │ Control Plane         │
│ (execution_healing) │────▶│ (HTTP adapter)     │────▶│ POST /v1/buildgate/   │
│                     │     │                    │     │ validate              │
└─────────────────────┘     └────────────────────┘     └───────────────────────┘
                                                                │
                                                                ▼
                            ┌────────────────────┐     ┌───────────────────────┐
                            │ Build Gate Worker  │◀────│ Job Queue (pending)   │
                            │ (docker execution) │     │                       │
                            └────────────────────┘     └───────────────────────┘
                                      │
                                      ▼
                            ┌────────────────────┐
                            │ BuildGateStage     │
                            │ Metadata returned  │
                            │ (passed/failed)    │
                            └────────────────────┘
```

**Flow:**
1. Orchestrator calls `GateExecutor.Execute()` with repo URL, ref, and optional diff_patch.
2. The HTTP adapter submits a validation job to `POST /v1/buildgate/validate`.
3. A Build Gate worker claims the job, executes docker validation, and reports results.
4. The adapter polls or waits for completion, returning `BuildGateStageMetadata`.
5. For healing flows: re-gate submits a new job with the workspace diff applied.

This architecture enables:
- Gate validation on dedicated Build Gate worker nodes (horizontal scaling).
- Mods execution and gate execution on different nodes (separation of concerns).
- Consistent workspace semantics via repo+diff reconstruction.

See `docs/build-gate/README.md` for HTTP API details and worker configuration.

### Gate failure semantics

All Build Gate failures (pre or post) follow identical handling:

- **Without healing mods**: The run fails immediately with `reason="build-gate"`.
- **With healing mods**: The system enters the fail → heal → re-gate loop:
  1. Gate fails: capture build output to `/in/build-gate.log`.
  2. Execute healing mods (e.g., Codex) to fix the issue.
  3. Re-run the gate on the healed workspace.
  4. Repeat until gate passes or max retries exhausted.
  5. If exhausted: run fails with `ErrBuildGateFailed`.

The final gate result (pre-gate for runs with no mods executed, or the last
post-gate) is surfaced in:
- `Metadata["gate_summary"]` in `GET /v1/mods/{id}` responses.
- `ploy mod inspect <ticket-id>` output as `Gate: passed|failed ...`.

### Multi-strategy healing

The healing configuration supports two forms for backward compatibility:

**Single-strategy (legacy form)** — A flat `mods` list executed sequentially:
```yaml
build_gate_healing:
  retries: 1
  mods:
    - image: docker.io/user/mods-codex:latest
      command: mod-codex --input /workspace --out /out
```

**Multi-strategy (branching form)** — Multiple named strategies that can be
executed in parallel by the control plane:
```yaml
build_gate_healing:
  retries: 2
  strategies:
    - name: codex-ai
      mods:
        - image: docker.io/user/mods-codex:latest
          command: mod-codex --input /workspace --out /out
    - name: static-patch
      mods:
        - image: docker.io/user/mods-patcher:latest
          command: apply-known-fixes.sh
```

When using the legacy `mods` form, it is internally normalized to a single
unnamed strategy, preserving existing behavior. If both `mods` and `strategies`
are present, `strategies` takes precedence.

#### Multi-strategy semantics

- **Independent workspaces**: Each strategy operates on its own workspace clone.
- **Parallel execution**: Strategies execute in parallel (subject to node availability).
- **Sequential mods within strategy**: Each strategy runs its mods[] sequentially,
  then triggers a re-gate.
- **First-wins racing**: The first strategy whose re-gate passes wins; other
  branches are canceled.
- **Exhaustion handling**: If all strategies exhaust retries without passing,
  the run fails.

This design enables racing different healing approaches (e.g., AI-powered vs.
deterministic patches) to reduce total healing time while ensuring the first
valid fix is applied.

#### Implementation references

- Type definitions: `internal/nodeagent/run_options.go` (`HealingConfig`,
  `HealingStrategy`, `NormalizedStrategies()`).
- Spec parsing: `internal/nodeagent/run_options.go` (`parseRunOptions`,
  `parseHealingStrategy`).
- Schema example: `docs/schemas/mod.example.yaml`.

### Workspace and rehydration semantics

This subsection clarifies which code version each Build Gate sees during execution.
Understanding workspace state is essential for debugging gate failures and reasoning
about multi-step runs where diffs accumulate across steps.

**Implementation reference:**
- `internal/nodeagent/execution_orchestrator.go` — `executeRun` and `rehydrateWorkspaceForStep`.

#### Pre-mod gate workspace

The **pre-mod gate** runs on the **initial hydrated workspace** (step 0). This workspace
is created by cloning the repository at `base_ref` (optionally checking out `commit_sha`)
and contains no modifications from any mods. The pre-mod gate validates that the baseline
code compiles and tests pass before any mods execute.

Workspace state for pre-mod gate:
```
base_ref (+ commit_sha if specified) → fresh clone → pre-mod gate
```

#### Post-mod gate workspace

Each **post-mod gate** runs on the **rehydrated workspace for that step**. The workspace
reflects all changes from prior mods (steps 0 through k-1) plus the changes from the
current mod (step k).

Before `mod[k]` executes, `rehydrateWorkspaceForStep` reconstructs the workspace for
step k from:

1. **Base clone**: A cached copy of the initial repository state (base_ref + commit_sha).
2. **Ordered diffs**: Diffs from steps 0 through k-1 fetched from the control plane and
   applied in order using `git apply`.

After `mod[k]` completes, its changes are present in the same workspace that the
post-mod gate validates.

Workspace state for post-mod gate at step k:
```
base_ref → base clone → apply diffs[0..k-1] → mod[k] execution → post-mod gate[k]
```

#### Multi-node execution

The rehydration strategy enables **multi-node execution**: any node can reconstruct
the workspace for step k by fetching the base clone and applying the ordered diff chain.
This decouples step execution from node affinity—step 0 can run on node A, step 1 on
node B, etc.

Key invariants:
- Each step uploads its diff (tagged with `step_index`) after successful execution.
- `rehydrateWorkspaceForStep` fetches diffs for steps `0..k-1` before executing step `k`.
- A baseline commit is created after rehydration (via `ensureBaselineCommitForRehydration`)
  so that `git diff HEAD` produces only the changes from step k, not cumulative changes.

#### Summary table

| Gate Phase     | Workspace State                                      | Code Reference                              |
|----------------|------------------------------------------------------|---------------------------------------------|
| Pre-mod gate   | Fresh clone of base_ref (+ commit_sha)               | `rehydrateWorkspaceForStep` with stepIndex=0 |
| Post-mod gate[k] | Base clone + diffs[0..k-1] + mod[k] changes         | `rehydrateWorkspaceForStep` with stepIndex=k |

### Implementation references

- Gate execution via HTTP API: `internal/workflow/runtime/step/gate_http.go` and
  `internal/workflow/runtime/step/gate_factory.go` (`GateExecutor`).
- Gate+healing orchestration: `internal/nodeagent/execution_healing.go`.
- Run orchestration: `internal/nodeagent/execution_orchestrator.go` (`executeRun`).
- Workspace rehydration: `internal/nodeagent/execution_orchestrator.go` (`rehydrateWorkspaceForStep`).
- Stats aggregation: `internal/domain/types/runstats.go` (`GateSummary()`).
- **Build Gate remote execution**: See `docs/build-gate/README.md` for the repo+diff
  validation model, HTTP API endpoints, and Build Gate worker configuration.

## 1.2 Stack-Aware Image Selection

Mods supports stack-aware image selection, allowing different container images to be
used based on the detected build stack. This enables optimized images for specific
build tools (e.g., dedicated Maven or Gradle images) while maintaining backward
compatibility with universal images.

### Image specification forms

The `image` field in `mod`, `mods[]`, and `build_gate_healing.mods[]` accepts two forms:

**Universal image (string)** — A single image used regardless of stack:
```yaml
mod:
  image: docker.io/user/mods-openrewrite:latest
```

**Stack-specific images (map)** — Different images per detected stack:
```yaml
mod:
  image:
    default: docker.io/user/mods-openrewrite:latest
    java-maven: docker.io/user/mods-orw-maven:latest
    java-gradle: docker.io/user/mods-orw-gradle:latest
```

### Stack detection via Build Gate

The Build Gate detects the workspace stack during validation based on file markers:

| Stack Name     | Detection Criteria                           | Build Tool |
|----------------|----------------------------------------------|------------|
| `java-maven`   | `pom.xml` present in workspace root          | Maven      |
| `java-gradle`  | `build.gradle` or `build.gradle.kts` present | Gradle     |
| `java`         | JDK markers but no build tool detected       | Generic    |
| `unknown`      | No recognized stack markers found            | None       |

The detected stack is propagated from the Build Gate to Mods steps via
`BuildGateStageMetadata.Tool`, which is converted to a `ModStack` using
`ToolToModStack()` in `internal/workflow/contracts/mod_image.go`.

### Image resolution rules

When resolving an image for a given stack:

1. **Universal image**: If `image` is a string, return it (ignores stack).
2. **Exact match**: If `image` is a map and contains the detected stack key
   (e.g., `java-maven`), use that image.
3. **Default fallback**: If no exact match, use the `default` key when present.
4. **Error**: If neither the stack key nor `default` exists, fail with an
   actionable error message.

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         Image Resolution Flow                               │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│   image: "docker.io/user/img:latest"                                        │
│       │                                                                     │
│       └──▶ Return "docker.io/user/img:latest" (universal, any stack)        │
│                                                                             │
│   image:                                                                    │
│     default: img:default                                                    │
│     java-maven: img:maven                                                   │
│     java-gradle: img:gradle                                                 │
│       │                                                                     │
│       ├─ stack="java-maven"  ──▶ Return "img:maven"     (exact match)       │
│       ├─ stack="java-gradle" ──▶ Return "img:gradle"    (exact match)       │
│       ├─ stack="java"        ──▶ Return "img:default"   (fallback)          │
│       ├─ stack="unknown"     ──▶ Return "img:default"   (fallback)          │
│       └─ stack="python-pip"  ──▶ Return "img:default"   (fallback)          │
│                                                                             │
│   image:                                                                    │
│     java-maven: img:maven   (NO default key)                                │
│       │                                                                     │
│       ├─ stack="java-maven"  ──▶ Return "img:maven"     (exact match)       │
│       └─ stack="java-gradle" ──▶ ERROR: no image for stack, no default      │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Consistency across run lifecycle

Stack detection occurs during the pre-mod Build Gate execution. The detected stack
is then used consistently for all subsequent Mods steps within the same run:

1. **Pre-mod gate**: Build Gate detects workspace stack (e.g., `java-maven`).
2. **Stack propagation**: The stack is stored in run context/metadata.
3. **Image resolution**: Each mod step resolves its image using the same stack.
4. **Healing steps**: Stack remains consistent across heal → re-gate cycles.

This ensures deterministic image selection: a Maven workspace always uses the
Maven-specific image throughout the entire run, including healing retries.

### Example: Stack-aware OpenRewrite

A common use case is dedicated OpenRewrite images for Maven and Gradle:

```yaml
mod:
  image:
    default: docker.io/user/mods-openrewrite:latest
    java-maven: docker.io/user/mods-orw-maven:latest
    java-gradle: docker.io/user/mods-orw-gradle:latest
  env:
    RECIPE_CLASSNAME: org.openrewrite.java.migrate.UpgradeToJava17
```

When this spec runs against a Maven project (`pom.xml` present):
- Build Gate detects `java-maven` stack.
- Image resolves to `mods-orw-maven:latest`.
- The Maven-specific entrypoint executes OpenRewrite via `mvn rewrite:run`.

When the same spec runs against a Gradle project (`build.gradle` present):
- Build Gate detects `java-gradle` stack.
- Image resolves to `mods-orw-gradle:latest`.
- The Gradle-specific entrypoint executes OpenRewrite via `gradle rewriteRun`.

### Implementation references

- Image type and resolution: `internal/workflow/contracts/mod_image.go`
  (`ModImage`, `ResolveImage`, `ParseModImage`, `ToolToModStack`).
- Stack propagation: `internal/workflow/contracts/build_gate_metadata.go`
  (`BuildGateStageMetadata.Tool`).
- Image resolution in executor: `internal/nodeagent/run_options.go`.
- Unit tests: `internal/workflow/contracts/mod_image_test.go`.

## 1.3 Job Graph (DAG) Visualization

Mods runs form a directed acyclic graph (DAG) of jobs. The graph package
(`internal/workflow/graph`) materializes jobs into an explicit graph structure
for visualization and debugging. Jobs are ordered by `step_index` (float values
like 1000, 2000, 3000), which determines execution order and edge relationships.

### Node types

| Type        | Description                                  | Example        |
|-------------|----------------------------------------------|----------------|
| `pre_gate`  | Pre-mod Build Gate validation                | `pre-gate`     |
| `mod`       | Modification container execution             | `mod-0`        |
| `post_gate` | Post-mod Build Gate validation               | `post-gate`    |
| `heal`      | Healing job after gate failure               | `heal-0`       |
| `re_gate`   | Re-validation after healing                  | `re-gate`      |

### Simple run graph

A successful single-mod run creates a linear three-node chain:

```
┌───────────┐       ┌───────────┐       ┌───────────┐
│ pre-gate  │──────▶│   mod-0   │──────▶│ post-gate │
│  (1000)   │       │  (2000)   │       │  (3000)   │
└───────────┘       └───────────┘       └───────────┘
```

### Healing run graph

When a gate fails with healing configured, heal and re-gate jobs are inserted
at midpoint `step_index` values:

```
┌───────────┐     ┌───────────┐     ┌───────────┐     ┌───────────┐     ┌───────────┐
│ pre-gate  │────▶│  heal-0   │────▶│  re-gate  │────▶│   mod-0   │────▶│ post-gate │
│  (1000)   │     │  (1250)   │     │  (1500)   │     │  (2000)   │     │  (3000)   │
│  FAILED   │     │           │     │  PASSED   │     │           │     │           │
└───────────┘     └───────────┘     └───────────┘     └───────────┘     └───────────┘
```

### Parallel healing branches (Phase E)

Multi-strategy healing creates concurrent branches with distinct `step_index`
windows. The first branch whose re-gate passes wins; losing branches are canceled:

```
                           ┌─────────────────────────────────────┐
                           │         Parallel Branches           │
                     ┌─────┴─────┐                         ┌─────┴─────┐
                     │ Branch A  │                         │ Branch B  │
                     └─────┬─────┘                         └─────┬─────┘
                           │                                     │
post-gate  ───────────────▶├─▶ heal-a (1500) → re-gate-a (1600) ─┤
 FAILED                    │                                     │
                           └─▶ heal-b (1700) → re-gate-b (1800) ─┘
                                                                 │
                                              (first pass wins) ─┘
```

### Implementation references

- Graph types: `internal/workflow/graph/types.go`
- Graph builder: `internal/workflow/graph/builder.go`
- Detailed DAG documentation: `ROADMAP_DAG.md`

## 2. Data Model

### 2.1 Ticket summary (`internal/mods/api`)

- `TicketSummary` (in `internal/mods/api/types.go`) is the wire type returned by
  `GET /v1/mods/{id}` and streamed on SSE:
  - `ticket_id` — run UUID.
  - `state` — ticket lifecycle state (`pending`, `running`, `succeeded`,
    `failed`, `cancelled`).
  - `repository` — repo URL for this run.
  - `metadata` — string map for additional diagnostics:
    - `repo_base_ref`, `repo_target_ref`
    - `node_id` (claiming worker)
    - `mr_url` (if MR was created)
    - `gate_summary` (Build Gate result)
    - `reason` (terminal error reason when available).
  - `stages` — map keyed by **job UUID** (`jobs.id`), value is `StageStatus`.
    Note: The `stages` field name is retained for API backward compatibility,
    but each entry represents a `jobs` table row. The map key is the job's UUID.

- `StageStatus`:
  - `state` — job lifecycle state (mirrors `jobs.status`).
  - `artifacts` — map of artifact logical names to bundle CIDs.
  - `step_index` — float index for job ordering (mirrors `jobs.step_index`).

### 2.2 Jobs and diffs

- **Jobs** (`jobs` table)
  - Created by the control plane when a run is submitted via `POST /v1/mods`.
  - Each job row has:
    - `id` — job UUID (used as key in `TicketSummary.stages`).
    - `name` — job name (e.g., `pre-gate`, `mod-0`, `post-gate`).
    - `step_index` — float for ordering (e.g., 1000, 2000, 3000).
  - `status` — job state (`created`, `pending`, `running`, `succeeded`,
      `failed`, `canceled`).
    - `node_id` — which node claimed this job.
    - `meta` — JSONB with job metadata:
      - `mod_type` — job phase (`pre_gate`, `mod`, `post_gate`, `heal`, `re_gate`).
      - `mod_image` — container image for this job (optional, for diagnostics).
  - Float `step_index` enables dynamic job insertion:
    - Initial jobs: `pre-gate` (1000), `mod-0` (2000), `post-gate` (3000).
    - Healing jobs inserted at midpoints: `heal-1` (1500), `re-gate` (1750).
    - `GetAdjacentJobIndices` query computes midpoints for insertion.

- **Server-driven scheduling**
  - Jobs are created with status `created` (not yet claimable) or `pending`
    (ready to claim). The first job (`pre-gate`) is created as `pending`.
  - `ClaimJob` (`internal/store/queries/jobs.sql`) only returns `pending`
    jobs. This ensures nodes cannot claim jobs until the server decides they
    are ready.
  - When a job completes successfully, `ScheduleNextJob` transitions the first
    `created` job to `pending`, allowing the next node claim.
  - This model enforces sequential execution: `pre-gate` → `mod-0` → `post-gate`.
  - Healing jobs follow the same pattern: heal jobs are created with status
    `pending` to be claimed immediately after insertion.

- **Diffs**
  - Generated by the workflow runtime (`internal/workflow/runtime/step`) and
    uploaded by nodeagents using `/v1/runs/{run_id}/jobs/{job_id}/diff`.
  - Exposed via:
    - `GET /v1/mods/{id}/diffs` (`internal/server/handlers/handlers_diffs.go`)
      — returns a list of diffs with `job_id`, `step_index` and summary
      metadata.
    - `GET /v1/diffs/{id}?download=true` — returns the gzipped unified diff.
  - Diffs are ordered by job `step_index` for rehydration.

### 2.3 Artifacts

- Nodeagents upload artifact bundles with:
  - `POST /v1/runs/{run_id}/jobs/{job_id}/artifact`.
  - Control plane exposes bundles per ticket:
    - `POST /v1/mods/{id}/artifact_bundles`.
    - `GET /v1/artifacts` and `GET /v1/artifacts/{id}` for listing/downloading
      by CID/id.
- `StageStatus.Artifacts` map keys are human-readable names; values are bundle
  CIDs.

## 3. Control Plane HTTP Surfaces

### 3.1 Mods endpoints (`internal/server/handlers`)

- `POST /v1/mods` — submit a Mods ticket.
  - Simplified shape: `{repo_url, base_ref, target_ref, commit_sha?, spec?, created_by?}`.
  - Handler: `submitTicketHandler`.
  - Behaviour:
    - Creates a `runs` row with `status=queued`.
    - Creates `jobs` rows from the spec (pre-gate, mod, post-gate).
    - Jobs use float step_index for ordering (1000, 2000, 3000).
    - Publishes an initial `TicketSummary` snapshot to SSE via
      `events.Service.PublishTicket`.

- `GET /v1/mods/{id}` — ticket status.
  - Handler: `getTicketStatusHandler`.
  - Aggregates:
    - `runs` row.
    - `jobs` rows (including `meta` JSONB with job metadata).
    - Artifact bundles per job.
    - Run stats (MR URL, gate summary).
  - Returns `TicketStatusResponse` (`modsapi.TicketStatusResponse{Ticket: TicketSummary}`).

- `GET /v1/mods/{id}/events` — SSE event stream for a ticket.
  - Handler: `getModEventsHandler`.
  - Uses the internal hub (`internal/stream`) and events service to stream:
    - `event: log`, data: `LogRecord {timestamp,stream,line}`.
    - `event: ticket`, data: `TicketSummary`.
    - `event: retention`, data: `RetentionHint`.
    - `event: done`, data: `Status {status:"done"}` sentinel.
  - Supports `Last-Event-ID` for resumption.

- `POST /v1/mods/{id}/cancel` — cancel a ticket.
  - Handler: `cancelTicketHandler`.
  - Behaviour:
    - Transitions run to `canceled`, updates jobs in `pending|running` to
      `canceled`.
    - Publishes a final `TicketSummary` with `state=cancelled`.
    - Emits a terminal `done` status on the stream.

- `GET /v1/mods/{id}/diffs` and `GET /v1/diffs/{id}` — diff list and download.
  - Handler: `listRunDiffsHandler` and `getDiffHandler`.
  - Enable node and CLI callers to enumerate and fetch per-step diffs.

- `POST /v1/mods/{id}/logs`, `POST /v1/mods/{id}/diffs`,
  `POST /v1/mods/{id}/artifact_bundles` — control-plane write endpoints used by
  nodeagents to persist logs, diffs and artifacts.

### 3.2 Node endpoints (`internal/server/handlers/register.go`)

Nodeagents use `/v1/nodes/*` to execute work:

- `POST /v1/nodes/{id}/heartbeat` — report node liveness.
- `POST /v1/nodes/{id}/claim` — claim a queued job (returns job with all prior
  jobs succeeded/skipped).
- `POST /v1/nodes/{id}/ack` — confirm job start.
- `POST /v1/nodes/{id}/complete` — report final status and stats for a job.
- `POST /v1/nodes/{id}/logs` — upload gzipped log chunks.
- `POST /v1/runs/{run_id}/jobs/{job_id}/diff` — upload per-job diffs.
- `POST /v1/runs/{run_id}/jobs/{job_id}/artifact` — upload per-job artifacts.
- `POST /v1/nodes/{id}/buildgate/*` — claim/ack/complete Build Gate jobs.

All mutating requests from worker nodes (POST/PUT/DELETE) must include the
`PLOY_NODE_UUID` header set to the node's UUID. The control plane uses this
header to validate job ownership and attribute artifacts/diffs to the correct
node.

## 4. Node Execution and Rehydration

### 4.1 Single-step runs

For a spec without `mods[]` (single `mod` or legacy top-level image):

1. CLI (`ploy mod run`) builds a `TicketSubmitRequest` in
   `cmd/ploy/mod_run_exec.go` and an optional spec JSON payload in
   `cmd/ploy/mod_run_spec.go`.
2. CLI submits to `POST /v1/mods`. The control plane:
   - Creates jobs (pre-gate, mod, post-gate) with float step_index.
   - Publishes an initial `TicketSummary`.
3. A node:
   - Claims jobs sequentially via `/v1/nodes/{id}/claim` (ClaimJob enforces
     dependency: only returns a job when all prior jobs succeeded/skipped).
   - For each claimed job:
     - Hydrates the workspace using `step.WorkspaceHydrator`.
     - Executes the job (gate check or mod container).
     - Generates diffs with `DiffGenerator` and uploads them.
     - Completes the job via `/v1/nodes/{id}/complete`.
4. Control plane updates ticket status and emits a final `ticket` snapshot plus
   a `done` status on the SSE stream.

### 4.2 Multi-step runs (`mods[]`) and rehydration

For a spec with `mods[]`:

1. CLI preserves the `mods[]` array as-is (`buildSpecPayload` does not rewrite
   or reorder entries).
2. `POST /v1/mods`:
   - Creates jobs for pre-gate, each mod, and post-gates with float step_index.
   - Job metadata includes `mod_type` (pre_gate, mod, post_gate, heal, re_gate)
     and `mod_image`.
3. Scheduler and nodeagents:
   - ClaimJob returns jobs in step_index order, but only when all prior jobs
     have succeeded or been skipped.
   - Execute each job against a workspace that reflects all prior steps.

Workspace rehydration is implemented in `internal/nodeagent/execution_orchestrator.go`:

- `rehydrateWorkspaceForStep`:
  - Copies the base clone (base_ref + optional commit_sha).
  - Applies diffs for prior jobs in order using `git apply`.
  - Diffs are fetched via `GET /v1/mods/{id}/diffs`, ordered by `step_index`.

- `ensureBaselineCommitForRehydration`:
  - After applying prior diffs, creates a local commit that becomes the new
    `HEAD`.
  - Ensures that `git diff HEAD` after the job produces an **incremental**
    patch containing only changes from that job.
  - Control plane stores per-job diffs under the job's `step_index`.

This design guarantees that:

- Any node can reconstruct the identical workspace for a job using base clone +
  prior diffs.
- Jobs execute sequentially due to ClaimJob dependency enforcement.

## 5. Container Contract for Mods Images

Mods container images are standard OCI images with the following expectations:

- **Workspace mounts**
  - `/workspace` — repository working tree (read-write) for the step.
  - `/out` — output directory for artifacts and summaries (read-write).
  - `/in` — optional read-only mount for cross-phase inputs such as:
    - initial Build Gate logs (`/in/build-gate.log`),
    - prompt files (`/in/prompt.txt`), etc.

- **Environment**
  - Spec `env` and `env_from_file` are resolved and merged by
    `buildSpecPayload`.
    - `env_from_file` paths are resolved on the CLI side and injected as string
      values.
    - Supported on:
      - top-level spec,
      - `mod` section,
      - each `mods[]` entry,
      - `build_gate_healing.mods[]`.

- **Execution**
  - Entry point should read/modify the repo under `/workspace`.
  - Output artifacts, logs and plans should be written under `/out`.
  - Exit code `0` signals success. Non-zero exit code is treated as failure and
    surfaces in:
    - ticket `state=failed`,
    - `metadata["reason"]` where available,
    - Build Gate summary (if the failure happened in the gate).

- **Retention**
  - `retain_container` in the spec causes the node runtime
    (`internal/workflow/runtime/step` and `internal/nodeagent`) to skip
    container removal after completion.
  - Logs are still streamed through `CreateAndPublishLog` and SSE.

## 6. CLI Surfaces for Mods

The CLI entry points for Mods are implemented in `cmd/ploy`:

- `ploy mod run`:
  - Parses flags in `cmd/ploy/mod_run_flags.go`.
  - Builds the spec payload in `cmd/ploy/mod_run_spec.go` (handles `env` and
    `env_from_file`).
  - Constructs `TicketSubmitRequest` with stage definitions in
    `cmd/ploy/mod_run_exec.go`.
  - Submits via `internal/cli/mods.SubmitCommand`.
  - Optional `--follow` streams ticket events via
    `internal/cli/mods.EventsCommand`, backed by `internal/cli/stream`.

- `ploy mods logs <ticket>`:
  - Streams logs from `/v1/mods/{id}/events`, focusing on `log` and
    `retention` events (see `cmd/ploy/mods_jobs_commands.go` and
    `internal/cli/runs/follow.go`).

- `ploy runs inspect <ticket>`:
  - Calls `GET /v1/mods/{id}` and prints a concise summary
    (`internal/cli/runs/inspect.go`).

## 7. SSE Contract

The event hub (`internal/stream/hub.go`) and HTTP wrapper (`internal/stream/http.go`)
implement a minimal SSE protocol used by the Mods endpoints.

- Event types:
  - `"log"` — `LogRecord {timestamp, stream, line}`.
  - `"retention"` — `RetentionHint {retained, ttl, expires_at, bundle_cid}`.
  - `"ticket"` — `mods/api.TicketSummary`.
  - `"done"` — `Status {status:"done"}` sentinel; the stream is finished and the
    hub closes subscribers.

- Clients:
  - `internal/cli/stream.Client` uses `Last-Event-ID` and backoff to resume and
    retry streams.
  - `internal/cli/mods.EventsCommand` handles `"ticket"` and `"stage"` events
    (from higher-level publishers) and ignores unknown types to remain
    forwards-compatible.
  - `internal/cli/runs.FollowCommand` and `ploy mods logs` focus on `"log"` and
    `"retention"` events for human-readable tails.

## 8. References

Code paths most relevant for Mods:

- CLI:
  - `cmd/ploy/mod_run_exec.go`
  - `cmd/ploy/mod_run_spec.go`
  - `cmd/ploy/mod_controlplane_commands.go`
  - `internal/cli/mods/*`
- Control plane:
  - `internal/mods/api/*`
  - `internal/server/handlers/handlers_mods_ticket.go`
  - `internal/server/handlers/handlers_diffs.go`
  - `internal/server/handlers/nodes_complete.go` — job completion
  - `internal/server/handlers/nodes_claim.go` — job claiming
  - `internal/server/events/service.go`
  - `internal/stream/hub.go`, `internal/stream/http.go`
- Database:
  - `internal/store/schema.sql` — single source of truth for database schema (jobs table, float step_index)
  - `internal/store/queries/jobs.sql` — job queries including `ClaimJob` (claims pending jobs)
    and `ScheduleNextJob` (transitions next created job to pending)
- Nodeagent:
  - `internal/nodeagent/execution_orchestrator.go`
  - `internal/nodeagent/execution_healing.go`
  - `internal/workflow/runtime/step/*`
- Graph:
  - `internal/workflow/graph/types.go` — graph node/edge types
  - `internal/workflow/graph/builder.go` — DAG materialization from jobs
  - `ROADMAP_DAG.md` — detailed job graph documentation

For concrete end-to-end scenarios and sample specs see:

- `tests/e2e/mods/README.md`
- `tests/e2e/mods/scenario-orw-pass.sh`
- `tests/e2e/mods/scenario-orw-fail/run.sh`
- `tests/e2e/mods/scenario-multi-step/mod.yaml`
- `tests/e2e/mods/scenario-multi-node-rehydration/run.sh`

## 9. Quick checklist for coding agents

When changing Mods behaviour, prefer these anchors:

- Ticket/status model:
  - Update `internal/mods/api/types.go` (ticket/job types).
  - Wire server handlers in `internal/server/handlers/handlers_mods_*.go`.
  - Keep `docs/mods-lifecycle.md` and `tests/e2e/mods/README.md` in sync.
- SSE/event flow:
  - Use `internal/server/events/service.go` and `internal/stream/*` for hub/SSE.
  - Adjust CLI consumers under `internal/cli/mods` and `internal/cli/runs`.
- Node execution/rehydration:
  - Use `internal/nodeagent/execution_orchestrator.go` plus
    `internal/workflow/runtime/step/*`.
  - Keep `step_index` relationships consistent across jobs and diffs.
- Job scheduling:
  - `ClaimJob` in `internal/store/queries/jobs.sql` only returns `pending` jobs.
  - `ScheduleNextJob` transitions the first `created` job to `pending` after completion.
  - This server-driven model ensures jobs execute in `step_index` order.
