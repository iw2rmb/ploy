# Mods Workflow Parallel Planner (Roadmap 19)

## Purpose

Reboot the Mods workflow so planning, OpenRewrite execution, and human
checkpoints run as parallel stages that align with Grid-driven workflow
orchestration. This slice reintroduces the legacy Mods runner capabilities
recovered from commit `3b11d7e8` (builder log enrichment, healing flow) while
embracing the current stateless CLI and JetStream contracts. The planner must
schedule the canonical sequence of `orw-apply`, `orw-gen`, `llm-plan`,
`llm-exec`, and `human-in-the-loop` steps with concurrency awareness and
explicit Grid stage metadata.

## Status

- [x] Planner skeleton (docs/tasks/mods/01-planner-skeleton.md) — Mods DAG emitted
      by default planner (2025-09-26).
- [x] Knowledge Base feedback loop (docs/tasks/mods/02-knowledge-base-feedback.md)
      — Mods planner now records knowledge base advice inside
      `stage_metadata.mods` (2025-09-26).
- [x] CLI surface and Grid wiring (docs/tasks/mods/03-cli-grid-wiring.md) —
      `ploy workflow run` exposes planner hints and pushes concurrency metadata
      into Grid/JetStream (2025-09-26).
- [x] Runner parallel execution (docs/tasks/mods/04-runner-parallel-execution.md) —
      Workflow runner executes Mods stages according to planner parallelism
      hints with dependency-aware scheduling (2025-09-27).
- [x] Mods Grid restoration (docs/design/mods-grid-restoration/README.md) —
      Steps 1–4 completed 2025-10-05 (repo materialisation, Mods lanes, healing
      retries). SHIFT migration of lanes tracked via
      `docs/tasks/mods-grid/05-refactor.md`.

## Scope

- Applies to a new `internal/workflow/mods` package housing planner, executor
  stubs, and Grid integration shims.
- Updates workflow stage construction so Mods contributes multiple parallel
  stages to the root DAG emitted by `ploy workflow run`.
- Extends event contracts with Mods-specific stage metadata (step kind, recipe
  IDs, planning status) without altering global schema fields.
- Coordinates with the Knowledge Base slice
  (`docs/design/knowledge-base/README.md`) for error recommendations during
  `llm-plan` and `human-in-the-loop` checkpoints.

## Background & Prior Art

- Commit `3b11d7e8` shipped the previous `ModRunner.runBuildGate`/healing flow
  with builder log hydration and step orchestration inside the Nomad-era runner.
  We will port the planning heuristics, healing retry structure, and logging
  semantics into the new workflow package while replacing controller calls with
  Grid workflow submissions.
- Existing roadmap docs (`docs/tasks/orw-test.md`, `docs/tasks/recipes.md`) capture
  OpenRewrite recipe usage; these become inputs for the new planner API.

## Behaviour

- `llm-plan` generates a plan graph by evaluating repo state, recipes, and
  Knowledge Base suggestions. Planning emits JetStream checkpoints with a
  `stage_metadata.mods.plan` payload describing selected recipes, concurrency
  lanes, and expected human checkpoints.
- `orw-apply` and `orw-gen` run as sibling stages. They depend on the `llm-plan`
  output and execute OpenRewrite transformations inside Grid jobs using the
  configured lane. Each stage publishes artifact manifests (diff bundles) to
  `ploy.artifact.<ticket>`.
- `llm-exec` consumes the plan plus OpenRewrite diffs, executing iterative fixes
  through Grid jobs. Failures trigger Knowledge Base lookups and optionally
  spawn `human-in-the-loop` review tickets.
- `human-in-the-loop` is a manual gate that surfaces curated instructions and
  diff previews to operators. Completion toggles a checkpoint acknowledged via
  JetStream; timeouts escalate back to `llm-exec`.
- Mods checkpoints embed advisor output inside `stage_metadata.mods.plan`,
  `stage_metadata.mods.recommendations`, and `stage_metadata.mods.human` so Grid
  tooling can recover recipe choices, human expectations, and confidence values
  from checkpoints.
- Planner configuration publishes execution hints (`plan_timeout`,
  `max_parallel`) alongside the Mods plan metadata so Grid can tune runner
  behaviour without code changes.
- Healing retries reuse the legacy approach: unsuccessful builds or diff
  validation trigger another planning cycle unless the Knowledge Base signals no
  viable remediation.

## Implementation Notes

- Introduce `mods.Planner` that loads roadmap recipes, Knowledge Base hints, and
  repo metadata (lanes, manifests) to build a DAG spec. Planner outputs map to
  workflow stages consumed by the existing runner planner.
- Define stage kinds (`mods-plan`, `orw-apply`, `orw-gen`, `llm-plan`,
  `llm-exec`, `mods-human`) in `internal/workflow/contracts` so checkpoints
  carry consistent metadata.
- Rehydrate builder log enrichment from commit `3b11d7e8` by adapting to Grid
  status streams: stage failures request `jobs.<run_id>.events` transcripts and
  artifact CIDs rather than hitting the retired controller HTTP API.
- Extend the workflow runner to fan out `orw-*` stages concurrently when the
  plan marks them parallelizable. Use Grid workflow dependencies to express
  concurrency instead of bespoke goroutines.
- Persist Mods plan summaries and diff bundles to IPFS via existing artifact
  publishers, referencing returned CIDs in checkpoints for Knowledge Base
  lookups.
- Provide CLI flags (`--mods-plan-timeout`, `--mods-max-parallel`) surfaced
  through `ploy workflow run` for operator control.

## Tests

- Unit tests for `mods.Planner` covering recipe selection, dependency graph
  construction, and Knowledge Base fusion; mocked Knowledge Base responses drive
  deterministic expectations.
- Workflow runner tests asserting stage metadata, concurrency fan-out, and
  JetStream artifact publication for `orw-*` stages via the in-memory Grid stub.
- Integration-style tests exercising healing retries by injecting synthetic
  build failures and validating Knowledge Base escalations to
  `human-in-the-loop`.
- Repository-wide `go test -cover ./...` with focus on ≥90% coverage inside
  `internal/workflow/mods` and planner utilities.
- Maintain RED → GREEN → REFACTOR: introduce failing planner/runner tests first,
  add minimal code to go GREEN, then refactor once coverage guardrails hold.

## Rollout & Follow-ups

- Add roadmap entries under `docs/tasks/mods/` capturing planner implementation,
  Grid wiring, Knowledge Base integration, and CLI surface work.
- Coordinate with the Build Gate reboot (`docs/design/build-gate/README.md`) to
  ensure post-plan builds reuse shared log parsing and static analysis outputs.
- Future slice: surface per-step analytics in `docs/design/telemetry/README.md`
  once telemetry strategy lands.
