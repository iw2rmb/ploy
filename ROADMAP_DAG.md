# Mods Job Graph (DAG) Documentation

This document visualizes the job directed acyclic graph (DAG) for Mods runs. It
complements `docs/mods-lifecycle.md` by focusing on the graph topology and how
jobs connect across different run scenarios.

## Overview

Each Mods run creates a sequence of **jobs** stored in the `jobs` table. Jobs
are ordered by `step_index` (float values like 1000, 2000, 3000), which determines
execution order. The graph package (`internal/workflow/graph`) materializes these
jobs into an explicit DAG for visualization and debugging.

### Node Types

Jobs map to one of five node types defined in `internal/workflow/graph/types.go`:

| Node Type   | Description                                     | Example Name   |
|-------------|-------------------------------------------------|----------------|
| `pre_gate`  | Pre-mod validation (Build Gate before mod)      | `pre-gate`     |
| `mod`       | Main modification execution                     | `mod-0`, `mod-1` |
| `post_gate` | Post-mod validation (Build Gate after mod)      | `post-gate`    |
| `heal`      | Healing job inserted after gate failure         | `heal-0`, `heal-1` |
| `re_gate`   | Re-validation after healing                     | `re-gate`      |

### Node Statuses

Jobs progress through these states (from `store.JobStatus`):

```
created → pending → running → succeeded | failed | skipped | canceled
```

- **created**: Job exists but is not yet claimable by nodes.
- **pending**: Job is ready to be claimed (`ClaimJob` only returns pending jobs).
- **running**: A node has claimed and is executing the job.
- **succeeded/failed**: Terminal states after execution.
- **skipped**: Job was bypassed (e.g., post-gate when mod fails).
- **canceled**: Job was canceled (e.g., losing branch in parallel healing).

---

## Simple Run DAG

A successful single-mod run without healing creates a linear three-node graph:

```
                    step_index ordering
                    ─────────────────────►

        ┌───────────┐       ┌───────────┐       ┌───────────┐
        │ pre-gate  │──────▶│   mod-0   │──────▶│ post-gate │
        │  (1000)   │       │  (2000)   │       │  (3000)   │
        └───────────┘       └───────────┘       └───────────┘
           pre_gate             mod              post_gate
```

**Execution flow:**

1. **pre-gate (1000)**: Build Gate validates baseline workspace. Must pass before
   mod executes. If it fails without healing configured, run fails immediately.

2. **mod-0 (2000)**: Main modification container runs against validated workspace.
   Produces a diff on success. If exit code ≠ 0, run fails (no post-gate).

3. **post-gate (3000)**: Build Gate validates workspace after mod changes. If it
   fails without healing, run fails. If it passes, run succeeds.

**Database view (jobs table):**
```sql
SELECT name, step_index, status FROM jobs WHERE run_id = $1 ORDER BY step_index;
-- name       | step_index | status
-- pre-gate   |     1000.0 | succeeded
-- mod-0      |     2000.0 | succeeded
-- post-gate  |     3000.0 | succeeded
```

---

## Healing Run DAG

When a Build Gate fails and healing is configured, the system inserts heal and
re-gate jobs between the failed gate and the next job:

### Pre-gate Failure with Healing

```
                      healing window inserted
                    ◄─────────────────────────►

    ┌───────────┐     ┌───────────┐     ┌───────────┐     ┌───────────┐     ┌───────────┐
    │ pre-gate  │────▶│  heal-0   │────▶│  re-gate  │────▶│   mod-0   │────▶│ post-gate │
    │  (1000)   │     │  (1250)   │     │  (1500)   │     │  (2000)   │     │  (3000)   │
    │  FAILED   │     │           │     │  PASSED   │     │           │     │           │
    └───────────┘     └───────────┘     └───────────┘     └───────────┘     └───────────┘
       pre_gate           heal            re_gate             mod            post_gate
```

**Flow:**

1. **pre-gate (1000)**: Fails (build/test errors).
2. **heal-0 (1250)**: Healing mod (e.g., Codex AI) attempts to fix issues.
3. **re-gate (1500)**: Re-runs Build Gate validation on healed workspace.
4. **mod-0 (2000)**: Executes after re-gate passes.
5. **post-gate (3000)**: Final validation.

**step_index midpoints**: Healing jobs are inserted at midpoints to preserve
order. If `pre-gate` is at 1000 and `mod-0` at 2000, the system computes midpoints
like 1250 (heal) and 1500 (re-gate) using `GetAdjacentJobIndices`.

### Post-gate Failure with Multiple Healing Retries

When healing is configured with `retries: 2`, the system can iterate:

```
┌───────────┐   ┌───────────┐   ┌───────────┐   ┌───────────┐   ┌───────────┐   ┌───────────┐
│ pre-gate  │──▶│   mod-0   │──▶│ post-gate │──▶│  heal-0   │──▶│ re-gate-0 │──▶│  heal-1   │──▶ ...
│  (1000)   │   │  (2000)   │   │  (3000)   │   │  (3250)   │   │  (3500)   │   │  (3625)   │
│  PASSED   │   │  PASSED   │   │  FAILED   │   │  PASSED   │   │  FAILED   │   │           │
└───────────┘   └───────────┘   └───────────┘   └───────────┘   └───────────┘   └───────────┘
```

The loop continues until:
- A re-gate passes → run proceeds (or succeeds if post-gate was the last gate).
- Max retries exhausted → run fails with `ErrBuildGateFailed`.

---

## Multi-Mod Run DAG

Runs with `mods[]` array create multiple mod/post-gate pairs:

