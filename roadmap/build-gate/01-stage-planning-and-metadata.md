# Build Gate Stage Planning & Metadata
- [x] Completed (2025-09-27)

## Why / What For
Introduce the `build-gate` and `static-checks` stages into the workflow planner so build verification happens ahead of tests, and surface structured build gate metadata through checkpoints for downstream consumers (Knowledge Base, telemetry).

## Required Changes
- Add planner support for the new stages, wiring dependencies after Mods and before tests.
- Define runner stage kinds and metadata structs for build gate results.
- Sanitize and publish build gate metadata (log digest, static check diagnostics) into workflow checkpoints and contracts.
- Update CLI/Aster overrides, tests, and fixtures to reflect the renamed stage and new checkpoint shape.

## Definition of Done
- Default planner emits `build-gate` → `static-checks` → `test` after the Mods human stage.
- Runner publishes build gate metadata in checkpoints with sanitized static check diagnostics.
- Contracts expose `build_gate` metadata with validation rules enforced in tests.
- Existing tests understand the new stage names, cache keys, and Aster overrides.

Status: Planner stages, runner metadata sanitisation, contract types, and CLI/test adjustments are complete as of 2025-09-27. Remaining slice work (sandbox runner, static check adapters, log retrieval) continues in follow-up tasks.

## Tests
- Expanded runner planner, execution, events, and grid tests to cover the new stages.
- Added `internal/workflow/buildgate` sanitisation tests.
- Extended contract validation tests for `build_gate` metadata.
- Repository-wide `go test -cover ./...` enforces coverage after the change.
