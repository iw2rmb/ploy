# Mods Lifecycle and Architecture

This document is the canonical reference for how Mods runs are represented and
executed across the CLI, control plane, and node agents. It replaces the older
checkpoint notes in the repository.

## 1. Core Concepts

- **Run** — A Mods run submitted to the control plane. Runs are stored as
  `runs` rows in PostgreSQL and exposed via the `/v1/mods` API.
- **Job** — A unit of work inside a  run (for example `pre-gate`, `mod-0`,
  `post-gate`). Jobs are stored as `jobs` rows.
- **StepIndex** — Float index that orders jobs and ties them to diffs. Jobs use
  float step_index (e.g., 1000, 2000, 3000) to allow dynamic insertion of
  healing jobs between existing jobs.
- **Spec** — YAML/JSON file or inline JSON describing container image,
  command, env, Build Gate and optional `mods[]` steps. Parsed by the CLI in
  `cmd/ploy/mod_run_spec.go`.
- **Build Gate** — Validation pass run via Docker containers to ensure the
  workspace compiles/tests successfully. The `GateExecutor` adapter
  (`internal/workflow/runtime/step`) abstracts execution; nodes claim gate jobs
  from the unified queue and execute them locally. Gates run at two distinct points
  in the lifecycle:
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

> **Note:** A single-repo submission is internally a degenerate batch with one
> `run_repos` entry. See § 1.4 (Batched Mods Runs) for the parent/child run
> model and how single-repo runs fit into the unified architecture.

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

### Gate execution via unified jobs queue

Pre-gate and re-gate validation runs through the `GateExecutor` adapter as part of
the unified jobs pipeline. Gate jobs are stored in the `jobs` table alongside mod
jobs and claimed by nodes in FIFO order by `step_index`:

```
┌─────────────────────┐     ┌────────────────────┐     ┌───────────────────────┐
│ Node Orchestrator   │     │ GateExecutor       │     │ Docker Container      │
│ (execution_healing) │────▶│ (docker adapter)   │────▶│ (local execution)     │
└─────────────────────┘     └────────────────────┘     └───────────────────────┘
                                                                │
                                                                ▼
                                                       ┌───────────────────────┐
                                                       │ BuildGateStage        │
                                                       │ Metadata returned     │
                                                       │ (passed/failed)       │
                                                       └───────────────────────┘
```

**Flow:**
1. Control plane creates gate jobs in the `jobs` table with status `pending`.
2. Node agent claims the next pending job via `/v1/nodes/{id}/claim`.
3. For gate jobs, the Docker gate executor runs validation in a local container.
4. Gate results are captured as `BuildGateStageMetadata` and returned to the orchestrator.
5. For healing flows: re-gate runs against the workspace with accumulated changes.

**Key characteristics:**
- Single unified queue: gate, mod, and healing jobs all use the same `jobs` table.
- Local Docker execution: gates run on the node that claims the job.
- FIFO ordering by `step_index`: ensures sequential pre-gate → mod → post-gate flow.

See `docs/build-gate/README.md` for gate configuration and execution details.

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
- `ploy mod inspect < run-id>` output as `Gate: passed|failed ...`.

### Healing configuration

The healing configuration requires the canonical `strategies[]` schema:

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

Each strategy defines a named healing branch with its own `mods[]` list. The
control plane creates parallel branches for each strategy, and the first branch
whose re-gate passes wins.

**Single-strategy example** — For a single healing approach, use one strategy:
```yaml
build_gate_healing:
  retries: 1
  strategies:
    - name: codex-fix
      mods:
        - image: docker.io/user/mods-codex:latest
          command: mod-codex --input /workspace --out /out
```

**Note:** Legacy single-strategy form (`build_gate_healing.mods[]` at top level)
is no longer supported. All healing configurations must use the `strategies[]`
schema.

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