```
┌───────────┐   ┌───────────┐   ┌───────────┐   ┌───────────┐   ┌───────────┐
│ pre-gate  │──▶│   mod-0   │──▶│post-gate-0│──▶│   mod-1   │──▶│post-gate-1│
│  (1000)   │   │  (2000)   │   │  (3000)   │   │  (4000)   │   │  (5000)   │
└───────────┘   └───────────┘   └───────────┘   └───────────┘   └───────────┘
   pre_gate         mod          post_gate          mod          post_gate
```

**Workspace rehydration**: Each mod step receives a workspace reconstructed from
the base clone plus diffs from all prior steps. See `rehydrateWorkspaceForStep`
in `internal/nodeagent/execution_orchestrator.go`.

---

## Parallel Healing Branches (Phase E)

Phase E introduces multi-strategy healing where multiple healing approaches run
in parallel. Each strategy operates on its own isolated workspace branch:

```
                              ┌─────────────────────────────────────┐
                              │         Parallel Branches           │
                              │                                     │
                        ┌─────┴─────┐                         ┌─────┴─────┐
                        │ Branch A  │                         │ Branch B  │
                        │ (codex)   │                         │ (patcher) │
                        └─────┬─────┘                         └─────┬─────┘
                              │                                     │
┌───────────┐   ┌───────────┐ │ ┌───────────┐   ┌───────────┐  │    │ ┌───────────┐   ┌───────────┐
│ pre-gate  │──▶│   mod-0   │─┼▶│ heal-a-0  │──▶│ re-gate-a │──┼────┼▶│ heal-b-0  │──▶│ re-gate-b │
│  (1000)   │   │  (2000)   │ │ │  (1500)   │   │  (1600)   │  │    │ │  (1700)   │   │  (1800)   │
│  PASSED   │   │  PASSED   │ │ └───────────┘   └───────────┘  │    │ └───────────┘   └───────────┘
└───────────┘   └───────────┘ │       │               │        │    │       │               │
                              │       └───────┬───────┘        │    │       └───────┬───────┘
                 post-gate────┘               │                │    │               │
                  FAILED                      ▼                │    │               ▼
                                        (first pass           │    │         (canceled)
                                         = winner)            │    │
                                              │                │    │
                                              └────────────────┴────┴────────────────┐
                                                                                     │
                                                                                     ▼
                                                                              ┌───────────┐
                                                                              │ post-gate │
                                                                              │  (3000)   │
                                                                              │continues  │
                                                                              └───────────┘
```

**Branch topology**:

- **Branch A (codex)**: `heal-a-0 (1500) → re-gate-a (1600)`
- **Branch B (patcher)**: `heal-b-0 (1700) → re-gate-b (1800)`

**Winner selection** (`E4` in ROADMAP.md):

1. Branches execute concurrently (subject to node availability).
2. First branch whose `re-gate` passes wins.
3. Losing branches are canceled (`status=canceled`).
4. Winner's diffs are preserved for mainline continuation.

**step_index allocation**: Each branch gets a distinct window to avoid overlap:
- Branch A: 1500-1600
- Branch B: 1700-1800

This ensures deterministic ordering when branches are flattened for display.

**Spec example (multi-strategy)**:

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

---

## Graph Materialization

The graph is materialized on-demand from `jobs` rows using
`internal/workflow/graph/builder.go`:

```go
// BuildFromJobs constructs the graph from existing jobs.
// Phase 1: Create nodes from jobs (ID, name, type, status, step_index).
// Phase 2: Compute edges by deriving parent/child from step_index order.
graph := graph.BuildFromJobs(runID, jobs)
```

**Edge computation** (`ComputeEdges`):

- Nodes are sorted by `step_index`.
- Each node's parent is the immediately preceding node.
- Linear graphs: every node has ≤1 parent and ≤1 child.
- Branching graphs: parallel healing creates nodes with multiple children.

**Graph properties**:

| Property   | Description                                          |
|------------|------------------------------------------------------|
| `RunID`    | Ticket/run UUID                                      |
| `Nodes`    | Map of job ID → `GraphNode`                          |
| `RootIDs`  | Entry points (typically `pre-gate`)                  |
| `LeafIDs`  | Terminal nodes (typically final `post-gate`)         |
| `Linear`   | `true` if no branching (all nodes have ≤1 child)     |

---

## Implementation References

| Concept                  | Location                                           |
|--------------------------|----------------------------------------------------|
| Node/graph types         | `internal/workflow/graph/types.go`                 |
| Graph builder            | `internal/workflow/graph/builder.go`               |
| Job scheduling           | `internal/store/queries/jobs.sql` (`ClaimJob`, `ScheduleNextJob`) |
| Healing job creation     | `internal/server/handlers/nodes_complete_healing.go` |
| Parallel branch planner  | `internal/server/handlers/nodes_complete.go`       |
| Workspace rehydration    | `internal/nodeagent/execution_orchestrator.go`     |
| Lifecycle documentation  | `docs/mods-lifecycle.md`                           |

---

## Summary

The Mods job DAG starts simple (three-node linear chain) and grows dynamically:

1. **Simple run**: `pre-gate → mod → post-gate` (linear).
2. **Healing run**: Gate failure inserts `heal → re-gate` (linear with extensions).
3. **Multi-mod run**: Multiple `mod → post-gate` pairs in sequence.
4. **Parallel healing** (Phase E): Concurrent branches race to fix failures; first
   passing re-gate wins.

The graph is materialized from `jobs` rows using `step_index` ordering, enabling
visualization and debugging without additional persistence.
