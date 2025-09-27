# Runner Parallel Execution
- [x] Done (2025-09-27)

## Why / What For
Unlock actual parallelism for Mods stages so the workflow runner honors planner concurrency hints without waiting for sequential execution. This slice ensures `orw-apply`, `orw-gen`, and other independent stages execute concurrently while respecting dependency edges, keeping workstation runs aligned with Grid expectations for stage parallelism.

## Required Changes
- Extend the workflow runner execution engine to schedule stages according to dependency readiness instead of a fixed linear order.
- Ensure Mods planner metadata that flags parallel stages (`stage_metadata.mods.plan.parallel_stages`) results in concurrent Grid submissions when dependencies are satisfied.
- Maintain deterministic behaviour for stages that must remain serial (e.g., human gate, llm-exec) by enforcing dependency sequencing.
- Update in-memory Grid stub and tests so parallel submissions retain reproducible ordering for assertions.

## Definition of Done
- Mods planner output that lists parallel stages results in those stages being submitted concurrently by the runner.
- Runner waits for all parallel stages to finish (including retries) before releasing dependents such as `llm-exec` and `mods-human`.
- Checkpoints continue to publish accurate stage metadata and status transitions for parallel stages without interleaving issues.
- Documentation (`docs/design/mods/README.md`, `docs/design/README.md`) reflects the completed milestone.

## Tests
- Runner unit tests verify that `orw-apply` and `orw-gen` run in parallel (e.g., using a stub Grid client that records overlapping execution windows) and that dependents wait for both to finish.
- Tests cover retry behaviour for parallel stages to ensure failed stages reschedule without breaking dependency accounting.
- Repository-wide `go test -cover ./...` meets coverage thresholds (≥60% overall, ≥90% within `internal/workflow/mods` and updated runner components).

Status: Workflow runner now schedules Mods stages according to dependency readiness, executes `orw-apply`/`orw-gen` alongside `llm-plan`, and blocks `llm-exec`/`mods-human` until prerequisites finish with retries honoured. Verified via new concurrency-focused runner tests and full `go test -cover ./...` run on 2025-09-27.
