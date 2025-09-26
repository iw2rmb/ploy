# Integration Manifest Schema
- [x] Done (2025-09-26)

## Why / What For
Provide a machine-readable contract for integration manifests so the workflow runner slice and external tooling can share validation logic without embedding Go structs.

## Required Changes
- Publish a JSON Schema describing manifest shape (topology, fixtures, lanes, Aster toggles) alongside the docs.
- Expose the schema through the CLI so downstream tools can fetch it without searching the repo.
- Document the schema location and expectations for authors ahead of the workflow runner wiring slice.

## Definition of Done
- `docs/schemas/integration_manifest.schema.json` exists and encodes the required fields (name, version, summary, topology, fixtures, lanes) with constraints matching the loader.
- `ploy manifest schema` prints the schema path and JSON payload.
- Design doc and manifests documentation reference the schema so roadmap slices stay aligned.

## Tests
- `go test ./internal/workflow/manifests` loads and inspects the schema asset to ensure required fields stay enforced.
- `go test ./cmd/ploy` exercises the CLI command and confirms the schema output contains expected metadata.
- Repository-wide `go test -cover ./...` continues to satisfy ≥60% overall coverage.
