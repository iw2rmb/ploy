# Mods Lifecycle and Architecture

This document is the canonical reference for how Mods runs are represented and
executed across the CLI, control plane, and node agents. It replaces the older
checkpoint notes in the repository.

## 1. Core Concepts

- **Run** — A Mods run submitted to the control plane. Runs are stored as
  `runs` rows in PostgreSQL and exposed via the `/v1/runs` API.
- **Job** — A unit of work inside a  run (for example `pre-gate`, `mod-0`,
  `post-gate`). Jobs are stored as `jobs` rows. Persisted job fields are
  `job_type`, `job_image`, and `next_id` (successor link in the job chain).
- **StepIndex** — Float index metadata used by parts of the runtime payloads and
  diagnostics. It is no longer a dedicated `jobs` table column.
- **Spec** — YAML/JSON file or inline JSON describing container image,
  command, env, Build Gate and optional `mods[]` steps. Parsed by the CLI in
  `cmd/ploy/mod_run_spec.go`.
- **Build Gate** — Validation pass run via Docker containers to ensure the
  workspace compiles/tests successfully. The `GateExecutor` adapter
  (`internal/workflow/step`) abstracts execution; nodes claim gate jobs
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
> `run_repos` entry. See § 1.4 (Batched Mods Runs) for the batch model
> (`runs` + `run_repos`) and how single-repo runs fit into the unified architecture.

When the spec does **not** contain a `mods[]` array (single-step run using
top-level `image`/`command`/`env`), the execution sequence is:

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
jobs and claimed by nodes using queue eligibility + `next_id` successor links:

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
1. Control plane creates gate jobs in the `jobs` table with status `Queued`.
2. Node agent claims the next queued job via `/v1/nodes/{id}/claim`.
3. For gate jobs, the Docker gate executor runs validation in a local container.
4. Gate results are captured as `BuildGateStageMetadata` and returned to the orchestrator.
5. For healing flows: re-gate runs against the workspace with accumulated changes.

**Key characteristics:**
- Single unified queue: gate, mod, and healing jobs all use the same `jobs` table.
- Local Docker execution: gates run on the node that claims the job.
- Chain progression via `next_id`: ensures sequential pre-gate → mod → post-gate flow.

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
- `Metadata["gate_summary"]` in run status responses.
Gate summary is exposed in `RunSummary.Metadata["gate_summary"]` and can be consumed via
`GET /v1/runs/{id}/status` or CLI surfaces such as `ploy run status`.

### Stack Gate failures

Stack Gate enforces stack expectations at gate boundaries. When failures occur:
1. No healing attempted (policy failures, not build failures)
2. Error stored in `run_repos.last_error`, shown in CLI follow output
3. Includes: phase, expected/detected, evidence (paths/keys only)

### Router and healing configuration

When the Build Gate fails and healing is configured, the node agent runs an
optional **router** (to summarize the failure) followed by a **healing** loop
(to fix the failure). Both are specified under `build_gate`:

```yaml
build_gate:
  enabled: true

  # Router runs once after gate failure to produce bug_summary.
  router:
    image: docker.io/user/mods-codex:latest
    env:
      CODEX_PROMPT: "Summarize the build failure in /in/build-gate.log as JSON: {\"bug_summary\":\"...\"}"
    env_from_file:
      CODEX_AUTH_JSON: ~/.codex/auth.json

  # Healing runs after router, retrying up to `retries` times.
  healing:
    retries: 2
    image: docker.io/user/mods-codex:latest
    env:
      CODEX_PROMPT: "Fix the compilation error in /in/build-gate.log"
    env_from_file:
      CODEX_AUTH_JSON: ~/.codex/auth.json
```

Healing fields (image, command, env, retain_container) are specified directly
under `healing` — there is no nested `mod` key.

**Router** runs once per gate failure that triggers healing (each iteration),
before the corresponding healing attempt. It reads `/in/build-gate.log` and writes a JSON one-liner to
`/out/codex-last.txt` containing `{"bug_summary":"..."}`. The bug_summary
(max 200 chars, single-line) is persisted in `jobs.meta.gate.bug_summary`.
Router is required when healing is configured.

**Healing** semantics:

- **Single workspace**: Healing runs on the same workspace that the failing gate validated.
- **Linear execution**: The healing mod runs, then the gate is re-run.
- **Retries**: If the gate still fails, the healing mod may be retried up to `retries`.
- **Exhaustion handling**: If all retries are exhausted and the gate still fails, the run fails.
- **action_summary**: After each healing iteration, the agent reads `/out/codex-last.txt`
  for `{"action_summary":"..."}` (max 200 chars, single-line). This is persisted in
  `jobs.meta.action_summary` for mod jobs.

### Per-iteration artifacts and healing log

During the heal → re-gate loop, the node agent writes per-iteration artifacts
to `/in` for debugging and cross-iteration context:

