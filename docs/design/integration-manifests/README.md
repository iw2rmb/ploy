# Integration Manifest v2 Schema

## Purpose

Expand Ploy integration manifests so they capture the same service, port, edge,
and exposure metadata that Grid's topology compiler expects. The richer schema
keeps workstation planning aligned with the execution plane once Grid begins
enforcing manifests.

## Status (2025-09-29)

- [x] [docs/tasks/integration-manifests/01-schema-upgrade.md](../../../docs/tasks/integration-manifests/01-schema-upgrade.md)
      — Schema, compiler, CLI tooling, manifests, and docs verified for the v2
      contract.

## Background

Current manifests (e.g. `configs/manifests/commit-app.toml`) advertise topology
as simple allow/deny pairs. Grid's fixtures under
[`../grid/testdata/topology/sample-gate.toml`](../../../../grid/testdata/topology/sample-gate.toml)
already model services with named ports, dependency edges, and exposure modes.
Without parity, Grid cannot accept Ploy's manifests once the topology compiler
ships and Ploy loses validation coverage for production-style policies.

## Goals

- Extend manifest schema to describe services, ports, optional/required
  dependencies, and exposures in a deterministic format.
- Preserve backwards compatibility by migrating existing manifests with tooling
  and fail-fast validation.
- Ensure manifest compiler emits JSON payloads matching Grid's topology fixtures
  so integration tests can share samples.
- Document the new schema for workstation users and downstream tooling.

## Non-Goals

- Implement Grid topology enforcement or DNS policy compilation (covered by Grid
  roadmap milestones).
- Deliver runtime ACL enforcement inside Ploy; Ploy remains a planner/validator.

## Proposed Changes

1. **Schema evolution**
   - Introduce a `services` array with per-service identity, ports, optional
     flag, and dependency requirements.
   - Add an `edges` array capturing explicit connectivity with named port/group
     references.
   - Define `exposures` to represent public/cluster/local visibility
     expectations.
   - Retain `fixtures`, `lanes`, and `aster` blocks, enforcing deterministic
     ordering.
2. **Compiler updates**
   - Update `internal/workflow/manifests` to parse new sections, normalise
     arrays (sort keys), and emit a versioned JSON structure
     (`manifest_version: "v2"`).
   - Maintain legacy `topology.allow/deny` parsing only for migration warnings,
     failing fast once v2 becomes mandatory.
3. **CLI & Docs**
   - Extend `ploy manifest schema` output and documentation
     (`docs/MANIFESTS.md`) with v2 examples mirroring Grid fixtures.
   - Provide a migration guide plus validation command
     (`ploy manifest validate --rewrite=v2`) to rewrite existing manifests
     safely.
4. **Tests**
   - Add red tests covering new schema parsing, deterministic output, and Grid
     fixture parity before implementation.

## Dependencies

- [`../../MANIFESTS.md`](../../MANIFESTS.md) — User-facing manifest guidance.
- [`../../../configs/manifests/commit-app.toml`](../../../configs/manifests/commit-app.toml)
  — Existing manifest requiring migration.
- [`../../shift/README.md`](../../shift/README.md) — SHIFT roadmap status
  tracker.
- [`../../workflow-rpc-alignment/README.md`](../../workflow-rpc-alignment/README.md)
  — Ensures job submissions keep referencing manifest payloads.
- [`../../../../grid/testdata/topology/sample-gate.toml`](../../../../grid/testdata/topology/sample-gate.toml)
  and
  [`../../../../grid/testdata/topology/events/topology_compiled.json`](../../../../grid/testdata/topology/events/topology_compiled.json)
  — Source-of-truth fixtures for the new schema.

## Risks & Mitigations

- **Schema drift between repos**: Mirror Grid fixtures locally as tests and add
  cross-repo links in docs.
- **Migration friction**: Ship a validator/rewrite tool and document manual
  steps.
- **Determinism regressions**: Keep normalization logic covered by golden tests.

## Test Strategy

- Unit tests in `internal/workflow/manifests` validating parsing, normalization,
  and JSON output parity against Grid fixtures.
- CLI tests for `ploy manifest schema` and migration commands ensuring help text
  and rewrites stay in sync.
- Integration tests simulating `ploy workflow run` manifest loading in both stub
  and Grid endpoint modes.
- RED → GREEN → REFACTOR cadence: add failing schema/migration tests, implement
  minimal parsing updates, then refactor once parity with Grid fixtures is
  confirmed.

## Deliverables

- Updated JSON schema (`docs/schemas/integration_manifest.schema.json`) with
  service/edge/exposure definitions.
- Migrated sample manifests (`configs/manifests/*.toml`) plus new fixture
  covering the Grid sample gate.
- CLI validation/migration tooling and docs.
- CHANGELOG entry capturing the new behaviour with concrete date.

## Outcome

- `manifest_version = "v2"` is now required in every manifest; the compiler
  preserves the value in JSON payloads.
- Services, edges, exposures, fixtures, lanes, and Aster toggles are normalised
  deterministically with new helpers in `internal/workflow/manifests`.
- `manifests.LoadFile` and `manifests.EncodeCompilationToTOML` enable
  single-file validation and canonical rewrites consumed by the CLI.
- `ploy manifest validate [--rewrite=v2]` validates or rewrites manifests in
  place, preserving permissions and mirroring Grid's connectivity expectations.
- `docs/MANIFESTS.md` documents the v2 schema, command usage, and migration
  workflow for workstation operators.

## Verification Plan

- 2025-09-29 —
  `go test ./internal/workflow/manifests ./cmd/ploy ./internal/workflow/runner ./internal/workflow/environments`
  (v2 schema coverage, CLI validate tests, runner assertions).
- 2025-09-29 — Manually reviewed rewritten manifests
  (`configs/manifests/commit-app.toml`, `configs/manifests/smoke.toml`) and
  schema file (`docs/schemas/integration_manifest.schema.json`) for v2
  compliance.
- 2025-09-29 — Confirmed design/docs alignment across `docs/MANIFESTS.md`, this
  design record, and roadmap entry
  `docs/tasks/integration-manifests/01-schema-upgrade.md`.
