# Documentation Cleanup
- [x] Done (2025-09-26)

## Why / What For
Align repo docs with the Grid-centric operating model and remove references to Nomad-era workflows.

## Required Changes
- Update `docs/LANES.md`, `docs/SNAPSHOTS.md`, `docs/MANIFESTS.md`, and README entries to reference the new CLI and Grid integration points.
- Add design doc cross-links and migration notes for teams adopting the new flows.
- Ensure CHANGELOG highlights the stateless CLI milestone and Nomad removal.

Status: README now anchors the doc set around the stateless CLI, cross-linking the design doc and refreshed guides. Lanes, manifests, and snapshots docs reference Grid/JetStream pathways, and a top-level documentation matrix in `docs/DOCS.md` steers contributors to the right guide. CHANGELOG captures the documentation cleanup milestone with concrete dates.

## Definition of Done
- All referenced docs describe the JetStream/Grid workflow and omit Nomad/Consul/Traefik guidance.
- Cross-links between design doc, roadmap tasks, and docs are in place.
- CHANGELOG entry summarises the shift for external consumers.

## Tests
- Markdown lint + link check covering updated docs.
- Optional spell check pre-commit hook run.
- Manual doc review sign-off captured in PR checklist.