| Artifact | Description |
|---|---|
| `/in/build-gate.log` | Latest gate failure log (updated after each re-gate) |
| `/in/build-gate-iteration-N.log` | Gate failure log snapshot for iteration N |
| `/in/healing-iteration-N.log` | Healing agent output log for iteration N |
| `/in/healing-log.md` | Cumulative markdown log across all iterations |

The `healing-log.md` format:

```markdown
# Healing Log

## Iteration 1

- Bug Summary: Missing semicolon on line 42 of Main.java
  Build Log: /in/build-gate-iteration-1.log
- Healing Attempt: Added missing semicolon to Main.java:42
  Agent Log: /in/healing-iteration-1.log

## Iteration 2
...
```

Implementation references:

- Type definitions: `internal/nodeagent/run_options.go` (`HealingConfig`, `HealingMod`, `RouterConfig`).
- Spec parsing: `internal/workflow/contracts/mods_spec_parse.go` (`parseHealingSpec`, `parseRouterSpec`).
- Spec conversion: `internal/nodeagent/run_options.go` (`modsSpecToRunOptions`).
- Router/healing execution: `internal/nodeagent/execution_healing.go` (`runGateWithHealing`).
- Summary parsing: `internal/nodeagent/execution_healing.go` (`parseBugSummary`, `parseActionSummary`).
- Metadata types: `internal/workflow/contracts/build_gate_metadata.go` (`BugSummary`),
  `internal/workflow/contracts/job_meta.go` (`ActionSummary`).
- Schema example: `docs/schemas/mod.example.yaml`.

### Workspace and rehydration semantics

This subsection clarifies which code version each Build Gate sees during execution.
Understanding workspace state is essential for debugging gate failures and reasoning
about multi-step runs where diffs accumulate across steps.

**Implementation reference:**
- `internal/nodeagent/execution_orchestrator.go` — `executeRun` and `rehydrateWorkspaceForStep`.

#### Pre-mod gate workspace

The **pre-mod gate** runs on the **initial hydrated workspace** (step 0). This workspace
is created by cloning the repository at `base_ref`
and contains no modifications from any mods. The pre-mod gate validates that the baseline
code compiles and tests pass before any mods execute.

Workspace state for pre-mod gate:
```
base_ref → fresh clone → pre-mod gate
```

#### Post-mod gate workspace

Each **post-mod gate** runs on the **rehydrated workspace for that step**. The workspace
reflects all changes from prior mods (steps 0 through k-1) plus the changes from the
current mod (step k).

Before `mod[k]` executes, `rehydrateWorkspaceForStep` reconstructs the workspace for
step k from:

1. **Base clone**: A cached copy of the initial repository state (base_ref).
2. **Ordered diffs**: Diffs from steps 0 through k-1 fetched from the control plane,
   sorted deterministically by chain position, then `(created_at, id)` in the node agent, and
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
- Each step uploads its diff (tagged with `job_id` and optional summary metadata) after successful execution.
- `rehydrateWorkspaceForStep` fetches diffs for steps `0..k-1` before executing step `k`.
- A baseline commit is created after rehydration (via `ensureBaselineCommitForRehydration`)
  so that `git diff HEAD` produces only the changes from step k, not cumulative changes.

#### Summary table

| Gate Phase     | Workspace State                                      | Code Reference                              |
|----------------|------------------------------------------------------|---------------------------------------------|
| Pre-mod gate   | Fresh clone of base_ref                              | `rehydrateWorkspaceForStep` with stepIndex=0 |
| Post-mod gate[k] | Base clone + diffs[0..k-1] + mod[k] changes         | `rehydrateWorkspaceForStep` with stepIndex=k |

### Implementation references

- Gate executor: `internal/workflow/step/gate_docker.go` (`GateExecutor`).
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

The `image` field (top-level, in `mods[]`, and in `build_gate.healing`/`build_gate.router`) accepts two forms:

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

### Example: Parameterized OpenRewrite via rewrite.yml

When OpenRewrite recipes require parameters, you can generate a `rewrite.yml`
config as a code change, then let the stack-aware ORW Mods apply it.

1. **Generate rewrite.yml with mod-shell** (scripts live in the repo):

```yaml
mods:
  - name: generate-rewrite-config
    image: docker.io/user/mods-shell:latest
    env:
      MOD_SHELL_SCRIPT: ./generate-rewrite.sh
```

`./generate-rewrite.sh` runs inside `/workspace` and writes a complete
`rewrite.yml` to the repo root, for example:

```yaml
type: specs.openrewrite.org/v1beta/recipe
name: PloyApplyYaml
recipeList:
  - org.openrewrite.java.migrate.UpgradeToJava17
  # options: ...
```

2. **Apply OpenRewrite using the YAML recipe name**:

```yaml
mods:
  - name: generate-rewrite-config
    image: docker.io/user/mods-shell:latest
    env:
      MOD_SHELL_SCRIPT: ./generate-rewrite.sh
  - name: apply-openrewrite
    image:
      java-maven: docker.io/user/mods-orw-maven:latest
      java-gradle: docker.io/user/mods-orw-gradle:latest
    env:
      RECIPE_GROUP: org.openrewrite.recipe
      RECIPE_ARTIFACT: rewrite-migrate-java
      RECIPE_VERSION: 3.20.0
      RECIPE_CLASSNAME: org.openrewrite.java.migrate.UpgradeToJava17
```

