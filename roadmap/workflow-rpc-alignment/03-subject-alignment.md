# JetStream Subject Alignment

- [x] Completed 2025-09-30

## Why / What For

Synchronise webhook and status subject naming with Grid so ticket claims,
checkpoint publication, and build-gate log retrieval operate against the same
streams (`webhook.<tenant>.<source>.<event>`, `jobs.<run_id>.events`).

## Required Changes

- Update `internal/workflow/contracts` to export the new subject patterns and
  adjust ticket/checkpoint helpers accordingly. ✅
- Refresh build-gate log retrieval design/tasks to pull job logs via
  `jobs.<run_id>.events` metadata. ✅ (design updates complete; build-gate
  implementation continues under Roadmap 21).
- Update documentation and changelog entries to reflect the subject migration,
  referencing the Grid docs for consolidation. ✅

## Definition of Done

- Contracts and tests assert the new subject patterns (`jobs.<run_id>.events`,
  `webhook.<tenant>.<source>.<event>`).
- Documentation (design docs, build gate roadmap) references the updated
  subjects.
- Any code that previously referenced `grid.webhook.*` or `grid.status.*` now
  uses the consolidated constants.

## Tests

- Contract tests validating subject derivation for tenants and run IDs. ✅
  (added whitespace trimming coverage in `internal/workflow/contracts`).
- Build-gate metadata/log retrieval tests covering the new status stream. ⏳
  (tracked under Roadmap 21 build-gate execution).

## References

- Ploy Event Contracts design (`docs/design/event-contracts/README.md`).
- Ploy Workflow RPC Alignment design
  (`docs/design/workflow-rpc-alignment/README.md`).
- Grid Workflow RPC design (`../grid/docs/design/workflow-rpc/README.md`).
- Grid Jobs publisher (`../grid/internal/jobs/publisher_jetstream.go`)
  verification notes tracked in the design doc.
