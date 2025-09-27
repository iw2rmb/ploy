# Knowledge Base Catalog

The workstation knowledge base catalog powers the Mods advisor by default when running `ploy workflow run`. Populate `catalog.json` in this directory with the JSON format described in `docs/design/knowledge-base/README.md`.

## Format
- `schema_version`: version string (e.g., `2025-09-27.1`).
- `incidents`: array of incidents containing:
  - `id`: unique identifier.
  - `errors`: textual fragments used for fuzzy matching.
  - `recipes`: Mods recipes suggested for remediation.
  - `summary`: short planner summary.
  - `human_gate`: whether the Mods plan should force a human review stage.
  - `playbooks`: optional human playbooks to surface.
  - `recommendations`: list of `{source, message, confidence, recipes, artifact_cid}` entries.

Leave the file absent when no catalog is available; the CLI falls back to the previous Mods behaviour automatically.