The ORW Mods honor an existing `rewrite.yml` in the workspace:

- `rewrite.configLocation` points to `rewrite.yml`.
- `rewrite.activeRecipes` defaults to the top-level `name:` in `rewrite.yml`
  (or `REWRITE_ACTIVE_RECIPES` if provided).
- If no `rewrite.yml` exists, ORW Mods fall back to running the class recipe
  directly using `RECIPE_CLASSNAME` and the artifact coordinates.

### Implementation references

- Image type and resolution: `internal/workflow/contracts/mod_image.go`
  (`ModImage`, `ResolveImage`, `ParseModImage`, `ToolToModStack`).
- Stack propagation: `internal/workflow/contracts/build_gate_metadata.go`
  (`BuildGateStageMetadata.Tool`).
- Image resolution in executor: `internal/nodeagent/run_options.go`.
- Unit tests: `internal/workflow/contracts/mod_image_test.go`.

## 1.3 Job Order (DAG)

Mods runs form a directed acyclic graph (DAG) of jobs linked through `next_id`
successor pointers. Healing updates this chain by rewiring links in a single
transaction.

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
└───────────┘       └───────────┘       └───────────┘
```

### Healing run graph

When a gate fails with healing configured, heal and re-gate jobs are inserted
by rewiring `next_id` links:

```
┌───────────┐     ┌───────────┐     ┌───────────┐     ┌───────────┐     ┌───────────┐
│ pre-gate  │────▶│  heal-0   │────▶│  re-gate  │────▶│   mod-0   │────▶│ post-gate │
│  FAILED   │     │           │     │  PASSED   │     │           │     │           │
└───────────┘     └───────────┘     └───────────┘     └───────────┘     └───────────┘
```

Rewire example:
- Before failure handling: `failed.next_id = old_next`
- After insertion: `failed.next_id = heal.id`, `heal.next_id = re_gate.id`, `re_gate.next_id = old_next`

### Parallel healing branches (Phase E)

Multi-strategy healing creates concurrent branches with independent chain segments.
The first branch whose re-gate passes wins; losing branches are cancelled:

```
                           ┌─────────────────────────────────────┐
                           │         Parallel Branches           │
                     ┌─────┴─────┐                         ┌─────┴─────┐
                     │ Branch A  │                         │ Branch B  │
                     └─────┬─────┘                         └─────┬─────┘
                           │                                     │
post-gate  ───────────────▶├─▶ heal-a → re-gate-a ───────────────┤
 FAILED                    │                                     │
                           └─▶ heal-b → re-gate-b ───────────────┘
                                                                 │
                                              (first pass wins) ─┘
