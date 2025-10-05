# Documentation Cleanup

- [x] Done (2025-09-26)

## Why / What For

Align repo docs with the Grid-centric operating model and remove references to
Nomad-era workflows.

## Required Changes

- Update `docs/LANES.md`, `docs/SNAPSHOTS.md`, `docs/MANIFESTS.md`, and README
  entries to reference the new CLI and Grid integration points.
- Add design doc cross-links and migration notes for teams adopting the new
  flows.
- Ensure CHANGELOG highlights the stateless CLI milestone and Nomad removal.

## Current Status (2025-09-26)

- README anchors the doc set around the stateless CLI with links to refreshed
  guides.
- `docs/LANES.md`, `docs/SNAPSHOTS.md`, and `docs/MANIFESTS.md` reference
  Grid/JetStream pathways and active CLI behaviour.
- `docs/DOCS.md` provides the documentation matrix and CHANGELOG records the
  cleanup milestone with dates.

## Definition of Done

- All referenced docs describe the JetStream/Grid workflow and omit
  Nomad/Consul/Traefik guidance.
- Cross-links between design doc, roadmap tasks, and docs are in place.
- CHANGELOG entry summarises the shift for external consumers.

## Tests

- Markdown lint + link check covering updated docs.
- Optional spell check pre-commit hook run.
- Manual doc review sign-off captured in PR checklist.
- Honour RED → GREEN → REFACTOR by landing lint failures first, applying minimal
  doc fixes, then refactoring link structure once checks pass.
