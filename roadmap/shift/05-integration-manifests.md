# Integration Manifest Compiler
- [x] Done (2025-09-26)

## Why / What For
Ensure topology, fixtures, and lane requirements are declared once and enforced across every workflow run.

## Required Changes
- Define TOML/Markdown schema with validation rules and helpful errors.
- Build compiler that turns manifests into JSON payloads for Grid topology enforcement.
- Update docs and samples to teach teams how to author manifests.

Status: Manifest loader now lives at `internal/workflow/manifests`, validates schema rules, normalises Aster toggles, and emits JSON payloads consumed by the workflow runner. `runner.Options` requires a manifest compiler; `ploy workflow run` loads configs under `configs/manifests/` and fails fast on invalid manifests. The in-memory Grid stub checks lane allowlists before executing stages. Documentation covering schema/usage was added to `docs/MANIFESTS.md`, with sample manifests (`smoke`, `commit-app`) shipping in-repo.

## Definition of Done
- CLI rejects invalid manifests with actionable error messages.
- Grid stub receives compiled topology payload and enforces allowlists before execution.
- Example manifests exist for mods workflows and commit-scoped environments.

## Tests
- `go test ./internal/workflow/manifests` validates schema coverage and compilation behaviour.
- `go test ./internal/workflow/runner` ensures manifest compilation wiring surfaces Grid errors and enforces lane allowlists.
- `go test ./cmd/ploy` checks CLI flag parsing and manifest loader error propagation.