- Gate executor: `internal/workflow/runtime/step/gate_docker.go` (`GateExecutor`).
- Gate+healing orchestration: `internal/nodeagent/execution_healing.go`.
- Run orchestration: `internal/nodeagent/execution_orchestrator.go` (`executeRun`).
- Workspace rehydration: `internal/nodeagent/execution_orchestrator.go` (`rehydrateWorkspaceForStep`).
- Stats aggregation: `internal/domain/types/runstats.go` (`GateSummary()`).
- **Build Gate configuration**: See `docs/build-gate/README.md` for gate configuration
  and Docker-based execution details.

## 1.2 Stack-Aware Image Selection

Mods supports stack-aware image selection, allowing different container images to be
used based on the detected build stack. The image field accepts two canonical forms:
universal images (string) for simple configurations, and stack-specific images (map)
for optimized per-build-tool containers (e.g., dedicated Maven or Gradle images).

### Image specification forms

The `image` field (top-level, in `mods[]`, and in `build_gate_healing.strategies[].mods[]`) accepts two forms:

**Universal image (string)** — A single image used regardless of stack:
```yaml
image: docker.io/user/mods-openrewrite:latest
```

**Stack-specific images (map)** — Different images per detected stack:
```yaml
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

## 1.4 Batched Mods Runs (`runs` + `run_repos`)

This section describes how batch runs coordinate multiple repositories under a
single specification. A batch run allows executing the same mod workflow across
many repos without submitting separate  runs for each.

### Conceptual model

Batched runs introduce a parent–child relationship between tables:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          Batch Run Hierarchy                                │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│   ┌─────────────┐          ┌──────────────┐          ┌──────────────┐       │
│   │  runs (P)   │──────────│  run_repos   │──────────│  runs (C)    │       │
│   │  (parent)   │  1 : N   │  (mapping)   │  1 : 1   │  (child)     │       │
│   └─────────────┘          └──────────────┘          └──────────────┘       │
│         │                        │                         │                │
│         │                        │                         │                │
│   ┌─────▼─────┐            ┌─────▼─────┐             ┌─────▼─────┐          │
│   │   spec    │            │ repo_url  │             │   jobs    │          │
│   │   name    │            │ base_ref  │             │  diffs    │          │
│   │  status   │            │ target_ref│             │   logs    │          │
│   │           │            │  status   │             │ artifacts │          │
│   └───────────┘            │  attempt  │             └───────────┘          │
│                            │ execution │                                    │
│                            │ _run_id   │                                    │
│                            └───────────┘                                    │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

- **Parent run (`runs`)** — Stores the shared specification (`spec` JSONB),
  optional batch name, and aggregate status. The parent holds no per-repo
  details; those live in `run_repos`.

- **Run repos (`run_repos`)** — Mapping table that attaches repositories to a
  parent run. Each row captures:
  - `repo_url`, `base_ref`, `target_ref` — repository coordinates.
  - `status` — per-repo execution state (`pending`, `running`, `succeeded`,
    `failed`, `skipped`, `cancelled`).
  - `attempt` — retry counter; incremented on `restart`.
  - `execution_run_id` — foreign key to the child `runs` row that holds the
    actual job pipeline for this repo.

- **Child run (`runs`)** — Created when a `run_repo` transitions from `pending`
  to `running`. The child inherits the parent's `spec` and owns its own `jobs`
  rows (pre-gate, mod, post-gate, heal, re-gate). Logs, diffs, and artifacts
  are stored against the child run.

### Single-repo vs batch runs

A single-repo submission via `ploy mod run --repo-url ... --spec ...` is
internally a **degenerate batch** with exactly one `run_repos` entry. The same
code paths handle both cases:

| Aspect           | Single-repo run              | Batch run                              |
|------------------|------------------------------|----------------------------------------|
| Parent run       | Created with `repo_url`      | Created with optional `name`, no repo  |
| `run_repos` rows | 1 (auto-created)             | 0 initially; added via `repo add`      |
| Child runs       | 1 (linked by `execution_run_id`) | 1 per `run_repo`                   |
| Spec storage     | On parent; inherited by child| Same                                   |

### State machines

#### Parent run state machine

The parent run aggregates status from its `run_repos` entries:

```
         ┌─────────────────────────────────────────────────────────┐
         │                  Parent Run Status                      │
         ├─────────────────────────────────────────────────────────┤
         │                                                         │
         │    ┌─────────┐                                          │
         │    │ queued  │  (initial; no repos running yet)         │
         │    └────┬────┘                                          │
         │         │ first run_repo transitions to 'running'       │
         │         ▼                                               │
         │    ┌─────────┐                                          │
         │    │ running │  (at least one repo is active)           │
         │    └────┬────┘                                          │
         │         │ all run_repos reach terminal state            │
         │         ▼                                               │
         │    ┌──────────────────────────────────┐                 │
         │    │ succeeded │ failed │ canceled   │                  │
         │    └──────────────────────────────────┘                 │
         │    (aggregate: all succeeded → succeeded,               │
         │     any failed → failed, else canceled)                 │
         │                                                         │
         └─────────────────────────────────────────────────────────┘
