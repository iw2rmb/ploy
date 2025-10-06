# Knowledge Base CLI Ingest

- [x] Done (2025-09-27)

## Why / What For

Mods operators need a workstation-friendly flow to append new incidents to the
knowledge base catalog right after a successful healing run. Surfacing an ingest
CLI keeps the catalog current without manual JSON editing and prepares for
future Grid integration where incidents stream from artifacts automatically.

## Required Changes

- Add a `knowledge-base` top-level CLI command with an `ingest` subcommand for
  appending incidents to `configs/knowledge-base/catalog.json`.
- Parse incident fixtures (JSON) describing one or more incident entries,
  merging them into the catalog while avoiding duplicate IDs and preserving
  schema versioning.
- Produce human-readable CLI output summarising ingested incidents and skipped
  duplicates so operators can confirm catalog updates.
- Update loading utilities to expose catalog write helpers reused by the CLI
  without impacting read-only workflows.

## Definition of Done

- `ploy knowledge-base ingest --from <fixture>` validates input, writes merged
  catalog data atomically, and retains sorted incident IDs.
- Duplicate incident IDs are rejected (or skipped with a warning) without
  corrupting the catalog.
- Knowledge base package exposes writer utilities covered by unit tests.
- Documentation and roadmap references note the new CLI workflow.

## Current Status (2025-09-27)

- `ploy knowledge-base ingest` loads fixtures, merges incidents with duplicate
  safeguards, and writes catalog updates atomically.
- Writer utilities maintain ≥90% package coverage and power both CLI and
  internal helpers.
- Docs and roadmap entries reflect the ingest workflow.

## Tests

- CLI unit tests covering success path (merges incidents), duplicate collisions,
  and missing catalog scenarios.
- Knowledge base unit tests exercising merge/write helpers with fixture data and
  ensuring schema version is preserved.
- Repository-wide `go test -cover ./...` remains ≥60% overall and the knowledge
  base package stays ≥90%.
- Apply RED → GREEN → REFACTOR: add failing ingest tests, layer minimal writer
  logic, then refactor once coverage holds.
