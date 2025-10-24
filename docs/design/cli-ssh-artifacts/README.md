# CLI SSH Artifact Handling

- Status: Draft
- Owner: Codex
- Created: 2025-10-24
- Sequence: 3 of 3 (`SSH Transport Pivot`)

## Summary
Enable repository uploads and report downloads over the SSH transport introduced for CLI control-plane calls. Provide simple, reliable file transfer without relying on HTTP endpoints.

## Goals
- Reuse SSH control connections to move artifacts (SFTP, SCP, or custom channel).
- Keep CLI UX close to existing commands (`ploy upload`, `ploy report`).
- Support large payloads and resumable transfers where feasible.

## Non-Goals
- Replacing tunnel setup or control-plane API calls (handled in other design docs).
- Building new object store abstractions beyond SSH-forwarded services.

## Current State
- Artifact transfer occurs via HTTPS endpoints behind the beacon.
- No SSH-based file movement exists.

## Proposed Changes
1. **SFTP/SCP Subsystem**
   - Enable SFTP (preferred) or `scp -T` subsystem on nodes.
   - Restrict file paths to per-job workspaces via `authorized_keys` `command=` wrappers or server-side enforcement.
2. **CLI Integration**
   - Extend `sshtransport` with `CopyTo` and `CopyFrom` helpers using the existing control socket.
   - Update CLI commands to stream data through those helpers.
3. **Resumable / Large Transfers**
   - Optional: support `rsync` over SSH or chunked transfer when size exceeds threshold.
4. **Security & Auditing**
   - Log all transfers with job IDs and calling key fingerprints.
   - Enforce quotas/limits to prevent abuse.

## Work Plan
1. Add SFTP server subsystem config and tests on nodes.
2. Implement CLI file transfer helpers leveraging tunnel manager.
3. Update CLI commands and docs to use SSH-based transfers.
4. Add integration tests covering upload/download success, permissions, and large files.

## Testing Strategy
- Unit: mock SFTP client interactions, ensure path scoping.
- Integration: test uploading/downloading fixtures via SSH in CI.
- Stress: large file transfer test to validate throughput and error handling.

## Documentation
- Update CLI manuals with new transfer flow and troubleshooting.
- Document SFTP server configuration, path policies, and monitoring.

## Dependencies
- Requires tunnel manager from `cli-ssh-transport`.
- Relies on SSH surface prepared in `controlplane-ssh-surface`.