```

#### Run repo state machine

Each `run_repos` row tracks individual repository progress:

```
         ┌───────────────────────────────────────────────────────────────┐
         │                   Run Repo Status                             │
         ├───────────────────────────────────────────────────────────────┤
         │                                                               │
         │    ┌─────────┐    scheduler picks up repo                     │
         │    │ pending │ ─────────────────────────────┐                 │
         │    └────┬────┘                              │                 │
         │         │                                   ▼                 │
         │         │              ┌─────────────────────────────────┐    │
         │         │              │ Create child run + jobs        │    │
         │         │              │ Link via execution_run_id      │    │
         │         │              └────────────────┬────────────────┘    │
         │         │                               │                     │
         │         │                               ▼                     │
         │         │                         ┌─────────┐                 │
         │         │                         │ running │                 │
         │         │                         └────┬────┘                 │
         │         │                              │                      │
         │         │         ┌────────────────────┼──────────────────┐   │
         │         │         │                    │                  │   │
         │         ▼         ▼                    ▼                  ▼   │
         │    ┌─────────┐ ┌─────────┐       ┌──────────┐      ┌─────────┐│
         │    │ skipped │ │succeeded│       │  failed  │      │cancelled││
         │    └─────────┘ └─────────┘       └──────────┘      └─────────┘│
         │                                                               │
         └───────────────────────────────────────────────────────────────┘
```

### Jobs pipeline within a batch

Each `run_repo` that transitions to `running` spawns its own child run with a
complete jobs pipeline. The pipeline follows the same logic described in
§ 1.1 (Build Gate Sequence):

```
  run_repos[0] → child_run_0 → jobs: pre-gate → mod-0 → post-gate
  run_repos[1] → child_run_1 → jobs: pre-gate → mod-0 → post-gate
  ...
```

Child runs execute independently. There is no cross-repo ordering within a
batch—repos may complete in any order depending on node availability and
execution time.

### Batch scheduler

The `batchscheduler` package (`internal/store/batchscheduler/batch_scheduler.go`)
automatically starts pending repos:

1. Polls for parent runs with `run_repos` in `pending` status.
2. For each pending repo, creates a child run and links it via
   `execution_run_id`.
3. Transitions the `run_repo` to `running`.
4. When the child run completes, a completion callback updates the `run_repo`
   status to the child's terminal state.

### CLI workflow for batched runs

In a batch workflow, `ploy mod run` submits the spec once, then
`ploy mod run repo add` attaches multiple repositories under the same run via
`run_repos`:

```bash
# 1. Create a batch run with a shared spec (no repo attached yet).
ploy mod run --spec mod.yaml --name my-batch

# 2. Add repos to the batch.
ploy mod run repo add --repo-url https://github.com/org/repo1.git \
    --base-ref main --target-ref feature-branch my-batch
ploy mod run repo add --repo-url https://github.com/org/repo2.git \
    --base-ref main --target-ref feature-branch my-batch

# 3. Monitor per-repo status within the batch.
ploy mod run repo status my-batch

# 4. Optionally restart a failed repo with updated refs.
# Repo IDs are NanoID(8) strings (e.g., "a1b2c3d4").
ploy mod run repo restart --repo-id <repo-id> --base-ref hotfix my-batch

