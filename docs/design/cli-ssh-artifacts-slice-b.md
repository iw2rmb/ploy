# SSH Transfer Slice B — Registry (OCI) Backend

- Status: Draft
- Owner: Codex
- Created: 2025-10-26
- Parent: `docs/design/cli-ssh-artifacts/README.md`

## Summary
Provide first-class OCI registry semantics over the SSH transfer pipeline so container artifacts can be uploaded via `/v1/registry/*`. This slice covers manifest/blob metadata, upload sessions, and reusing SSH slots for blob staging.

## Goals
- Define etcd schema for repositories, manifests, and blobs (content-addressed, with media type, size, references).
- Implement `/v1/registry` handlers (uploads, manifests, blobs, tags) per the Docker Registry v2 API subset we expose.
- Reuse the slot manager for blob payload staging: start upload → stage via SSH slot → PATCH append → PUT commit → publish to IPFS and persist metadata.

## Non-Goals
- Pin-state metrics/CLI work (Slice C) and documentation updates (Slice D).
- Artifact list/read/delete (Slice A handles that path).

## Plan
1. **Schema & Store** — Create `internal/controlplane/registry/store.go` to manage etcd documents for blobs/manifests/tags. Support CAS semantics when linking manifests to tags.
2. **Upload Flow** — Extend the transfer manager with "blob" slot types that enforce digest verification during commit. Map Docker Registry verbs to slots (POST start, PATCH append metadata, PUT finalize).
3. **Handlers** — Replace the current 501 responses in `handleRegistry*` with real implementations: manifest fetch/write/delete, blob upload/download/delete, tag listing.
4. **IPFS Publish** — On blob commit, publish payload via IPFS Cluster, persist the resulting CID, and update blob status to `available`. Blobs referenced by manifests must be validated before the manifest write succeeds.

## Testing
- Table-driven handler tests for each route (start upload, patch append, finalize, manifest fetch, tag list, delete).
- End-to-end registry test using a small OCI fixture (config JSON + layer tar) that exercises the full upload path via a fake transfer manager.
- Error cases: digest mismatch, unknown session, manifest referencing missing blobs, unauthorized scopes.

## Risks / Follow-ups
- Need to ensure compatibility with standard `docker push/pull` clients; document any deviations.
- Storage growth: plan for background GC of unreferenced blobs once Slice C or GC work lands.