```

### Implementation references

- Job ordering/claim semantics: `internal/store/queries/jobs.sql`, `internal/nodeagent/claimer.go`
- Healing job insertion: `internal/server/handlers/nodes_complete_healing.go`

## 1.4 Batched Mods Runs (`runs` + `run_repos`)

This section describes how batch runs coordinate multiple repositories under a
single specification. A batch run allows executing the same mod workflow across
many repos without submitting separate  runs for each.

### Conceptual model

Batched runs use a single `runs` row with per-repo `run_repos` rows. Jobs (and all
artifacts) remain job-addressed via `job_id`.

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           Batch Run Model                                   │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│   ┌─────────────┐          ┌──────────────┐          ┌──────────────┐       │
│   │    runs     │──────────│  run_repos   │──────────│     jobs     │       │
│   │ (run-level) │  1 : N   │ (per repo)   │  1 : N   │ (per step)   │       │
│   └─────────────┘          └──────────────┘          └──────────────┘       │
│         │                        │                         │                │
│         │                        │                         │                │
│   ┌─────▼─────┐            ┌─────▼─────┐             ┌─────▼─────┐          │
│   │   spec    │            │ repo_url  │             │   jobs    │          │
│   │   name    │            │ base_ref  │             │  diffs    │          │
│   │  status   │            │ target_ref│             │   logs    │          │
│   │           │            │  status   │             │ artifacts │          │
│   └───────────┘            │  attempt  │             └───────────┘          │
│                            └───────────┘                                    │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

- **Run (`runs`)** — Stores a run referencing `mod_id` + `spec_id` and run-level
  status (`Started`, `Finished`, `Cancelled`). Per-repo execution lives in `run_repos`.

- **Specs (`specs`)** — Append-only spec JSON dictionary. Runs and mods reference
  a spec by ID.

- **Repo set (`mod_repos`)** — Managed repositories for a mod project, each with
  current `repo_url`, `base_ref`, and `target_ref`.

- **Run repos (`run_repos`)** — One row per `(run_id, repo_id)` capturing snapshot
  `repo_base_ref`/`repo_target_ref`, per-repo status (`Queued`, `Running`, `Success`,
  `Fail`, `Cancelled`), and retry `attempt`.

- **Jobs (`jobs`)** — Jobs are scoped to `(run_id, repo_id, attempt)`; logs/diffs/artifacts
  attach to `job_id`. There are no per-repo child runs in v1.

### Single-repo vs batch runs

Single-repo submission via `ploy run --repo ... --base-ref ... --target-ref ... --spec ...`
(or via `ploy mod run --repo-url ... --spec ...`) is
internally a **degenerate batch** with exactly one `run_repos` entry. The same
code paths handle both cases:

| Aspect         | Single-repo run                 | Batch run                               |
|----------------|----------------------------------|-----------------------------------------|
| Run (`runs`)   | Created (`Started`)              | Created (`Started`)                     |
| `mod_repos`    | 1 repo created/managed           | N repos created/managed                 |
| `run_repos`    | 1 (auto-created)                 | N (added via batch creation / repo add) |
| Spec storage   | `specs` referenced by `runs.spec_id` | Same                                |

### State machines

#### Run derived status (v1)

The control plane exposes a run-level derived status from `run_repos` counts (`RunRepoCounts.derived_status`):

```
	         ┌─────────────────────────────────────────────────────────┐
	         │                Batch Derived Status                      │
	         ├─────────────────────────────────────────────────────────┤
	         │                                                         │
	         │    ┌─────────┐                                          │
	         │    │ pending │  (initial; no repos running yet)         │
	         │    └────┬────┘                                          │
	         │         │ first run_repo transitions to 'running'       │
	         │         ▼                                               │
	         │    ┌─────────┐                                          │
	         │    │ running │  (at least one repo is active)           │
	         │    └────┬────┘                                          │
	         │         │ all run_repos reach terminal state            │
	         │         ▼                                               │
	         │    ┌──────────────────────────────────┐                 │
	         │    │ completed │ failed │ cancelled │                  │
	         │    └──────────────────────────────────┘                 │
	         │    (aggregate: any cancelled → cancelled,               │
	         │     any failed → failed, else completed)                │
	         │                                                         │
	         └─────────────────────────────────────────────────────────┘