# 5. Remove a repo from the batch (marks pending as skipped, running as cancelled).
ploy mod run repo remove --repo-id <repo-id> my-batch
```

### Relationship summary

| Table        | Purpose                                        | Key Relationships                     |
|--------------|------------------------------------------------|---------------------------------------|
| `runs`       | Stores spec + status for parent or child runs  | Parent→run_repos (1:N), Child→jobs    |
| `run_repos`  | Maps repos to a parent run; tracks per-repo state | run_repos→parent (N:1), →child (1:1)|
| `jobs`       | Execution units (pre-gate, mod, post-gate, etc.) | jobs→child_run (N:1)                |
| `diffs`      | Per-job workspace patches                      | diffs→child_run, diffs→job            |
| `logs`       | Execution logs                                 | logs→child_run, logs→job              |

### Pulling Diffs Locally (`mod run pull`)

The `ploy mod run pull` command enables developers to reconstruct Mods-generated changes
in their local git repository. This is useful for reviewing, testing, or continuing work
on changes produced by a batch run without relying on MR-based workflows.

**High-level sequence:**

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        mod run pull Workflow                                │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  1. Resolve run + repo                                                      │
│     ├─ Get origin URL from `git remote get-url origin`                      │
│     ├─ Call GET /v1/repos/{repo_id}/runs to list runs for this repo         │
│     └─ Match <run-name|run-id> to find RepoRunSummary                       │
│                                                                             │
│  2. Fetch execution details                                                 │
│     ├─ Use RepoRunSummary.execution_run_id to identify the child run        │
│     └─ Call GET /v1/mods/{execution_run_id} to get commit_sha, target_ref   │
│                                                                             │
│  3. Verify commit reachability                                              │
│     └─ git fetch <origin> <commit_sha> --depth=1                            │
│                                                                             │
│  4. Create branch                                                           │
│     ├─ Check no local/remote collision for target_ref                       │
│     ├─ git branch <target_ref> <commit_sha>                                 │
│     └─ git checkout <target_ref>                                            │
│                                                                             │
│  5. Apply diffs                                                             │
│     ├─ Call GET /v1/mods/{execution_run_id}/diffs to list diffs             │
│     ├─ For each diff (ordered by step_index):                               │
│     │   ├─ Download via GET /v1/diffs/{id}?download=true                    │
│     │   ├─ Decompress gzipped patch                                         │
│     │   └─ git apply (skip empty patches)                                   │
│     └─ Print success summary                                                │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

**Preconditions enforced by the CLI:**

- **Inside git worktree**: The command must be run from within a git repository.
- **Clean working tree**: No staged or unstaged changes allowed (prevents data loss
  and ensures deterministic patch application).
- **Resolvable remote**: The specified `--origin` remote must exist and have a URL
  that matches the `repo_url` stored in `run_repos`.

**Key fields used:**

| Field                            | Source                     | Purpose                                    |
|----------------------------------|----------------------------|--------------------------------------------|
| `run_repos.repo_url`             | API / run_repos table      | Match against local origin URL             |
| `run_repos.execution_run_id`     | API / run_repos table      | Link to child run with diffs               |
| `runs.commit_sha`                | API / runs table           | Pinned commit for branch creation          |
| `run_repos.target_ref`           | API / run_repos table      | Branch name for the reconstructed changes  |
| `diffs.step_index`               | API / diffs table          | Order diffs for correct application        |

**API endpoints consumed:**

- `GET /v1/repos/{repo_id}/runs` — List runs for the repository (repo_id is URL-encoded
  origin URL).
- `GET /v1/mods/{id}` — Fetch run details including `commit_sha`.
- `GET /v1/mods/{id}/diffs` — List diffs for the execution run.
- `GET /v1/diffs/{id}?download=true` — Download gzipped patch content.

**Normalization:**

The CLI normalizes the local origin URL by stripping trailing slashes and `.git` suffixes
for comparison purposes. However, the raw URL is used for API calls to ensure exact
matching against stored `run_repos.repo_url` values.

**Example usage:**

```bash
# After a batch run completes:
cd /path/to/service-a
ploy mod run pull java17-fleet

