# SSH Transfer Slice D — Schema & Operations Documentation

- Status: Draft
- Owner: Codex
- Created: 2025-10-26
- Parent: `docs/design/cli-ssh-artifacts/README.md`

## Summary
Document the end-to-end SSH transfer workflow (slots, artifact store, registry, CLI) so operators can run and troubleshoot it. This slice focuses on API references, runbooks, migration steps, and environment configuration updates once slices A–C land.

## Goals
- Update `docs/next/api.md` with request/response examples for `/v1/transfers/*`, `/v1/artifacts/*`, and `/v1/registry/*`.
- Refresh `docs/next/ipfs.md`, `docs/next/devops.md`, and runbooks to describe the SFTP subsystem, slot lifecycle, pin monitoring, and recovery procedures.
- Provide migration guidance for operators moving from direct IPFS uploads to the control-plane-managed flow (env vars, CLI changes, rollout steps).
- Add FAQ/troubleshooting entries (resume uploads, digest mismatches, stuck pins, cleaning staged files).

## Non-Goals
- Implementing any code (covered in slices A–C).

## Plan
1. **API Reference** — Extend `docs/next/api.md` with detailed sections covering transfer slot endpoints, artifact CRUD, and registry uploads/downloads, including auth scopes and error payloads.
2. **Operations Guides** — Update `docs/next/devops.md` and runbooks with:
   - Steps to enable the SFTP subsystem, guard binary, and service units.
   - Monitoring guidance (Prometheus metrics, logs) and remediation flows.
3. **Migration How-To** — Author a new section (or dedicated doc) explaining how to roll out the new CLI commands, retire direct IPFS credentials, and backfill existing artifact metadata into the store.
4. **FAQ** — Add troubleshooting entries (resume CLI uploads, handle digest mismatch, interpret pin states, cleaning abandoned slots).

## Deliverables
- Updated docs checked into `docs/next/*` and linked from the CLI README.
- Changelog entries summarising the new transfer workflow and operator impact.

## Risks
- Docs must stay synchronized with implementation; plan to review them at the end of slices A–C.
