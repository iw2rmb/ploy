# Integration Manifests

## Purpose

Describe topology, fixtures, lane requirements, and (when `PLOY_ASTER_ENABLE` is
set) Aster toggles that every Mods run must honour when it dispatches jobs
to Grid. Manifests keep the workflow runner stateless while ensuring commit- and
workflow-scoped environments stay within approved boundaries.

## Current Status

- CLI loads manifests from `configs/manifests/` and refuses to run when a ticket
  references an unknown or mismatched version.
- `smoke` and `commit-app` manifests ship with the v2 schema:
  `manifest_version = "v2"`, service/edge/exposure metadata, fixtures, lanes,
  and Aster toggles.
- `ploy manifest validate` rewrites manifests in place when `--rewrite=v2` is
  supplied, keeping TOML output deterministic for review.
- Grid enforcement is stubbed locally: the in-memory Grid rejects stages that
  target lanes not declared in the manifest. JetStream/Grid wiring will replace
  the stub once `GRID_ENDPOINT` integration resumes.

## Usage / Commands

- `ploy mod run --tenant <tenant>` automatically loads the referenced
  manifest and surfaces actionable errors when validation fails. When
  `PLOY_ASTER_ENABLE` is set you can combine `--aster` with
  `--aster-step <stage=toggle|stage=off>` to enable or disable Aster toggles on
  a per-stage basis while keeping manifests canonical.
- Manifests compile to JSON payloads that the workflow runner attaches to each
  stage before dispatching to Grid.
- Use TOML files under `configs/manifests/` to add or update manifests. Run
  `go test ./internal/workflow/manifests` to exercise schema validation helpers.
- `ploy manifest schema` prints the machine-readable schema located at
  `docs/schemas/integration_manifest.schema.json` so other tools can validate
  manifests without embedding Ploy's loader.
- `ploy manifest validate [--rewrite=v2] <path>` validates one or more manifests
  and optionally rewrites them with canonical ordering. The rewrite happens in
  place and preserves file permissions.

## Schema

- JSON Schema (Draft 2020-12): `docs/schemas/integration_manifest.schema.json`.
- Top-level fields are required: `manifest_version` (currently fixed to `v2`),
  `name`, `version`, `summary`, `topology`, `fixtures`, `lanes`, `services`, and
  `edges`.
- `topology.allow` must list at least one flow (`from`/`to`), while
  `topology.deny` entries require explicit reasons for observability.
- `services` enumerate workloads with identities, ports, optional flags, and
  dependency requirements. `edges` capture connectivity between services with
  explicit port/protocol references.
- `exposures` describe public/cluster/local visibility per service port. The
  field is optional but required when a service must be reachable outside the
  topology graph.
- `fixtures.required` and `lanes.required` demand at least one entry, ensuring
  Mods runs always enumerate baseline fixtures/lanes.
- `aster.required`/`aster.optional` accept unique toggle names so cache keys
  remain deterministic.

## Development Notes

- Keep manifests workstation-friendly. Rely on Grid discovery for remote
  endpoints instead of hard-coding JetStream routes or IPFS gateways inside
  manifests.
- Nomad manifests are historical; favour TOML manifests documented here so Grid
  remains the sole execution surface.
- Each manifest must include:
  - `manifest_version = "v2"`, `summary` (Markdown), `topology.allow` flows,
    optional `topology.deny` blocks with reasons.
  - `services` definitions with identities, ports, dependency requirements, and
    optional flags.
  - `edges` wiring services together with explicit port/protocol references plus
    optional `exposures` when ports need to be public or cluster-visible.
  - `fixtures.required` references (snapshots or services) and optional fixtures
    with reasons.
  - `lanes.required` plus optional lanes that Grid can provision.
  - `aster` toggles (required/optional) to keep cache keys deterministic.
- The manifest compiler deduplicates and sorts services, dependencies, edges,
  exposures, and Aster toggles; keep files readable to minimise churn in golden
  JSON payloads.

## Related Docs

- `docs/design/overview/README.md` — overarching feature design.
- `docs/DOCS.md` — documentation matrix and editing conventions.
- `docs/SNAPSHOTS.md` — snapshot toolkit referenced by manifest fixtures.
- `docs/tasks/shift/05-integration-manifests.md` — roadmap slice covering this
  work.
- `docs/tasks/shift/08-documentation-cleanup.md` — documentation alignment status
  and verification notes.