# Preview without making changes:
ploy mod run pull --dry-run java17-fleet

# Use a different remote:
ploy mod run pull --origin upstream java17-fleet
```

See `cmd/ploy/README.md` § "Pull Mods Changes Locally" for CLI reference.

### Implementation references

- Parent/child run creation: `internal/server/handlers/runs_batch_http.go`.
- Run repos queries: `internal/store/queries/run_repos.sql`.
- Batch scheduler: `internal/store/batchscheduler/batch_scheduler.go`.
- CLI subcommands: `cmd/ploy/mod_run_repo.go`.
- Schema: `internal/store/schema.sql` (see `runs`, `run_repos`, `jobs` tables).

## 2. Data Model

### 2.1 Run summary (`internal/mods/api`)

`RunSummary` is the **canonical wire type** for Mods run status. It is the single
response schema for:

- `POST /v1/mods` (submit) — 201 response body.
- `GET /v1/mods/{id}` (status) — 200 response body.
- `event: run` SSE payloads on `/v1/mods/{id}/events`.

**Wire contract guarantees:**

- No wrapper types — `RunSummary` is returned directly as the JSON root.
- Field names are stable and match `internal/mods/api/types.go` exactly.
- All IDs use KSUID format (27 characters, lexicographically sortable).

**OpenAPI reference:** See `docs/api/components/schemas/controlplane.yaml#/RunSummary`
for the formal schema definition.

#### RunSummary fields

| Field        | Type                        | Description                                       |
|--------------|-----------------------------|---------------------------------------------------|
| `run_id`     | string (KSUID)              | Unique run identifier (27 characters).            |
| `state`      | string (enum)               | Lifecycle state: `pending`, `running`, `succeeded`, `failed`, `cancelling`, `cancelled`. |
| `submitter`  | string (optional)           | Submitter identifier (e.g., user email).          |
| `repository` | string                      | Git repository URL.                               |
| `metadata`   | map[string]string           | Additional diagnostics (see below).               |
| `created_at` | string (RFC 3339)           | Run creation timestamp.                           |
| `updated_at` | string (RFC 3339)           | Timestamp of the latest status update.            |
| `stages`     | map[string]StageStatus      | Job execution states keyed by job ID.             |

**Metadata keys:**

- `repo_base_ref`, `repo_target_ref`: Git refs used for this run.
- `node_id`: ID of the node that claimed/executed the run.
- `mr_url`: Merge request URL when available (GitLab/GitHub).
- `gate_summary`: Build Gate health summary from run stats.
- `reason`: Terminal failure/cancellation reason when available.
- `resume_count`, `last_resumed_at`: Resume history when present.

#### stages map semantics

The `stages` field is a map keyed by **job ID** (`jobs.id`, KSUID string). Each
value is a `StageStatus` object describing that job's execution state.

**Key semantics:**

- Keys are job IDs (KSUID strings), **not** job names or step indices.
- Use `step_index` within each `StageStatus` for ordering jobs in multi-step runs.
- Typical entries: `pre-gate`, `mod-0`, `post-gate` jobs, plus dynamically inserted
  `heal-*` and `re-gate` jobs for healing flows.

#### StageStatus fields

| Field           | Type                | Description                                         |
|-----------------|---------------------|-----------------------------------------------------|
| `state`         | string (enum)       | Job state: `pending`, `queued`, `running`, `succeeded`, `failed`, `cancelling`, `cancelled`. |
| `attempts`      | int                 | Number of execution attempts for this job.          |
| `max_attempts`  | int                 | Maximum allowed attempts.                           |
| `current_job_id`| string (optional)   | Execution job ID (may differ in retry scenarios).   |
| `artifacts`     | map[string]string   | Artifact logical names → bundle CIDs.               |
| `last_error`    | string (optional)   | Error message from the most recent failed attempt.  |
| `step_index`    | int                 | Float index from `jobs.step_index` for ordering.    |

**step_index values:**

