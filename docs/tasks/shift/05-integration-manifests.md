# Integration Manifest Compiler

- [x] Done (2025-09-26)

## Why / What For

Ensure topology, fixtures, and lane requirements are declared once and enforced
across every workflow run.

## Required Changes

- Define TOML/Markdown schema with validation rules and helpful errors.
- Build compiler that turns manifests into JSON payloads for Grid topology
  enforcement.
- Update docs and samples to teach teams how to author manifests.

## Current Status (2025-09-26)

- Manifest loader resides in `internal/workflow/manifests`, applying schema
  validation and Aster toggle normalisation.
- `runner.Options` now requires a manifest compiler; `ploy workflow run` loads
  configs under `configs/manifests/` and fails fast on invalid inputs.
- The in-memory Grid stub enforces lane allowlists, and docs + samples (`smoke`,
  `commit-app`) explain authoring flows in `docs/MANIFESTS.md`.

## Definition of Done

- CLI rejects invalid manifests with actionable error messages.
- Grid stub receives compiled topology payload and enforces allowlists before
  execution.
- Example manifests exist for mods workflows and commit-scoped environments.

## Tests

- `go test ./internal/workflow/manifests` validates schema coverage and
  compilation behaviour.
- `go test ./internal/workflow/runner` ensures manifest compilation wiring
  surfaces Grid errors and enforces lane allowlists.
- `go test ./cmd/ploy` checks CLI flag parsing and manifest loader error
  propagation.
- Maintain RED → GREEN → REFACTOR discipline: add failing manifest validation
  tests, wire minimal compiler logic, then refactor once CLI coverage holds.
