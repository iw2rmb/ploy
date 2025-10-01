# IPFS Artifact Publishing
- [x] Done (2025-09-26)

## Why / What For
Bridge the snapshot toolkit to real artifact storage by uploading captured datasets to IPFS when a workstation gateway is available, fulfilling the remaining item on the SHIFT design doc Next Steps.

## Required Changes
- Build an IPFS gateway-backed `ArtifactPublisher` that streams payloads to `/api/v0/add` and returns the gateway-provided CID.
- Teach the CLI snapshot registry loader to inject the gateway publisher when discovery reports an IPFS gateway, retaining the in-memory fallback for offline runs.
- Surface the returned CID in CLI output and metadata structures so downstream systems can reuse snapshot artifacts.

## Definition of Done
- `ploy snapshot capture` uploads artifacts to the discovery-provided IPFS gateway and reports the returned CID.
- When discovery omits a gateway the deterministic in-memory behaviour remains unchanged.
- Design docs and README note the new capability and no longer list IPFS publishing as a TODO.

## Tests
- `go test ./internal/workflow/snapshots` verifies the gateway publisher happy path and non-200 failures via an `httptest` server.
- `go test ./cmd/ploy` exercises the CLI integration end-to-end with a temporary snapshot catalog and asserts that the CID from the gateway appears in command output.
- Repository-wide `go test -cover ./...` maintains ≥60% coverage overall and ≥90% in the snapshot package.