- Pre-gate: 1000
- First mod: 2000
- Post-gate: 3000
- Healing jobs: midpoints (e.g., 1500, 1750)

#### Example response

```json
{
  "run_id": "2NQPoBfVkc8dFmGAQqJnUwMu9jR",
  "state": "running",
  "repository": "https://github.com/org/repo.git",
  "metadata": {
    "repo_base_ref": "main",
    "repo_target_ref": "feature-branch",
    "node_id": "aB3xY9"
  },
  "created_at": "2025-01-15T10:00:00Z",
  "updated_at": "2025-01-15T10:05:00Z",
  "stages": {
    "2NQPoBfVkc8dFmGAQqJnUwMu9jS": {
      "state": "succeeded",
      "step_index": 1000
    },
    "2NQPoBfVkc8dFmGAQqJnUwMu9jT": {
      "state": "running",
      "step_index": 2000
    },
    "2NQPoBfVkc8dFmGAQqJnUwMu9jU": {
      "state": "pending",
      "step_index": 3000
    }
  }
}
```

### 2.2 Jobs and diffs

- **Jobs** (`jobs` table)
  - Created by the control plane when a run is submitted via `POST /v1/mods`.
  - Each job row has:
    - `id` — job ID (KSUID string, used as key in `RunSummary.stages`).
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
  - Control plane exposes bundles per  run:
    - `POST /v1/mods/{id}/artifact_bundles`.
    - `GET /v1/artifacts` and `GET /v1/artifacts/{id}` for listing/downloading
      by CID/id.
- `StageStatus.Artifacts` map keys are human-readable names; values are bundle
  CIDs.

## 3. Control Plane HTTP Surfaces

### 3.1 Mods endpoints (`internal/server/handlers`)

- `POST /v1/mods` — submit a Mods run.
  - Simplified shape: `{repo_url, base_ref, target_ref, commit_sha?, spec?, created_by?}`.
  - Handler: `submitRunHandler`.
  - Behaviour:
    - Creates a `runs` row with `status=queued`.
    - Creates `jobs` rows from the spec (pre-gate, mod, post-gate).
    - Jobs use float step_index for ordering (1000, 2000, 3000).
    - Publishes an initial `RunSummary` snapshot to SSE via
      `events.Service.PublishRun`.

- `GET /v1/mods/{id}` — run status.
  - Handler: `getRunStatusHandler`.
  - Aggregates:
    - `runs` row.
    - `jobs` rows (including `meta` JSONB with job metadata).
    - Artifact bundles per job.
    - Run stats (MR URL, gate summary).
  - Returns `RunSummary` directly (Go type `modsapi.RunSummary`); the canonical JSON shape for run state.

- `GET /v1/mods/{id}/events` — SSE event stream for a run.
  - Handler: `getModEventsHandler`.
  - Uses the internal hub (`internal/stream`) and events service to stream:
    - `event: log`, data: `LogRecord {timestamp,stream,line,node_id,job_id,mod_type,step_index}` (see § 7.2).
    - `event:  run`, data: `RunSummary`.
    - `event: retention`, data: `RetentionHint`.
    - `event: done`, data: `Status {status:"done"}` sentinel.
  - Supports `Last-Event-ID` for resumption.

- `POST /v1/mods/{id}/cancel` — cancel a run.
  - Handler: `cancelRunHandler`.
  - Behaviour:
    - Transitions run to `canceled`, updates jobs in `pending|running` to
      `canceled`.
    - Publishes a final `RunSummary` with `state=cancelled`.
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
- `POST /v1/nodes/{id}/claim` — claim the next pending job from the unified
  jobs queue (FIFO by `step_index`; returns the claimed job plus parent run
  metadata). This endpoint now transitions the parent run to `running` when
  still queued and publishes SSE "running" events for real-time client updates.
  (The separate `/v1/nodes/{id}/ack` endpoint has been removed.)
- `POST /v1/jobs/{job_id}/complete` — report final status and stats for a job
  (canonical endpoint; node-based `/v1/nodes/{id}/complete` has been removed).
