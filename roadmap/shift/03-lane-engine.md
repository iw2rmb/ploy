# Lane Engine Redesign
- [x] Done (2025-09-26)

## Why / What For
Recast lanes as build profiles optimised for Grid runtimes while letting Ploy compute deterministic cache keys.

## Required Changes
- Define new lane spec format (`configs/lanes/*.toml`) with runtime families, cache namespaces, and commands.
- Implement cache key composer incorporating lane, commit, snapshot, manifest, and Aster toggles.
- Expose inspection command `ploy lanes describe` with rich output.

## Definition of Done
- All legacy Nomad lane metadata removed.
- CLI prints accurate lane descriptions and cache previews.
- Grid stub validates that submitted jobs carry expected lane metadata.

Status: Lane specs now live under `configs/lanes/*.toml`, `internal/workflow/lanes` loads and validates them, cache keys incorporate commit/snapshot/manifest/Aster toggles, and `ploy lanes describe` exposes the inspection view with previews for the stubbed Grid submissions.

## Tests
- Unit tests for cache key math and lane parsing.
- Golden tests for `describe` command output.
- Static analysis to ensure no Nomad lane references remain.