```

#### Run repo state machine (v1)

Each `run_repos` row tracks execution for a single repository within a run:

`Queued` → `Running` → (`Success` | `Fail` | `Cancelled`)

- `Queued` is created on run submission / repo add.
- The repo transitions to `Running` when the first job for that `(run_id, repo_id, attempt)` is claimed.
- Terminal status is set when the repo’s last job finishes, or when cancelled.

### Jobs pipeline within a batch (v1)

Jobs are stored directly in `jobs` and scoped to `(run_id, repo_id, attempt)`.
The first job for a repo attempt is `Queued`, and later jobs are `Created`. Healing may
insert `heal-*` + `re-gate-*` jobs by rewiring `next_id` links.

### Batch scheduler (v1)

The background scheduler ensures queued repos have jobs and promotes the next job for a
repo attempt. It does not create per-repo child runs.

### Relationship summary (v1)

| Table       | Purpose                                    | Key relationships                         |
|-------------|--------------------------------------------|-------------------------------------------|
| `specs`     | Append-only spec dictionary                | referenced by `mods.spec_id`, `runs.spec_id` |
| `mods`      | Mod projects                               | `mods` → `mod_repos` (1:N), `mods` → `runs` (1:N) |
| `mod_repos` | Managed repo set for a mod                 | `mod_repos` → `run_repos` (1:N), `mod_repos` → `jobs` (1:N) |
| `runs`      | Run record                                 | `runs` → `run_repos` (1:N), `runs` → `jobs` (1:N) |
| `run_repos` | Per-repo execution state within a run      | `(run_id, repo_id)` → `jobs` (1:N)        |
| `jobs`      | Execution units (pre-gate, mod, heal, etc.)| `jobs` → `diffs`/`logs`/artifacts via `job_id` |

### Pulling Diffs Locally (`run pull` / `mod pull`)

The `ploy run pull <run-id>` and `ploy mod pull` commands enable developers to reconstruct
Mods-generated changes in their local git repository. This is useful for reviewing,
testing, or continuing work on changes produced by a run without relying on MR-based
workflows.

**High-level sequence:**

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        pull Workflow (v1)                                   │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  1. Resolve repo context                                                    │
│     ├─ Get origin URL from `git remote get-url <origin>`                    │
│     ├─ (run pull) Call POST /v1/runs/{run_id}/pull with repo_url             │
│     └─ (mod pull) Optionally infer mod via GET /v1/mods?repo_url=...         │
│               then call POST /v1/mods/{mod_id}/pull with repo_url + mode     │
│                                                                             │
│  2. Fetch base snapshot                                                      │
│     ├─ Call GET /v1/runs/{run_id}/repos and find repo_id                     │
│     ├─ Use run_repos.base_ref snapshot                                       │
│     └─ git fetch <origin> <base_ref> --depth=1                               │
│                                                                             │
│  3. Create branch                                                            │
│     ├─ Use repo_target_ref (branch name snapshot)                            │
│     ├─ Check no local/remote collision for repo_target_ref                   │
│     ├─ git branch <target_ref> FETCH_HEAD                                    │
│     └─ git checkout <target_ref>                                             │
│                                                                             │
│  4. Apply diffs                                                              │
│     ├─ Call GET /v1/runs/{run_id}/repos/{repo_id}/diffs to list diffs        │
│     ├─ For each diff (ordered by chain position):                           │
│     │   ├─ Download via GET /v1/runs/{run_id}/repos/{repo_id}/diffs?download=true&diff_id=<uuid> │
│     │   ├─ Stream-decompress gzipped patch                                  │
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
  that matches the `repo_url` stored in `mod_repos` / `run_repos` (see "Repo URL rules" below).

**Key fields used:**

| Field                       | Source                          | Purpose                                   |
|----------------------------|---------------------------------|-------------------------------------------|
| `repo_id`                  | API / `POST /v1/*/pull`         | Identify the repo within the run          |
| `repo_target_ref`          | API / `POST /v1/*/pull`         | Target branch name snapshot               |
| `run_repos.base_ref`       | API / `GET /v1/runs/{run_id}/repos` | Base ref snapshot for branch base     |
| `diffs.summary.step_index` | diffs summary JSON              | Optional legacy step metadata for display |
| `diffs.id`                 | API / `GET /v1/runs/.../diffs`   | UUID used as `diff_id` for download       |

**API endpoints consumed:**

- `POST /v1/runs/{run_id}/pull` — Resolve `repo_id` + `repo_target_ref` for the current repo within the run.
- `POST /v1/mods/{mod_id}/pull` — Resolve `run_id` + `repo_id` + `repo_target_ref` for the current repo within the selected run.
- `GET /v1/runs/{run_id}/repos` — Fetch run repo snapshots (used to read `base_ref`).
- `GET /v1/runs/{run_id}/repos/{repo_id}/diffs` — List diffs for the repo execution within a run.
- `GET /v1/runs/{run_id}/repos/{repo_id}/diffs?download=true&diff_id=<uuid>` — Download gzipped patch content.

**Download size limits (CLI):**

- The CLI streams and gunzips diff downloads (no “read-all then gunzip-all”) using `internal/cli/httpx.GunzipToBytes`.
- The decompressed patch is capped at 256 MiB (`httpx.MaxGunzipOutputBytes`) to mitigate gzip “zip bombs”.
- Diff IDs are validated as UUIDs (`internal/domain/types.DiffID`) when decoded from API responses and when constructing download requests.

**Repo URL rules:**

Repo URL matching uses the shared `vcs.NormalizeRepoURL` helper (see `internal/vcs/repourl.go`):
- Normalization: trim whitespace, strip trailing `/` and `.git` suffix.
- Matching (server): compare normalized strings; no URL parsing is performed.
The CLI derives `repo_url` from the git remote URL; the server performs normalized matching
to select the correct `run_repos` entry.

The CLI validates `repo_url` using `internal/domain/types.RepoURL` (allowed schemes: `https://`, `ssh://`, `file://`) when:
- the user provides a repo URL explicitly (submit, batch create, `mod repo add`, `mod run --repo`), and
- the CLI derives `repo_url` from a git remote for pull commands (`run pull`, `mod pull`).

If your git remote uses SCP-like syntax (example: `git@github.com:org/repo.git`), change it to an allowed form (example: `ssh://git@github.com/org/repo.git`) or use an HTTPS remote.

**Example usage:**

```bash
# After a run completes:
cd /path/to/service-a

# Run-based pull:
ploy run pull <run-id>

# Mod-based pull:
ploy mod pull <mod-id|name>

# Preview without making changes:
ploy run pull --dry-run <run-id>

# Use a different remote:
ploy run pull --origin upstream <run-id>
```

See `cmd/ploy/README.md` § "Pull Mods Changes Locally" for CLI reference.

### Implementation references

- Run submission + repo add: `internal/server/handlers/runs_submit.go`, `internal/server/handlers/runs_batch_http.go`.
- Run repos queries: `internal/store/queries/run_repos.sql`.
- Batch scheduler: `internal/store/batchscheduler/batch_scheduler.go`.
- CLI subcommands: `cmd/ploy/mod_run_repo.go`.
- Schema: `internal/store/schema.sql` (see `runs`, `run_repos`, `jobs` tables).

## 2. Data Model

### 2.1 Run summary (`internal/mods/api`)

`RunSummary` is the **canonical wire type** for Mods run status. It is the single
response schema for:

