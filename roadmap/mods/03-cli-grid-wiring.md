# Mods CLI Surface & Grid Wiring

- [x] Done (2025-09-26)

## Why / What For

Expose Mods planner controls through the CLI so operators can adjust plan
execution without code changes and ensure the runner sends Mods stages to Grid
with the right concurrency hints. This slice unlocks the next workstation
milestone where Mods lanes must run with configurable parallelism while
respecting workstation-only execution.

## Required Changes

- Add CLI flags on `ploy workflow run` for Mods planner tuning (plan timeout,
  max parallel stages) and plumb them to the runner options.
- Extend the workflow runner planner configuration so Mods stages honour the
  CLI-provided options when constructing the DAG.
- Ensure Grid invocations carry Mods-specific execution hints, including the new
  concurrency metadata, while keeping schema validation intact.
- Update stubs and tests to cover the new configuration fields and runner/Grid
  wiring.

## Definition of Done

- `ploy workflow run` accepts `--mods-plan-timeout` (duration string) and
  `--mods-max-parallel` (integer) flags, validating input and passing values to
  the workflow runner.
- Runner planner configuration forwards Mods options into `mods.NewPlanner` and
  includes the values in stage metadata or execution hints as defined by the
  design.
- Grid client receives Mods stages with the configured options reflected in the
  request payload; in-memory Grid captures the same.
- Docs and change log note the new flags and Mods wiring; design index
  references the completed milestone.

## Current Status (2025-09-26)

- CLI exposes `--mods-plan-timeout` and `--mods-max-parallel`, pushing validated
  options into the workflow runner.
- Stage metadata and Grid invocation payloads reflect the configured parallelism
  hints.
- Docs and CHANGELOG entries document the new flags and wiring.

## Tests

- CLI unit test verifying `recordingRunner` receives Mods planner options from
  the new flags and rejects invalid flag values.
- Runner unit test ensuring Mods planner options propagate into stage
  metadata/execution hints and through the Grid invocation path.
- Repository-wide `go test -cover ./...` remains ≥60% overall coverage and
  maintains ≥90% in Mods packages.
- Reinforce RED → GREEN → REFACTOR: write failing CLI/runner option tests, add
  minimal wiring, then refactor once coverage steadies.