- `POST /v1/nodes/{id}/logs` — upload gzipped log chunks.
- `POST /v1/runs/{run_id}/jobs/{job_id}/diff` — upload per-job diffs.
- `POST /v1/runs/{run_id}/jobs/{job_id}/artifact` — upload per-job artifacts.
- Legacy HTTP Build Gate endpoints (`/v1/nodes/{id}/buildgate/*`) have been
  removed; gate execution now runs as jobs in the unified queue claimed via
  `/v1/nodes/{id}/claim`. See `docs/build-gate/README.md` for gate configuration,
  unified jobs behavior, and a brief historical note on the removed HTTP mode.

All mutating requests from worker nodes (POST/PUT/DELETE) must include the
`PLOY_NODE_UUID` header set to the node's ID (NanoID(6) string). The
control plane uses this header to validate job ownership and attribute
artifacts/diffs to the correct node.

### 3.3 Runs endpoints (`internal/server/handlers/runs_batch_http.go`)

- `GET /v1/runs` — list batch runs with basic metadata (repo_url, refs, status, timestamps) and optional per-repo status counts.
- `GET /v1/runs/{id}` — inspect a single batch run with aggregated repo counts from `run_repos`.
- `POST /v1/runs/{id}/stop` — stop a batch run by transitioning the run to `canceled` and marking pending `run_repos` as `cancelled` (idempotent for terminal runs).

## 4. Node Execution and Rehydration

### 4.1 Single-step runs

For a spec without `mods[]` (single `mod` or legacy top-level image):

1. CLI (`ploy mod run`) builds a `RunSubmitRequest` in
   `cmd/ploy/mod_run_exec.go` and an optional spec JSON payload in
   `cmd/ploy/mod_run_spec.go`.
   2. CLI submits to `POST /v1/mods`. The control plane:
   - Creates jobs (pre-gate, mod, post-gate) with float step_index.
   - Publishes an initial `RunSummary`.
3. A node:
   - Claims jobs sequentially via `/v1/nodes/{id}/claim` (ClaimJob enforces
     dependency: only returns a job when all prior jobs succeeded/skipped).
   - For each claimed job:
     - Hydrates the workspace using `step.WorkspaceHydrator`.
     - Executes the job (gate check or mod container).
     - Generates diffs with `DiffGenerator` and uploads them.
     - Completes the job via `/v1/jobs/{job_id}/complete`.
4. Control plane updates  run status and emits a final `run` snapshot plus
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
  - **Global env injection**: The control plane injects server-configured global
    environment variables at job claim time via `mergeGlobalEnvIntoSpec()`. Global
    env vars are filtered by scope (`all`, `mods`, `heal`, `gate`) to match job types:
    - `all` → injected into every job
    - `mods` → `mod` and `post_gate` jobs
    - `heal` → `heal` and `re_gate` jobs
    - `gate` → `pre_gate`, `re_gate`, and `post_gate` jobs
  - **Precedence**: Per-run env (spec or CLI flags) wins over global env—existing
    keys are never overwritten.
  - **Common global vars**: `CA_CERTS_PEM_BUNDLE`, `CODEX_AUTH_JSON`, `OPENAI_API_KEY`.
    See `docs/envs/README.md` § "Global Env Configuration" for full details.

- **Execution**
  - Entry point should read/modify the repo under `/workspace`.
  - Output artifacts, logs and plans should be written under `/out`.
  - Exit code `0` signals success. Non-zero exit code is treated as failure and
    surfaces in:
    -  run `state=failed`,
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
  - Constructs `RunSubmitRequest` with stage definitions in
    `cmd/ploy/mod_run_exec.go`.
  - Submits via `internal/cli/mods.SubmitCommand`.
  - Optional `--follow` streams  run events via
    `internal/cli/mods.EventsCommand`, backed by `internal/cli/stream`.

- `ploy mods logs < run>`:
  - Streams logs from `/v1/mods/{id}/events`, focusing on `log` and
    `retention` events (see `cmd/ploy/mods_jobs_commands.go` and
    `internal/cli/runs/follow.go`).

