# Mods DAG / Explicit Step Graph

> When following this template:
> - Align to the template structure
> - Include steps to update relevant docs

Scope: Evolve Mods execution from an implicit, per-node loop (gate → heal → re-gate → main) into an explicit control-plane workflow graph (DAG). Represent gates, healing runs, and main mods as first-class steps/nodes with edges, enabling resumability, richer orchestration (branching and merging), and future parallel healing strategies.

Documentation: `docs/mods-lifecycle.md`, `docs/build-gate/README.md`, `docs/schemas/mod.example.yaml`, `cmd/ploy/README.md`, `tests/README.md`, `ROADMAP.md`, `ROADMAP_BG_DECOUPLE.md`.

Legend: [ ] todo, [x] done.

## Phase A — Graph model and persistence
- [ ] Define a control-plane workflow graph model for Mods — Make gates, healing, and main mods explicit nodes.
  - Component: `internal/workflow/contracts`, `internal/store`, `internal/nodeagent`.
  - Scope:
    - Introduce a `WorkflowGraph` / `StepGraph` abstraction in contracts (control plane side) with:
      - Nodes: `id`, `type` (e.g. `gate`, `healing`, `mod`), `stage_id`, `step_index`, `attempt`, etc.
      - Edges: parent/child relationships (dependencies).
      - Status: `pending`, `running`, `succeeded`, `failed`, `canceled`.
    - Extend the store schema to persist this graph per Mods ticket:
      - Either as a normalized `workflow_nodes` / `workflow_edges` schema or as a serialized graph on the ticket (to be decided when implementing).
      - Ensure each node is addressable by `node_id` (logical, not VPS node).
    - Clarify mapping to existing concepts:
      - A “step” in current code (per manifest) becomes at least one node, possibly multiple (gate, heal, main mod).
      - `build_gate_healing` config defines a pattern of nodes to insert after a gate failure.
  - Test: Add unit tests for graph construction in a new package (e.g. `internal/workflow/graph`) to verify that a simple one-step spec with `build_gate` and `build_gate_healing` yields the expected node/edge set.

## Phase B — Graph construction from spec
- [ ] Build an initial linear graph for Mods runs — Represent the original spec as a sequence of nodes before healing/branching.
  - Component: `internal/workflow/planner` (new), `cmd/ploy` (if CLI needs visibility).
  - Scope:
    - Implement a planner that converts the submitted Mods spec into an initial graph:
      - For each `mods[]` entry in the spec:
        - Create a `gate` node (if `build_gate.enabled`).
        - Create a `mod` node (the actual container/image).
        - Connect `gate -> mod` (or just `mod` if no gate).
      - Store this graph on ticket creation.
    - For multi-step scenarios (e.g. `mods[0..N-1]`):
      - Connect `mod[i] -> gate[i+1]` (or `mod[i] -> mod[i+1]` if `gate` disabled for that stage).
  - Test: Add tests under `internal/workflow/planner` that:
    - Given a sample spec (like `tests/e2e/mods/scenario-multi-step/mod.yaml`), the planner outputs the expected sequence of nodes with correct order and types.

- [ ] Encode build_gate_healing as a graph pattern — Describe how gate failure expands the graph.
  - Component: `internal/workflow/planner`, `internal/nodeagent`.
  - Scope:
    - Define a small, reusable pattern for healing:
      - On a `gate` node failure where `build_gate_healing` is configured:
        - Insert a `healing` node (currently one mod, e.g. `mods-codex`).
        - Insert a `re-gate` node after the `healing` node.
      - Wire edges: `gate_failed -> healing -> re-gate -> (continue original successor if re-gate passes)`.
    - Persist a reference from each `healing` node back to the original `gate` node and main `mod` node for this stage so logs and metadata can be correlated.
  - Test: Extend planner tests to:
    - For a spec with `build_gate_healing` and one mod, verify that planned graph includes a healing + re-gate pattern attached to the relevant gate node.

## Phase C — Scheduler and node assignment
- [ ] Make scheduler operate on graph nodes instead of implicit “run with healing” — Each node represents one unit of work.
  - Component: `internal/nodeagent/claimer_spec.go`, `internal/nodeagent/doc.go`, scheduler logic in control plane.
  - Scope:
    - Change Mods execution scheduling to:
      - Select the next `pending` node whose dependencies are all `succeeded`.
      - Assign that node to a worker (existing `claimRunHandler` / scheduler logic may need to be generalized to “claim node”).
    - Ensure nodes have:
      - A `stage_id` / `step_index` that still maps to existing artifact/diff APIs.
      - A type to decide which executor (Mods runner, Build Gate client, healing mod runner) to use.
  - Test: Add scheduler tests confirming:
    - Nodes are claimed in the right order for a simple linear graph.
    - Nodes are not scheduled until dependencies are completed.

