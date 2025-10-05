# Runner Parallel Execution

- [x] Done (2025-09-27)

## Why / What For

Unlock actual parallelism for Mods stages so the workflow runner honors planner
concurrency hints without waiting for sequential execution. This slice ensures
`orw-apply`, `orw-gen`, and other independent stages execute concurrently while
respecting dependency edges, keeping workstation runs aligned with Grid
expectations for stage parallelism.

## Required Changes

- Extend the workflow runner execution engine to schedule stages according to
  dependency readiness instead of a fixed linear order.
- Ensure Mods planner metadata that flags parallel stages
  (`stage_metadata.mods.plan.parallel_stages`) results in concurrent Grid
  submissions when dependencies are satisfied.
- Maintain deterministic behaviour for stages that must remain serial (e.g.,
  human gate, llm-exec) by enforcing dependency sequencing.
- Update in-memory Grid stub and tests so parallel submissions retain
  reproducible ordering for assertions.

## Definition of Done

- Mods planner output that lists parallel stages results in those stages being
  submitted concurrently by the runner.
- Runner waits for all parallel stages to finish (including retries) before
  releasing dependents such as `llm-exec` and `mods-human`.
- Checkpoints continue to publish accurate stage metadata and status transitions
  for parallel stages without interleaving issues.
- Documentation (`docs/design/mods/README.md`, `docs/design/README.md`) reflects
  the completed milestone.

## Current Status (2025-09-27)

- Workflow runner schedules Mods stages according to dependency readiness,
  running `orw-apply`/`orw-gen` alongside `llm-plan`.
- `llm-exec` and `mods-human` block until prerequisites finish, including retry
  handling.
- Concurrency-focused runner tests and `go test -cover ./...` verified the
  behaviour on 2025-09-27.

## Tests

- Runner unit tests verify that `orw-apply` and `orw-gen` run in parallel (using
  a stub Grid client to record overlapping execution windows) and that
  dependents wait for both to finish.
- Tests cover retry behaviour for parallel stages to ensure failed stages
  reschedule without breaking dependency accounting.
- Repository-wide `go test -cover ./...` meets coverage thresholds (≥60%
  overall, ≥90% within `internal/workflow/mods` and updated runner components).
- Keep RED → GREEN → REFACTOR: write failing concurrency tests, add minimal
  scheduler updates, then refactor once retries and coverage hold steady.
