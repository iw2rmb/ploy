# Documentation Cleanup
- [ ] Pending

## Why / What For
Align repo docs with the Grid-centric operating model and remove references to Nomad-era workflows.

## Required Changes
- Update `docs/LANES.md`, `docs/TESTING.md`, mods guides, and README entries to reference the new CLI and Grid integration points.
- Add design doc cross-links and migration notes for teams adopting the new flows.
- Ensure CHANGELOG highlights the stateless CLI milestone and Nomad removal.

## Definition of Done
- All referenced docs describe the JetStream/Grid workflow and omit Nomad/Consul/Traefik guidance.
- Cross-links between design doc, roadmap tasks, and docs are in place.
- CHANGELOG entry summarises the shift for external consumers.

## Tests
- Markdown lint + link check covering updated docs.
- Optional spell check pre-commit hook run.
- Manual doc review sign-off captured in PR checklist.
