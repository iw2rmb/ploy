# Backbone Foundation 1.1 – etcd Layout Alignment

**Status**: Completed on 2025-10-24 — scheduler persists the expanded etcd schema with watchers and tests in place.

## Why
- Roadmap slice [1.1](../../next/roadmap.md) mandates persisting expanded scheduler metadata and watchers so etcd remains authoritative.
- [docs/next/etcd.md](../../next/etcd.md) defines the control-plane schema; scheduler writes must comply so downstream services and CLI clients remain consistent.

## What to do
- Extend [internal/controlplane/scheduler](../../../internal/controlplane/scheduler) to persist `expires_at`, retention bundle references, lease metadata, and node capacity snapshots on every job mutation.
- Add focused watchers for `leases/jobs`, `gc/jobs`, and `nodes/<node-id>/status` so the scheduler reconciles expirations and node health without polling.
- Document the schema contract with table-driven tests in [internal/controlplane/scheduler/scheduler_test.go](../../../internal/controlplane/scheduler/scheduler_test.go), covering renders with the new fields.
- After verification, flip roadmap item 1.1 to checked in [docs/next/roadmap.md](../../next/roadmap.md) and update the queue entry to mark the slice complete.

## Where to change
- [internal/controlplane/scheduler/scheduler.go](../../../internal/controlplane/scheduler/scheduler.go) — persist additional metadata inside transaction helpers and watcher callbacks.
- [internal/controlplane/scheduler/types.go](../../../internal/controlplane/scheduler/types.go) — extend job and lease structs with the new fields; ensure JSON/etcd encoding tags stay stable.
- [internal/controlplane/scheduler/scheduler_test.go](../../../internal/controlplane/scheduler/scheduler_test.go) — extend fixtures and assertions documenting the etcd schema.
- [docs/next/etcd.md](../../next/etcd.md) — confirm examples reflect the persisted shape after code changes.

## COSMIC evaluation
| Functional process | E | X | R | W | CFP |
|--------------------|---|---|---|---|-----|
| Job state persistence (added fields) | 0 | 0 | 0 | 1 | 1 |
| Lease/node watcher setup (new subscriptions) | 1 | 0 | 0 | 0 | 1 |
| TOTAL | 1 | 0 | 0 | 1 | 2 |

_Assumptions_: Treat storing multiple attributes in the existing job record as one Write against the `mods/<ticket>/jobs/<job-id>` data group. Watchers reuse existing etcd clients; registering new watches counts as an added Entry triggered by etcd change events. No additional Exits are required because existing logging and metrics surfaces already cover success/failure cases.

## How to test
- `go test ./internal/controlplane/scheduler`
- Manual etcd smoke: run the scheduler against a dev cluster, enqueue a job, and inspect `mods/<ticket>/jobs/<job-id>` plus watcher-triggered updates to validate persisted fields and reconciliation.
