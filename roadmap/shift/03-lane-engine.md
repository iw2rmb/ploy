# Lane Engine Redesign

- [x] Done (2025-09-26)

## Why / What For

Recast lanes as build profiles optimised for Grid runtimes while letting Ploy
compute deterministic cache keys.

## Required Changes

- Define new lane spec format (`configs/lanes/*.toml`) with runtime families,
  cache namespaces, and commands.
- Implement cache key composer incorporating lane, commit, snapshot, manifest,
  and Aster toggles.
- Expose inspection command `ploy lanes describe` with rich output.

## Definition of Done

- All legacy Nomad lane metadata removed.
- CLI prints accurate lane descriptions and cache previews.
- Grid stub validates that submitted jobs carry expected lane metadata.

## Current Status (2025-09-26)

- Lane specs live under `configs/lanes/*.toml` with validation handled by
  `internal/workflow/lanes`.
- Cache keys incorporate commit, snapshot, manifest, and Aster toggles.
- `ploy lanes describe` exposes lane metadata and cache previews against the
  Grid stub.

## Tests

- Unit tests for cache-key composition and lane parsing.
- Golden tests for `ploy lanes describe` output.
- Static analysis ensuring no Nomad lane references remain.
- Follow RED → GREEN → REFACTOR: add failing parser tests, implement minimal
  lane wiring, then tighten outputs once tests pass.
