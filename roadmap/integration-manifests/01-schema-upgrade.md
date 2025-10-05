# Integration Manifest Schema Upgrade

- [x] Completed (2025-09-29)

## Why / What For

Grid's topology compiler consumes manifests with service, port, edge, and
exposure metadata (see `../../../grid/testdata/topology/sample-gate.toml`). Ploy
still emits v1 manifests limited to allow/deny pairs, preventing end-to-end
enforcement once Grid ships its compiler. Upgrading the schema keeps workstation
planning aligned with the execution plane and unblocks integration testing.

## Required Changes

- Extend `docs/schemas/integration_manifest.schema.json` with service, port,
  edge, and exposure definitions that mirror Grid fixtures.
- Update `internal/workflow/manifests` to parse the v2 schema, normalise data,
  and emit versioned payloads.
- Migrate shipped manifests under `configs/manifests/` to the new structure.
- Add CLI tooling (`ploy manifest validate`, `ploy manifest schema`) plus
  documentation updates to guide users through the new format.
- Introduce a migration validator or rewrite helper so existing manifests can be
  converted deterministically.

## Definition of Done

- v2 schema validated by failing unit tests before implementation, then passing
  once changes land.
- Sample manifests (`commit-app`, `smoke`) emit JSON payloads matching Grid
  fixtures (deterministic ordering, version tag).
- CLI documentation (`docs/MANIFESTS.md`) and design records reference the
  upgraded schema, with CHANGELOG entry dated 2025-09-xx.
- Knowledge base/runner integration tests load v2 manifests without regressions.

## Tests to Perform

- `go test ./internal/workflow/manifests` (including new table-driven cases for
  v2 parsing and determinism).
- CLI acceptance tests covering `ploy manifest schema` and migration workflow.
- Snapshot/golden comparison asserting parity with
  `../../../grid/testdata/topology/sample-gate.toml` and
  `events/topology_compiled.json`.

## Status Log

- 2025-09-29 — RED tests added for v2 schema, CLI surface, and schema asset
  enforcement.
- 2025-09-29 — GREEN: compiler, validator, CLI, manifests, docs, and CHANGELOG
  updated;
  `go test ./internal/workflow/manifests ./cmd/ploy ./internal/workflow/runner ./internal/workflow/environments`.

## Links & References

- Design: `docs/design/integration-manifests/README.md`.
- Grid fixtures: `../../../grid/testdata/topology/README.md` and related
  samples.
- SHIFT tracker: `docs/design/shift/README.md`.
