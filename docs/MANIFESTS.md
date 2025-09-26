# Integration Manifests

## Purpose
Describe topology, fixtures, lane requirements, and Aster toggles that every workflow run must honour when it dispatches jobs to Grid. Manifests keep the workflow runner stateless while ensuring commit- and workflow-scoped environments stay within approved boundaries.

## Current Status
- CLI loads manifests from `configs/manifests/` and refuses to run when a ticket references an unknown or mismatched version.
- `smoke` and `commit-app` manifests ship with representative allowlists, fixtures, and Aster toggles.
- Grid enforcement is stubbed locally: the in-memory Grid rejects stages that target lanes not declared in the manifest. JetStream/Grid wiring will replace the stub once `GRID_ENDPOINT` integration resumes.

## Usage / Commands
- `ploy workflow run --tenant <tenant>` automatically loads the referenced manifest and surfaces actionable errors when validation fails. Combine `--aster` with `--aster-step <stage=toggle|stage=off>` to enable or disable Aster toggles on a per-stage basis while keeping manifests canonical.
- Manifests compile to JSON payloads that the workflow runner attaches to each stage before dispatching to Grid.
- Use TOML files under `configs/manifests/` to add or update manifests. Run `go test ./internal/workflow/manifests` to exercise schema validation helpers.
- `ploy manifest schema` prints the machine-readable schema located at `docs/schemas/integration_manifest.schema.json` so other tools can validate manifests without embedding Ploy's loader.

## Schema
- JSON Schema (Draft 2020-12): `docs/schemas/integration_manifest.schema.json`.
- Top-level fields are required: `name`, `version`, `summary`, `topology`, `fixtures`, `lanes`.
- `topology.allow` must list at least one flow (`from`/`to`), while `topology.deny` entries require explicit reasons.
- `fixtures.required` and `lanes.required` demand at least one entry, ensuring workflow runs always enumerate baseline fixtures/lanes.
- `aster.required`/`aster.optional` accept unique toggle names so cache keys remain deterministic.

## Development Notes
- Keep manifests workstation-friendly. Defer any live `JETSTREAM_URL`, `GRID_ENDPOINT`, or `IPFS_GATEWAY` references until the JetStream slice resumes; mark them as TODOs when new manifests depend on remote endpoints.
- Nomad manifests are historical; favour TOML manifests documented here so Grid remains the sole execution surface.
- Each manifest must include:
  - `summary` (Markdown), `topology.allow` flows, optional `topology.deny` blocks with reasons.
  - `fixtures.required` references (snapshots or services) and optional fixtures with reasons.
  - `lanes.required` plus optional lanes that Grid can provision.
  - `aster` toggles (required/optional) to keep cache keys deterministic.
- The manifest compiler deduplicates and sorts Aster toggles; keep files readable to minimise churn in golden JSON payloads.

## Related Docs
- `docs/design/shift/README.md` — overarching SHIFT design.
- `docs/DOCS.md` — documentation matrix and editing conventions.
- `docs/SNAPSHOTS.md` — snapshot toolkit referenced by manifest fixtures.
- `roadmap/shift/05-integration-manifests.md` — roadmap slice covering this work.
- `roadmap/shift/08-documentation-cleanup.md` — documentation alignment status and verification notes.
