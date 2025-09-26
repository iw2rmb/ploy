# IPFS Artifact Publishing (Roadmap 15)

## Purpose
Provide a workstation-ready pathway for snapshot captures to upload artifacts to IPFS when a gateway is available, while preserving the existing in-memory stub for offline slices. This closes the remaining gap between the snapshot toolkit and the Grid/VPS integration plan called out in `SHIFT.md`.

## Scope
- Applies to the snapshot toolkit (`ploy snapshot capture`) and any other feature that reuses the snapshot registry's `ArtifactPublisher` hook.
- Targets workstation environments; VPS/Grid slices will reuse the same publisher once JetStream wiring resumes.
- Metadata publishing remains a TODO until the JetStream streams are provisioned for snapshots.

## Behaviour
- When ``IPFS_GATEWAY`` is set, the CLI loads the snapshot registry with an IPFS gateway publisher. Artifacts are streamed to `<gateway>/api/v0/add?pin=true` using multipart uploads, and the returned CID is surfaced to the operator.
- Without ``IPFS_GATEWAY``, the registry falls back to the deterministic in-memory publisher so existing tests and offline workflows remain unaffected.
- Errors bubble up with the HTTP status code and any response snippet to help diagnose misconfigured gateways.
- Returned CIDs are forwarded to metadata payloads and printed in the CLI summary alongside fingerprints and diff summaries.

## Implementation Notes
- `internal/workflow/snapshots.NewIPFSGatewayPublisher` constructs a reusable publisher with a 15s HTTP timeout and optional pinning.
- The publisher tolerates newline-delimited JSON responses from the IPFS gateway and extracts the first non-empty `Hash`/`Cid` field.
- `cmd/ploy/main.go` now inspects ``IPFS_GATEWAY`` inside the default snapshot registry loader and injects the gateway-backed publisher when present.
- Existing metadata publishing remains a no-op; the JetStream metadata publisher will replace it in a later slice once streams are available.

## Tests
- `internal/workflow/snapshots/registry_test.go` exercises the gateway publisher against an `httptest` server, validating multipart payloads, `pin=true` query wiring, CID parsing, and non-200 handling.
- `cmd/ploy/main_test.go` runs an end-to-end capture with a temporary snapshot catalog and confirms the CLI switches to the gateway, emitting the CID returned by the test server.
- Repository-wide `go test -cover ./...` continues to enforce the ≥60% coverage bar with `internal/workflow/snapshots` staying above 90%.

## Follow-ups
- Replace the no-op metadata publisher with a JetStream-backed implementation once snapshot streams are available.
- Share the IPFS publisher with the workflow runner once build artifacts begin flowing through the same pipeline.
- Document gateway authentication expectations if a bonded IPFS endpoint requires tokens.
