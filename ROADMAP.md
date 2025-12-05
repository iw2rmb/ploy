# Mods DAG / Explicit Step Graph

> When following this template:
> - Align to the template structure
> - Include steps to update relevant docs

Scope: Evolve Mods execution from an implicit, per-node loop (gate → heal → re-gate → main) into an explicit control-plane workflow graph (DAG). Represent gates, healing runs, and main mods as first-class steps/nodes with edges, enabling resumability, richer orchestration (branching and merging), and future parallel healing strategies.

Documentation: `docs/mods-lifecycle.md`, `docs/build-gate/README.md`, `docs/schemas/mod.example.yaml`, `cmd/ploy/README.md`, `tests/README.md`, `ROADMAP.md`, `ROADMAP_BG_DECOUPLE.md`.

Legend: [ ] todo, [x] done.

Current engine: Mods tickets are stored as `runs` rows with per-step `jobs` rows (mod_type `pre_gate`, `mod`, `heal`, `re_gate`, `post_gate`) ordered by float `step_index`. Healing inserts additional `heal` and `re_gate` jobs between existing jobs, and workspace rehydration applies diffs ordered by `step_index`.

## Phase A — Graph model and persistence
- [x] Define a control-plane workflow graph view for Mods — Treat jobs as explicit nodes and expose dependencies on top of existing persistence.
  - Repository: `ploy`
  - Component: `internal/workflow/contracts`, `internal/workflow/mods/plan`, `internal/mods/api`, `internal/store`.
  - Scope: Introduce a `WorkflowGraph` / `StepGraph` abstraction that can be materialized from `jobs` rows and, when needed, Mods planner stages:
    - Nodes: `id` (job id), `type` (`pre_gate`, `mod`, `heal`, `re_gate`, `post_gate`), `step_index`, `attempt`, `status`.
    - Edges: parent/child relationships derived from `step_index` ordering and gate/healing windows.
    - Reuse `jobs` and `diffs` as the canonical persistence; keep any serialized graph view optional and debug-focused.
    - Clarify mapping:
      - A “step” in the spec (per manifest) maps to one or more jobs (gate, heal, main mod) for a given stage.
      - `build_gate_healing` defines the pattern of healing and re-gate jobs inserted after a gate failure.
  - Snippets:
    - Example graph node (JSON):
      - `{"id":"job-pre-gate","type":"pre_gate","step_index":1000,"status":"succeeded","children":["job-mod-0"]}`
  - Tests: Add unit tests for the graph view package (e.g. `internal/workflow/graph`) to verify that a simple one-step spec with `build_gate` and `build_gate_healing` and its `jobs` rows yields the expected node/edge set.

## Phase B — Graph construction from spec
- [x] Build an initial linear job graph for Mods runs — Represent the original spec as an ordered sequence of jobs before healing/branching.
  - Repository: `ploy`
  - Component: `internal/server/handlers/handlers_mods_ticket.go` (`createJobsFromSpec`), `internal/store/schema.sql`, `cmd/ploy`.
  - Scope: Use `createJobsFromSpec` to convert the submitted Mods spec into `jobs` rows:
    - Single-step: `pre-gate` (step_index=1000) → `mod-0` (2000) → `post-gate` (3000).
    - Multi-step (`mods[0..N-1]`): `pre-gate` (1000) → `mod[i]` (2000 + i*1000) for each `mods[i]` → `post-gate` after the last mod.
    - Keep edges implicit via `step_index` and server-driven scheduling (`ScheduleNextJob`); derive any graph view from jobs, not separate storage.
    - Inject `mod_index` on claim so `mod-N` jobs map back to `mods[N]` when building manifests on the node.
  - Snippets:
    - Example job sequence (names / step_index):
      - `pre-gate (1000) → mod-0 (2000) → post-gate (3000)`
  - Tests: Covered by `internal/server/handlers/handlers_mods_ticket_test.go` (single-step and multi-step job creation and image wiring).

