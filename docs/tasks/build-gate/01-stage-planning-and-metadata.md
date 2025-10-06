# Build Gate Stage Planning & Metadata

- [x] Completed (2025-09-27)

## Why / What For

Introduce the `build-gate` and `static-checks` stages into the workflow planner
so build verification happens ahead of tests, and surface structured build gate
metadata through checkpoints for downstream consumers (Knowledge Base,
telemetry).

## Required Changes

- Add planner support for the new stages, wiring dependencies after Mods and
  before tests.
- Define runner stage kinds and metadata structs for build gate results.
- Sanitize and publish build gate metadata (log digest, static check
  diagnostics) into workflow checkpoints and contracts.
- Update CLI/Aster overrides, tests, and fixtures to reflect the renamed stage
  and new checkpoint shape.

## Definition of Done

- Default planner emits `build-gate` → `static-checks` → `test` after the Mods
  human stage.
- Runner publishes build gate metadata in checkpoints with sanitized static
  check diagnostics.
- Contracts expose `build_gate` metadata with validation rules enforced in
  tests.
- Existing tests understand the new stage names, cache keys, and Aster
  overrides.

## Current Status (2025-09-27)

- Planner emits `build-gate` and `static-checks` stages with correct
  dependencies.
- Runner metadata sanitisation, contract updates, and CLI/test adjustments are
  complete.
- Follow-up work for sandbox runner, static check adapters, and log retrieval
  lives in subsequent roadmap slices.

## Tests

- Runner planner, execution, events, and grid tests cover the new stages.
- `internal/workflow/buildgate` sanitisation tests enforce metadata hygiene.
- Contract validation tests cover `build_gate` metadata.
- Repository-wide `go test -cover ./...` enforces coverage after the change.
- Keep RED → GREEN → REFACTOR cadence: start with failing planner/build gate
  tests, implement minimal metadata wiring, then refactor once coverage holds.
