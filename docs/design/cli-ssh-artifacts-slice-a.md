# SSH Transfer Slice A — Artifact Metadata Backend

- Status: Implemented
- Owner: Codex
- Created: 2025-10-26
- Parent: `docs/design/cli-ssh-artifacts/README.md`

## Summary
Build the persistent control-plane surface for artifact uploads triggered via SSH slots. Slot commits must publish payloads into IPFS Cluster, store CID/digest metadata in etcd, and back `/v1/artifacts` CRUD APIs so operators can list, inspect, and delete artifacts.

## Goals
- Implement `internal/controlplane/artifacts` for etcd-backed metadata (CID, digest, size, job/stage, replication hints, timestamps).
- Update `/v1/artifacts/*` handlers to use the store and return structured JSON instead of 501 stubs.
- When an upload slot commits, publish the staged payload using the existing IPFS Cluster client and persist the returned CID/digest.

## Non-Goals
- Registry (OCI) storage (Slice B).
- CLI parity (`ploy artifact *` using HTTP) and pin-state monitoring (Slice C).
- Operator documentation (Slice D).

## Plan
1. **Artifact Store** — Create `internal/controlplane/artifacts/store.go` encapsulating etcd layout (`artifacts/<id>`). Persist metadata plus a derived index keyed by job/stage for quick lookups.
2. **Handlers** — Replace `handleArtifactsList`, `handleArtifactsSubpath`, and `handleArtifactsUpload` with logic that calls the store, validates pagination, and surfaces proper status/error codes.
3. **Slot Integration** — Extend the transfer manager so that `Commit` streams the staged file into the IPFS publisher, captures the returned CID/digest, and feeds it into the artifact store transactionally.
4. **Deletion & GC Hooks** — Provide a soft-delete flag plus TTL metadata to support future GC. For now, `/v1/artifacts/{id}` DELETE removes the etcd entry and marks the staged file for cleanup.

## Implementation Notes
- `internal/controlplane/artifacts` now provides the etcd-backed metadata store with job/stage indexes plus soft-delete semantics.
- `/v1/artifacts/*` handlers in `internal/api/httpserver` stream uploads into the configured IPFS Cluster client, record digests, and expose list/get/delete APIs with pagination and scope enforcement.
- `internal/controlplane/transfers.Manager` publishes committed SSH slots via the same IPFS client and persists metadata so download flows can reuse the stored CIDs.

## Testing
- Unit tests for the store (create/list/get/delete, filtering by job/stage, concurrent updates).
- Handler tests covering list pagination, not-found errors, delete success, and bad input.
- A transfer flow test that mocks the IPFS client, commits a slot, and verifies metadata persisted.

## Risks / Follow-ups
- Publishing large payloads inside the control-plane process may block the HTTP handler. Consider async workers if latency becomes a problem.
- Need to coordinate with GC work so deleted artifacts eventually purge from IPFS.