- `ploy runs inspect < run>`:
  - Calls `GET /v1/mods/{id}` and prints a concise summary
    (`internal/cli/runs/inspect.go`).

## 7. SSE Contract

The event hub (`internal/stream/hub.go`) and HTTP wrapper (`internal/stream/http.go`)
implement a minimal SSE protocol used by the Mods endpoints.

**OpenAPI reference:** See `docs/api/paths/mods_id_events.yaml` for the formal
endpoint specification and event payload schemas.

### 7.1 Event types

| Event Type   | Payload Schema       | Description                                    |
|--------------|----------------------|------------------------------------------------|
| `run`        | `RunSummary`         | Run lifecycle snapshot (state changes)         |
| `log`        | `LogRecord`          | Enriched log line with execution context       |
| `retention`  | `RetentionHint`      | Artifact retention metadata                    |
| `done`       | `Status`             | Terminal sentinel; stream closes after this    |

**`event: run`** — Canonical `RunSummary` payload (see § 2.1). Published when run
or job state changes. Clients can poll for the latest snapshot using the staged
map and metadata fields.

### 7.2 LogRecord payload (`event: log`)

Each `event: log` frame carries a JSON-encoded `LogRecord` with both core and
enriched fields. Enriched fields provide execution context so clients can
correlate log lines with specific nodes, jobs, and pipeline stages.

**Core fields (always present):**

| Field       | Type   | Description                                       |
|-------------|--------|---------------------------------------------------|
| `timestamp` | string | RFC 3339 timestamp when the log line was captured |
| `stream`    | string | Output stream (`stdout` or `stderr`)              |
| `line`      | string | Log message content                               |

**Enriched fields (optional, omitempty):**

| Field        | Type   | Description                                                            |
|--------------|--------|------------------------------------------------------------------------|
| `node_id`    | string | Node ID (NanoID 6-character string) that produced this log line        |
| `job_id`     | string | Job ID (KSUID string) that produced this log line                      |
| `mod_type`   | string | Mods step type: `pre_gate`, `mod`, `post_gate`, `heal`, `re_gate`      |
| `step_index` | int    | Float index of the job within the pipeline (e.g., 1000, 2000)          |

**Example SSE frame:**

```
event: log
data: {"timestamp":"2025-10-22T10:00:00Z","stream":"stdout","line":"Step started","node_id":"aB3xY9","job_id":"2NQPoBfVkc8dFmGAQqJnUwMu9jR","mod_type":"mod","step_index":2000}
```

**Notes:**

- Enriched fields may be empty for events not tied to a specific job (e.g.,
  hub-generated system events) or when context is unavailable.
- `step_index` uses float values (1000, 2000, 3000) to allow dynamic insertion
  of healing jobs at midpoints (e.g., 1500 for heal-1).
- CLI consumers (`ploy mods logs`, `ploy runs follow`) use the enriched fields
  to display contextual information in structured output format.

### 7.3 Clients

- `internal/cli/stream.Client` uses `Last-Event-ID` and backoff to resume and
  retry streams.
- `internal/cli/mods.EventsCommand` handles `"run"` and `"stage"` events
  (from higher-level publishers) and ignores unknown types to remain
  forwards-compatible.
- `internal/cli/runs.FollowCommand` and `ploy mods logs` focus on `"log"` and
  `"retention"` events for human-readable tails.
- The shared log printer (`internal/cli/logs`) formats log records using
  enriched fields when available (see "Structured Log Format" below).

## 8. References

Code paths most relevant for Mods:

- CLI:
  - `cmd/ploy/mod_run_exec.go`
  - `cmd/ploy/mod_run_spec.go`
  - `cmd/ploy/mod_controlplane_commands.go`
  - `internal/cli/mods/*`
- Control plane:
  - `internal/mods/api/*`
  - `internal/server/handlers/handlers_mods_ run.go`
  - `internal/server/handlers/handlers_diffs.go`
  - `internal/server/handlers/jobs_complete.go` — job completion (via /v1/jobs/{job_id}/complete)
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

- Run/status model:
  - Update `internal/mods/api/types.go` ( run/job types).
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
