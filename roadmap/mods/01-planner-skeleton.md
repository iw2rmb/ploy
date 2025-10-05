# Mods Planner Skeleton

- [x] Completed (2025-09-26)

## Why / What For

Lay the groundwork for Roadmap 19 by replacing the single "mods" stage with a
DAG that captures the Mods planner, OpenRewrite concurrency, LLM execution, and
the human checkpoint. The workflow runner needs these stages to expose accurate
dependencies to Grid and future Knowledge Base hooks while keeping workstation
behaviour deterministic.

## Required Changes

- Introduce an `internal/workflow/mods` package that produces the Mods stage
  graph (plan → parallel OpenRewrite → LLM execution → human gate).
- Extend the default workflow planner to delegate Mods stage construction to the
  new package and append the existing build/test stages with updated
  dependencies.
- Enumerate Mods stage kinds so checkpoints expose stable identifiers for
  downstream consumers.

## Definition of Done

- `mods.Planner` builds stages named `mods-plan`, `orw-apply`, `orw-gen`,
  `llm-plan`, `llm-exec`, and `mods-human` with the dependency structure
  described in the design doc and lane assignments mapped to existing configs.
- Runner execution plans include the Mods stages ahead of `build` and `test`,
  with each later stage depending on the correct Mods predecessors.
- Checkpoints emit Mods stage kinds via the existing metadata path without
  schema validation errors.
- Roadmap entry marked done and design docs updated to reflect the completed
  skeleton.

## Current Status (2025-09-26)

- Mods planner skeleton emits the expanded stage DAG and updated checkpoints.
- Unit tests and `go test -cover ./...` verify dependency ordering and metadata
  propagation.
- Design docs reflect the new skeleton structure.

## Tests

- Unit tests for `internal/workflow/mods` verifying stage names, kinds, lane
  assignments, and dependencies.
- Workflow runner planner test asserting the expanded Mods-first DAG ordering.
- `go test -cover ./...` meets repository coverage expectations (≥60% overall,
  ≥90% Mods package).
- Maintain RED → GREEN → REFACTOR: fail planner tests first, add minimal DAG
  wiring, then refactor once coverage passes.
