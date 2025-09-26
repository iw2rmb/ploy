# Grid Workflow Client
- [x] Done (2025-09-26)

## Why / What For
Replace the in-memory Grid stub with the real Workflow RPC client so workstation runs can target the Dev Grid when `GRID_ENDPOINT` is available while still supporting offline slices.

## Required Changes
- Implement an HTTP client (`internal/workflow/grid`) that encodes stage requests, dispatches them to Grid, and captures outcomes.
- Teach `ploy workflow run` to instantiate the real client when `GRID_ENDPOINT` is set and preserve the stub fallback otherwise.
- Record invocation metadata so the CLI can continue printing Aster bundle summaries regardless of the backing Grid implementation.
- Refresh README/design docs to note the new behaviour and future IPFS follow-up.

## Definition of Done
- Workflow stages execute through the Grid Workflow RPC when `GRID_ENDPOINT` is configured; removing the variable restores the in-memory stub.
- CLI surface errors when Grid configuration fails and continues to print Aster summaries using invocation metadata.
- Documentation and changelog highlight the new toggle and roadmap slice completion.

## Tests
- `go test ./internal/workflow/grid` covering request encoding, response handling, and error propagation.
- `go test ./cmd/ploy` asserting Grid client selection for default and configured environments.
- Repository-wide `go test -cover ./...` to maintain ≥60% overall coverage.