- `GET /v1/runs/{id}/status` (status) — 200 response body.
- `event: run` SSE payloads on `/v1/runs/{id}/logs`.

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
| `stages`     | map[JobID]StageStatus       | Job execution states keyed by job ID (JSON keys are job ID strings). |

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
- Use `next_id` within each `StageStatus` to follow successor links.
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
| `last_error`    | string (optional)   | Error message from the most recent failed attempt. Includes explicit `exit code 137` OOM-kill hints for killed mod jobs. |
| `next_id`       | string (optional)   | Successor job ID from `jobs.next_id`; null for chain tail jobs. |

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
      "next_id": "2NQPoBfVkc8dFmGAQqJnUwMu9jT"
    },
    "2NQPoBfVkc8dFmGAQqJnUwMu9jT": {
      "state": "running",
      "next_id": "2NQPoBfVkc8dFmGAQqJnUwMu9jU"
    },
    "2NQPoBfVkc8dFmGAQqJnUwMu9jU": {
      "state": "pending",
      "next_id": null
    }
  }
}
```

### 2.2 Jobs and diffs

- **Jobs** (`jobs` table)
  - Created by the control plane when a run is submitted via `POST /v1/runs`.
		- Each job row has:
		    - `id` — job ID (KSUID string, used as key in `RunSummary.stages`).
		    - `name` — job name (e.g., `pre-gate`, `mod-0`, `post-gate`).
	    - `next_id` — successor job ID for chain progression (`null` for tail jobs).
		  - `status` — job status in the database (`Created`, `Queued`, `Running`, `Success`, `Fail`, `Cancelled`).
		    - `RunSummary.stages[*].state` is the external API representation (`pending`, `running`, `succeeded`, `failed`, `cancelled`).
		    - `node_id` — which node claimed this job.
	    - `job_type` — job phase (`pre_gate`, `mod`, `post_gate`, `heal`, `re_gate`, `mr`).
	    - `job_image` — container image name for this job (persisted by the node for mod/heal/gate jobs).
		    - `meta` — JSONB with structured job metadata (optional; see `internal/workflow/contracts.JobMeta`).
  - Dynamic insertion rewires explicit successor links:
    - Initial chain: `pre-gate -> mod-0 -> post-gate`.
    - Healing insertion updates `failed.next_id` to `heal`, then links healing tail to the former successor.

	- **Server-driven scheduling**
		  - Jobs are created with status `Created` (not yet claimable) or `Queued`
		    (ready to claim). The first job (`pre-gate`) is created as `Queued`.
		  - `ClaimJob` (`internal/store/queries/jobs.sql`) only returns `Queued`
		    jobs. This ensures nodes cannot claim jobs until the server decides they
		    are ready.
		  - When a job completes successfully, the server promotes that job's
		    `next_id` successor from `Created` to `Queued` (when present).
		  - This model enforces sequential execution: `pre-gate` → `mod-0` → `post-gate`.
		  - Healing jobs follow the same pattern: heal jobs are created with status
		    `Queued` to be claimed immediately after insertion.

- **Diffs**
  - Generated by the workflow runtime (`internal/workflow/step`) and
    uploaded by nodeagents using `/v1/runs/{run_id}/jobs/{job_id}/diff`.
  - Exposed via:
	    - `GET /v1/runs/{run_id}/repos/{repo_id}/diffs` (`internal/server/handlers/diffs.go`)
	      — returns a list of diffs with `job_id` and summary metadata, ordered by
	      producing job chain position, then `created_at` (ascending).
	    - `GET /v1/runs/{run_id}/repos/{repo_id}/diffs?download=true&diff_id=<uuid>` — returns the gzipped unified diff.
	  - Diffs are applied in chain order for rehydration.

### 2.3 Artifacts

- Nodeagents upload artifact bundles with:
  - `POST /v1/runs/{run_id}/jobs/{job_id}/artifact`.
  - Control plane exposes bundles per run via:
    - `GET /v1/artifacts` and `GET /v1/artifacts/{id}` for listing/downloading
      by CID/id.
- `StageStatus.Artifacts` map keys are human-readable names; values are bundle
  CIDs.

## 3. Control Plane HTTP Surfaces

### 3.1 Mods endpoints (`internal/server/handlers`)

- `POST /v1/runs` — submit a single-repo Mods run.
  - Shape: `{repo_url, base_ref, target_ref, spec, created_by?}`.
  - Handler: `createSingleRepoRunHandler`.
  - Behaviour (single source of truth for Mods execution):
    - Creates a spec (`specs`), a mod project (`mods`), a managed repo (`mod_repos`),
      a run (`runs`, `status=Started`), a run repo (`run_repos`, `status=Queued`),
      and the repo-scoped `jobs` pipeline (first job `Queued`, later jobs `Created`).
    - The run repo transitions to `Running` when the first job is claimed.
    - Publishes an initial `RunSummary` snapshot via `events.Service.PublishRun`
      (this run_id is used by SSE, diffs, and logs APIs).

- `GET /v1/runs/{id}/status` — run status.
  - Handler: `getRunStatusHandler`.
  - Aggregates:
    - `runs` row.
    - `jobs` rows (including `meta` JSONB with job metadata).
    - Artifact bundles per job.
    - Run stats (MR URL, gate summary).
  - Returns `RunSummary` directly (Go type `modsapi.RunSummary`); the canonical JSON shape for run state.

- `GET /v1/runs/{id}/logs` — SSE event stream for a run's logs and status.
  - Handler: `getRunLogsHandler`.
  - Uses the internal hub (`internal/stream`) and events service to stream:
    - `event: log`, data: `LogRecord {timestamp,stream,line,node_id,job_id,job_type,step_index}` (see § 7.2).
    - `event:  run`, data: `RunSummary`.
    - `event: retention`, data: `RetentionHint`.
    - `event: done`, data: `Status {status:"done"}` sentinel.
  - Supports `Last-Event-ID` for resumption.

- `POST /v1/runs/{id}/cancel` — cancel a run.
  - Handler: `cancelRunHandlerV1`.
  - Behaviour:
    - Transitions run to `Cancelled`.
    - Updates repos in `Queued|Running` to `Cancelled`.
    - Updates jobs in `Created|Queued|Running` to `Cancelled`.

- `GET /v1/runs/{run_id}/repos/{repo_id}/diffs` and `GET /v1/runs/{run_id}/repos/{repo_id}/diffs?download=true&diff_id=<uuid>` — diff list and download.
  - Handler: `listRunRepoDiffsHandler` (download mode is query-driven).
  - Enable node and CLI callers to enumerate and fetch per-step diffs for a repo execution.

- `POST /v1/runs/{id}/logs`, `POST /v1/runs/{id}/diffs`,
  `POST /v1/runs/{run_id}/jobs/{job_id}/artifact`, `POST /v1/runs/{run_id}/jobs/{job_id}/diff` —
  write endpoints used by nodeagents to persist logs, diffs, and artifacts.

### 3.2 Node endpoints (`internal/server/handlers/register.go`)

Nodeagents use `/v1/nodes/*` to execute work:

- `POST /v1/nodes/{id}/heartbeat` — report node liveness.
- `POST /v1/nodes/{id}/claim` — claim the next queued job from the unified
  jobs queue (returns the claimed job plus run
  metadata) and marks the repo as `Running` in `run_repos`.
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

- `GET /v1/runs` — list batch runs with basic metadata (mod_id, spec_id, status, timestamps) and optional per-repo status counts.
- `GET /v1/runs/{id}` — inspect a single batch run with aggregated repo counts from `run_repos`.
- `POST /v1/runs/{id}/cancel` — cancel a batch run by transitioning the run to `Cancelled` and marking `Queued`/`Running` `run_repos` as `Cancelled`, and cancelling/removing waiting jobs from the queue (idempotent for terminal runs). The CLI maps this to `ploy run stop <run-id>` and returns the canonical `RunSummary` payload.

## 4. Node Execution and Rehydration

### 4.1 Single-step runs

For a spec without `mods[]` (single-step top-level `image`/`command`/`env`):

1. CLI (`ploy mod run`) builds a `RunSubmitRequest` in
   `cmd/ploy/mod_run_exec.go` and an optional spec JSON payload in
   `cmd/ploy/mod_run_spec.go`.
2. CLI submits to `POST /v1/runs`. The control plane:
   - Creates jobs (pre-gate, mod, post-gate) as a `next_id`-linked chain.
   - Publishes an initial `RunSummary` over SSE.
3. A node:
   - Claims jobs via `/v1/nodes/{id}/claim` (jobs are claimed from a unified queue; within a repo attempt, the server promotes the next job only after prior jobs succeed).
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
2. `POST /v1/runs`:
   - Creates jobs for pre-gate, each mod, and post-gates as a linked chain.
   - Each job row includes `job_type` (pre_gate, mod, post_gate, heal, re_gate)
     and `job_image` (saved by the executing node before the container starts).
3. Scheduler and nodeagents:
   - ClaimJob returns queued jobs from the unified queue, and the server promotes
     the claimed job's `next_id` successor only after prior jobs succeed.
   - Execute each job against a workspace that reflects all prior steps.

Workspace rehydration is implemented in `internal/nodeagent/execution_orchestrator.go`:

- `rehydrateWorkspaceForStep`:
  - Copies the base clone (base_ref).
  - Applies diffs for prior jobs in order using `git apply`.
  - Diffs are fetched via `GET /v1/runs/{run_id}/repos/{repo_id}/diffs`, ordered by chain position.

- `ensureBaselineCommitForRehydration`:
  - After applying prior diffs, creates a local commit that becomes the new
    `HEAD`.
  - Ensures that `git diff HEAD` after the job produces an **incremental**
    patch containing only changes from that job.
  - Control plane stores per-job diffs under the job's `job_id`.

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
    - per-iteration gate logs (`/in/build-gate-iteration-N.log`),
    - per-iteration healing logs (`/in/healing-iteration-N.log`),
    - cumulative healing log (`/in/healing-log.md`),
    - Codex session state (`/in/codex-session.txt`),
    - prompt files (`/in/prompt.txt`), etc.

- **Environment**
  - Spec `env` and `env_from_file` are resolved and merged by
    `buildSpecPayload`.
    - `env_from_file` paths are resolved on the CLI side and injected as string
      values.
    - Supported on:
      - top-level spec (single-step runs),
      - each `mods[]` entry (multi-step runs),
      - `build_gate.healing` and `build_gate.router`.
  - **Global env injection**: The control plane injects server-configured global
    environment variables at job claim time via `mergeGlobalEnvIntoSpec()`. Global
    env vars are filtered by scope (`all`, `mods`, `heal`, `gate`) to match job types:
    - `all` → injected into every job
    - `mods` → `mod` and `post_gate` jobs
    - `heal` → `heal` and `re_gate` jobs
    - `gate` → `pre_gate`, `re_gate`, and `post_gate` jobs
    The job spec must be a JSON object; invalid/non-object specs are rejected at submission
    time (400). If a persisted spec in the DB is invalid or non-object, claim fails with a 500.
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
    - `run_repos.last_error` (for `exit_code=137`, includes a "killed; likely out of memory" message),
    - `metadata["reason"]` where available,
    - Build Gate summary (if the failure happened in the gate).

- **Retention**
  - `retain_container` in the spec causes the node runtime
    (`internal/workflow/step` and `internal/nodeagent`) to skip
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
  - Optional `--follow` displays a summarized per-repo job graph until completion,
    implemented via `internal/cli/follow.Engine`. The job graph refreshes on
    SSE events from `/v1/runs/{id}/logs` but does not stream container logs.
    Use `ploy run logs <run-id>` to stream logs.

- `ploy mod run <mod-id|name>`:
  - Creates a run from a mod project via `cmd/ploy/mod_run_project.go`.
  - Supports `--repo`, `--failed` for repo selection.
  - Optional `--follow` displays the job graph until completion.

- `ploy pull`:
  - Local repo pull workflow via `cmd/ploy/pull.go`.
  - Ensures a run exists for the current HEAD SHA and pulls diffs.
  - Optional `--follow` displays the job graph and proceeds to pull diffs.
  - `--dry-run` prints planned actions and does not initiate a run or save pull state.
  - Maintains per-repo pull state in `<git-dir>/ploy/pull_state.json`.

- `ploy run logs <run-id>`:
  - Streams logs/events from `/v1/runs/{id}/logs`, focusing on `log` and
    `retention` events (see `internal/cli/mods/logs.go`).
  - This is the canonical surface for streaming container stdout/stderr.

- Run summaries (gate/MR/job graph) are exposed via:
  - `ploy run status <run-id>` (CLI)
  - `GET /v1/runs/{id}/status` (HTTP)

## 7. SSE Contract

The event hub (`internal/stream/hub.go`) and HTTP wrapper (`internal/stream/http.go`)
implement a minimal SSE protocol used by the Mods endpoints.

**OpenAPI reference:** See `docs/api/paths/runs_id_logs.yaml` for the formal
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
| `job_type`   | string | Mods step type: `pre_gate`, `mod`, `post_gate`, `heal`, `re_gate`      |
| `step_index` | number | Optional step metadata used for log enrichment                          |

**Example SSE frame:**

```
event: log
data: {"timestamp":"2025-10-22T10:00:00Z","stream":"stdout","line":"Step started","node_id":"aB3xY9","job_id":"2NQPoBfVkc8dFmGAQqJnUwMu9jR","job_type":"mod","step_index":2000}
```

**Notes:**

- Enriched fields may be empty for events not tied to a specific job (e.g.,
  hub-generated system events) or when context is unavailable.
- `step_index` in logs is optional metadata and does not drive scheduler ordering.
- CLI consumers (`ploy run logs`) use the enriched fields
  to display contextual information in structured output format.

### 7.3 Clients

- `internal/cli/stream.Client` uses `Last-Event-ID` and backoff to resume and
  retry streams.
- `internal/cli/mods.EventsCommand` handles `"run"` and `"stage"` events
  (from higher-level publishers) and ignores unknown types to remain
  forwards-compatible.
- `internal/cli/runs.FollowCommand` and `ploy run logs` focus on `"log"` and
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
  - `internal/store/schema.sql` — single source of truth for database schema (`jobs.next_id` chain model)
  - `internal/store/queries/jobs.sql` — job queries including `ClaimJob` (claims `Queued` jobs) and `ScheduleNextJob` (transitions next `Created` job to `Queued`)
- Nodeagent:
  - `internal/nodeagent/execution_orchestrator.go`
  - `internal/nodeagent/execution_healing.go`
  - `internal/workflow/step/*`

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
    `internal/workflow/step/*`.
  - Keep `next_id` chain relationships consistent across jobs and diffs.
- Job scheduling:
  - `ClaimJob` in `internal/store/queries/jobs.sql` only returns `Queued` jobs.
  - `ScheduleNextJob` transitions the next chain successor from `Created` to `Queued` after completion.
  - This server-driven model ensures jobs execute in chain order.