- [ ] Introduce node-type-specific executors — Different handlers for gate, healing, and main mods.
  - Component: `internal/nodeagent/execution_orchestrator.go`, `internal/nodeagent/execution_healing.go`, Build Gate client.
  - Scope:
    - Replace `executeWithHealing` monolith with node-type executors:
      - `executeGateNode` — runs Build Gate (local docker or HTTP, depending on `PLOY_BUILDGATE_MODE`).
      - `executeHealingNode` — runs the healing mod (e.g., `mods-codex` with sentinel + session resume).
      - `executeModNode` — runs the main Mods container as today.
    - Node agent selects executor based on node type from the graph.
    - Healing loop becomes:
      - Control plane marks `gate` node failed → planner injects `healing` and `re-gate` nodes per pattern, with dependencies.
      - Scheduler dispatches `healing` node → `executeHealingNode` runs; when done, `re-gate` becomes eligible.
  - Test: Add tests around executor selection and node transitions:
    - `gate` node failure yields new nodes (`healing`, `re-gate`) and status transitions occur as expected.

## Phase D — Resumability and failure recovery
- [ ] Persist node status and allow resuming from last incomplete node — Make failures resumable at node granularity.
  - Component: `internal/store`, `internal/server/handlers`, `cmd/ploy`.
  - Scope:
    - Extend the store model to track per-node status and timestamps.
    - Update existing Mods ticket APIs (`GET /v1/mods/{id}`) to surface:
      - Node list or a condensed summary (e.g. “stage 1: gate→heal→re-gate; stage 2: gate→mod”).
    - Provide a resume mechanism:
      - For a failed ticket, allow `ploy mod resume <ticket-id>` to:
        - Keep the existing graph and node statuses.
        - Re-enable scheduling only for nodes in a `pending` or certain `failed` states (policy to be decided when implementing).
  - Test:
    - Add store tests to ensure node status updates are idempotent and persisted.
    - Add API tests verifying `GET /v1/mods/{id}` includes enough info to reconstruct the path for debugging.

## Phase E — Parallel healing and branching
- [ ] Support parallel healing branches for a failed gate — Run multiple healers on the same failure.
  - Component: `internal/workflow/planner`, `internal/nodeagent`, rehydration logic.
  - Scope:
    - Extend the healing pattern so `build_gate_healing` can optionally define multiple healing strategies (future work on spec shape).
    - For each strategy:
      - Insert a branch: `gate_failed -> healing_i -> re-gate_i`.
      - Each branch uses a separate rehydrated workspace:
        - Rehydrate from base clone + ordered diff chain up to the failing gate, plus branch-local diffs.
    - Define merge semantics:
      - When the first branch’s `re-gate_i` passes:
        - Mark the branch as the “winner”.
        - Option 1: Stop all other branches and discard their diffs.
        - Option 2: Preserve diffs as artifacts but do not apply them to the “main” path.
      - Continue the original linear graph from the winning branch’s workspace.
  - Test:
    - Add planner tests to verify multiple healing branches are represented as separate subgraphs with correct dependencies.
    - Later, add integration tests once multi-branch rehydration is implemented (after the diff/rehydration details are designed).

## Phase F — Docs, CLI UX, and observability
- [ ] Update lifecycle docs and CLI to describe DAG-based Mods execution — Make the model visible and debuggable.
  - Component: `docs/mods-lifecycle.md`, `cmd/ploy/README.md`, `tests/README.md`.
  - Scope:
    - In `docs/mods-lifecycle.md`, replace the implicit healing loop description with:
      - A diagram of the node graph for a simple run (gate→mod) and for a healing run (gate→healing→re-gate→mod).
      - A section on how future parallel healing branches are represented.
    - In `cmd/ploy/README.md`, enhance `ploy mod inspect` / `ploy mod status` docs:
      - Show how to list nodes and their statuses.
      - Provide examples of interpreting gate/healing sequences from the CLI.
  - Test:
    - Run `make test` to ensure CLI and docs tests remain green.
    - Manually run a simple Mods ticket in a dev environment and confirm the CLI surface matches documentation.