- [x] Encode build_gate_healing as a job graph pattern — Describe how gate failure expands the job sequence.
  - Repository: `ploy`
  - Component: `internal/server/handlers/nodes_complete.go` (`maybeCreateHealingJobs`), `internal/store/schema.sql`, `internal/nodeagent/execution_orchestrator.go`, `internal/nodeagent/execution_healing.go`.
  - Scope: On a gate job failure (`mod_type` `pre_gate`, `post_gate`, or `re_gate`) where `build_gate_healing` is configured:
    - `maybeCreateHealingJobs` inserts one `heal` job per configured healing mod plus a `re-gate` job between the failed gate and the next non-healing job, using intermediate `step_index` values.
    - The first healing job is created as `pending`, subsequent healing jobs and the `re-gate` job start as `created` and are scheduled by the server as prior jobs complete.
    - When healing retries are exhausted or no healing is configured, remaining jobs after the gate window are canceled via `cancelRemainingJobsAfterFailure`.
    - Healing jobs use the same diff/rehydration pipeline as mods; node agents rehydrate workspaces for each `heal` job and re-gate job using diffs ordered by `step_index`.
    - Future improvement (not yet implemented): add explicit job metadata linking each `heal` / `re-gate` job back to its base gate job and associated mod for easier correlation; today correlation relies on `step_index`, `mod_type`, and job names.
  - Snippets: Current healing window example in `internal/server/handlers/nodes_complete.go` (pre-gate 1000 → heal → re-gate → mod).
  - Tests: Covered by `internal/server/handlers/nodes_complete_healing_test.go` and `internal/server/handlers/server_runs_complete_test.go` (healing insertion, retry limits, and cancellation semantics).

## Phase C — Scheduler and node assignment
- [x] Make scheduler operate on job graph nodes instead of an implicit “run with healing” loop — Each job is one unit of work.
  - Repository: `ploy`
  - Component: `internal/store/querier.go` (`ClaimJob`, `ScheduleNextJob`), `internal/server/handlers/nodes_claim.go`, `internal/server/handlers/jobs_complete.go`, `internal/server/handlers/nodes_complete.go`, `internal/nodeagent/execution_orchestrator.go`.
  - Scope:
    - Control plane scheduler:
      - `ClaimJob` selects the next `pending` job for a node, ordered by `step_index` (see `jobs_pending_idx` index).
      - After a job completes, `ScheduleNextJob` transitions the next `created` job for the run to `pending`, enforcing linear execution per run.
      - Gate failures trigger `maybeCreateHealingJobs`, which injects `heal` and `re-gate` jobs into the same ordered sequence; exhausted healing or non-gate failures cancel remaining jobs via `cancelRemainingJobsAfterFailure`.
    - Node assignment and execution:
      - `claimJobHandler` hands jobs (with `mod_type` and `step_index`) to nodes; `executeRun` in the node agent dispatches based on `mod_type`.
      - `step_index` is used for diff rehydration (`rehydrateWorkspaceForStep`) and for tagging diffs/artifacts with the correct ordering semantics.
  - Snippets:
    - Example log sequence:
      - `job claimed job_name=pre-gate step_index=1000`
      - `job completed job_name=pre-gate`
      - `scheduled next job job_name=mod-0 step_index=2000`
  - Tests: Covered by `internal/server/handlers/server_runs_claim_test.go`, `internal/server/handlers/jobs_complete_test.go`, `internal/server/handlers/nodes_complete_healing_test.go`, and `internal/nodeagent/execution_orchestrator_test.go` (job ordering and completion behavior).

- [x] Introduce node-type-specific executors — Different handlers for gate, healing, and main mods.
  - Repository: `ploy`
  - Component: `internal/nodeagent/execution_orchestrator.go`, `internal/nodeagent/execution_orchestrator_gate.go`, `internal/nodeagent/execution_healing.go`, Build Gate client.
  - Scope:
    - Replace the old monolithic `executeWithHealing` flow with job-type executors:
      - `executeGateJob` — runs Build Gate (local Docker or HTTP, depending on `PLOY_BUILDGATE_MODE`), persists detected stack and first failing gate log for later healing jobs.
      - `executeHealingJob` — runs healing mods using the persisted stack, rehydrates workspaces from diffs, hydrates `/in/build-gate.log`, and uploads healing diffs.
      - `executeModJob` — runs main Mods containers, rehydrates workspaces from diffs, uploads mod diffs and artifacts, and triggers MR creation when configured.
    - `executeRun` dispatches to these executors based on `ModType` (`pre_gate`, `post_gate`, `re_gate`, `mod`, `heal`); healing orchestration at the job level is handled by the control plane via `maybeCreateHealingJobs`.
    - `executeWithHealing` remains as an internal helper for per-step gate+healing flows and tests but is no longer the primary production orchestrator for Mods runs.
  - Snippets:
    - Dispatch mapping:
      - `mod_type=pre_gate → executeGateJob`
      - `mod_type=mod → executeModJob`
      - `mod_type=heal → executeHealingJob`
  - Tests: Covered by `internal/nodeagent/execution_orchestrator_test.go`, `internal/nodeagent/execution_healing_test.go`, and `internal/nodeagent/execution_healing_retry_test.go` (executor selection, gate/healing behavior, and env wiring).

## Phase D — Resumability and failure recovery
- [x] D1 — Server resume endpoint — Allow resuming failed/canceled Mods tickets using existing runs/jobs.
  - Repository: `ploy`
  - Component: `internal/server/handlers`, `internal/store`.
  - Scope: Implement `POST /v1/mods/{id}/resume` on the control plane:
    - Define which run states are resumable (e.g. `failed`, `canceled`) and which are not.
    - Requeue eligible jobs using existing primitives (e.g. updating job status and invoking `ScheduleNextJob`).
    - Keep schema unchanged; rely on existing `runs` and `jobs` tables.
  - Snippets:
    - Route:
      - `POST /v1/mods/{id}/resume`
  - Tests: Add handler tests verifying resume behavior for failed, canceled, and invalid-state tickets and ensuring idempotent behavior when resume is called twice.

- [ ] D2 — CLI resume wiring — Make `ploy mod resume` use the server resume endpoint.
  - Repository: `ploy`
  - Component: `cmd/ploy/mod_controlplane_commands.go`, `internal/cli/mods/resume.go`, `cmd/ploy/README.md`.
  - Scope: Wire the existing `ResumeCommand` to `POST /v1/mods/{ticket}/resume`:
    - Confirm current flags/args remain unchanged (`ploy mod resume <ticket-id>`).
    - Improve error messages when the server responds with non-2xx.
    - Document the command briefly in `cmd/ploy/README.md`.
  - Snippets:
    - CLI:
      - `ploy mod resume 3f3d1c3e-...`
      - Output: `Resume requested`
  - Tests: Extend `cmd/ploy/mod_resume_test.go` to cover success and error paths (e.g. missing ticket, server error).

- [ ] D3 — Resume-aware status and events — Make resumed runs clearly visible in status and SSE.
  - Repository: `ploy`
  - Component: `internal/server/handlers/handlers_mods_ticket.go`, `internal/server/events`.
  - Scope:
    - Add minimal metadata (e.g. `resume_count`, `last_resumed_at`) into `modsapi.TicketSummary.Metadata`.
    - Ensure ticket events published via `events.Service` reflect resume transitions so watchers can see resumes in the stream.
    - Keep existing consumers compatible by making metadata additive and optional.
  - Snippets:
    - Metadata excerpt:
      - `"metadata":{"resume_count":"1","repo_base_ref":"main"}`
  - Tests: Add tests to assert metadata presence after a resume and to verify that events include at least one resume-related update.

- [ ] D4 — Resumability invariants — Guard against unsafe or confusing resumes.
  - Repository: `ploy`
  - Component: `internal/server/handlers`, `internal/store`.
  - Scope:
    - Define invariants (e.g. no resume on `succeeded` runs; do not re-run already completed jobs).
    - Enforce them in the resume handler with clear HTTP errors (e.g. 400/409) and log messages.
  - Snippets:
    - Example error:
      - `409 Conflict: ticket state=running is not resumable`
  - Tests: Add negative-path tests where resume is rejected for non-resumable states and verify error codes/messages.

## Phase E — Parallel healing and branching
- [ ] E1 — Spec shape for multi-strategy healing — Allow multiple healing strategies while preserving current behavior.
  - Repository: `ploy`
  - Component: `docs/schemas/mod.example.yaml`, `docs/mods-lifecycle.md`, spec parsing in `internal/nodeagent/claimer_spec.go`.
  - Scope:
    - Extend the `build_gate_healing` schema to express multiple strategies (branches) in a backward-compatible way.
    - Keep the current single-list form valid and map it to a single branch.
  - Snippets: Updated YAML examples in `docs/schemas/mod.example.yaml` showing multiple healing strategies.
  - Tests: Add spec parsing tests in `internal/nodeagent/claimer_spec.go` to validate the new shape and backward compatibility.

- [ ] E2 — Control-plane branch planner — Create branch-aware healing jobs from spec.
  - Repository: `ploy`
  - Component: `internal/server/handlers/nodes_complete.go`, `internal/store`.
  - Scope:
    - Extend or complement `maybeCreateHealingJobs` to:
      - Create parallel `heal_i` + `re-gate_i` job sequences for each configured strategy.
      - Allocate distinct `step_index` windows per branch to avoid overlap.
    - Maintain existing behavior when only a single strategy is configured.
  - Snippets:
    - Example branches (step_index):
      - `heal-branch-a-0 (1500), re-gate-a (1600)`
      - `heal-branch-b-0 (1700), re-gate-b (1800)`
  - Tests: Add tests verifying correct job creation for multi-branch specs and that legacy single-branch behavior is unchanged.

- [ ] E3 — Branch-local rehydration — Keep branch workspaces isolated.
  - Repository: `ploy`
  - Component: `internal/nodeagent/execution_orchestrator.go`, rehydration helpers.
  - Scope:
    - Define how branch-local workspaces are constructed from base clone + diffs, per branch.
    - Ensure diffs from one branch are not accidentally applied to another or to the mainline until a winner is chosen.
  - Snippets:
    - Pseudocode:
      - `workspace_branch_a = base + diffs[<=gate] + diffs_branch_a`
  - Tests: Add targeted tests to confirm that each branch sees only its own diffs during rehydration.

- [ ] E4 — Winner selection and loser teardown — Choose a winning branch and clean up others.
  - Repository: `ploy`
  - Component: `internal/server/handlers/nodes_complete.go`, `internal/store`.
  - Scope:
    - Define criteria for the winning branch (e.g. first `re-gate_i` that passes).
    - Implement cancellation or archival of losing branches without corrupting mainline diffs.
  - Snippets:
    - Example winner selection:
      - `re-gate-a: passed → mark branch-a winner, cancel branch-b jobs`
  - Tests: Add tests covering winner selection, loser cancellation, and final run status when multiple branches exist.

- [ ] E5 — Parallel healing tests and guardrails — Validate end-to-end behavior.
  - Repository: `ploy`
  - Component: Healing-related tests under `internal/server/handlers` and `internal/nodeagent`.
  - Scope: Add integration-style tests (or focused multi-branch unit tests) that exercise:
    - Multi-strategy specs.
    - Branch creation, execution, and winner selection.
    - Failure modes when all branches fail.
  - Snippets:
    - Example test name:
      - `TestParallelHealing_AllBranchesFail_TicketFails`
  - Tests: Extend test suites to cover the full multi-branch flow once design is stable.

## Phase F — Docs, CLI UX, and observability
- [ ] F1 — DAG and job-flow docs — Document the job graph clearly.
  - Repository: `ploy`
  - Component: `docs/mods-lifecycle.md`, `ROADMAP_DAG.md`.
  - Scope:
    - Add diagrams and short narratives showing:
      - Simple run: pre-gate→mod→post-gate.
      - Healing run: gate→heal→re-gate→mod.
      - How future parallel branches (Phase E) will look at the job level.
    - Keep the document aligned with the actual engine behavior.
  - Snippets:
    - ASCII job graph:
      - `pre-gate → mod-0 → post-gate`
      - `          │`
      - `          └─(fail)→ heal → re-gate → mod-0`
  - Tests: Manual doc review; ensure references in tests/docs still pass `make test`.

- [ ] F2 — CLI surface for graph/state — Make DAG state visible in CLI help and examples.
  - Repository: `ploy`
  - Component: `cmd/ploy/README.md`, `internal/cli/mods/inspect.go`, `tests/README.md`.
  - Scope:
    - Enhance `ploy mod inspect` / status documentation to:
      - Show how job-level state (gate/heal/re-gate) surfaces via `GET /v1/mods/{id}` and `TicketSummary.Metadata`.
      - Provide example outputs including gate summaries and, later, branch hints if implemented.
  - Snippets:
    - Example:
      - `Ticket mods-123: running`
      - `MR: https://gitlab.com/org/repo/-/merge_requests/1`
      - `Gate: failed pre-gate duration=567ms`
  - Tests: Run `make test` and validate CLI help text tests (if any) still pass.

- [ ] F3 — Optional graph/debug view — Provide a simple way to inspect the job graph.
  - Repository: `ploy`
  - Component: `internal/server/handlers`, optional tooling.
  - Scope:
    - Add a lightweight debug endpoint or internal tool (e.g. JSON adjacency list over jobs for a ticket).
    - Keep it clearly marked as debug-only and avoid coupling clients to it as a stable public API.
  - Snippets:
    - Example JSON:
      - `{"nodes":[{"id":"pre-gate","children":["mod-0"]},{"id":"mod-0","children":["post-gate"]}]}`
  - Tests: Add a basic handler test to ensure the debug view returns consistent, well-formed output for a test run.
