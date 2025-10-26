# Scheduler Package Decomposition

## Why
- [`internal/controlplane/scheduler/scheduler.go`](../../../internal/controlplane/scheduler/scheduler.go) is 1,440 LOC—the largest file in the repo—mixing submission APIs, lease/GC watchers, helpers, and record structs. The size makes reviews slow, increases merge conflicts, and obscures ownership boundaries.
- Watch goroutines (`watchLeases`, `watchGCMarkers`, `watchNodeStatus`) are tightly coupled with queue helpers, making it hard to reason about lifecycle behaviour or extend metrics.
- Record/utility structs live beside business logic, so even cosmetic tweaks trigger huge diffs and increase the risk of accidentally modifying persistence semantics.

## What to do
1. Keep `scheduler.go` focused on `Scheduler`, option compilation, and exported entrypoints (`New`, `Close`) plus thin delegators.
2. Introduce focused files inside `internal/controlplane/scheduler`:
   - `jobs_submit.go` for `SubmitJob`, `ListJobs`, `GetJob`, `RunningJobsForNode`, and mutation helpers that do not depend on leases.
   - `jobs_runtime.go` for `ClaimNext`, `tryClaimOnce`, `Heartbeat`, `CompleteJob`, retry logic, and metrics recording.
   - `leases_watch.go` for `watchLeases`, `handleLeaseExpiry`, and queue requeue helpers.
   - `gc_watch.go` for `watchGCMarkers` plus marker reconciliation helpers.
   - `nodes_watch.go` for `watchNodeStatus`, `applyNodeStatus`, and `captureNodeSnapshot`.
   - `records.go` for `jobRecord`, `bundleRecord`, `retentionRecord`, `nodeSnapshotRecord`, and transformation helpers (`export*`, `normalizeBundleRecords`, retention math).
   - `timeutil.go` for `encodeTime`, `decodeTime`, `snapshotTimestamp`, and map cloning helpers.
3. Ensure each file starts with a brief package-level comment describing its focus; keep shared helpers unexported and colocated with closest consumers.
4. Preserve current behaviour by moving code verbatim, only updating imports/comments when necessary; avoid renaming exported methods.
5. Update `scheduler_test.go` only where needed (helper locations, new file-specific build tags) to keep RED → GREEN cadence limited to the scheduler package.

- [`internal/controlplane/scheduler/scheduler.go`](../../../internal/controlplane/scheduler/scheduler.go) — reduce to struct + delegators.
- New files under `internal/controlplane/scheduler/` as listed above.
- [`internal/controlplane/scheduler/scheduler_test.go`](../../../internal/controlplane/scheduler/scheduler_test.go) — adjust helper references if needed.
- Upstream docs already aligned; no control-plane API surface changes.

## COSMIC evaluation

| Functional process | E | X | R | W | CFP |
|--------------------|---|---|---|---|-----|
| Restructure scheduler entrypoints into focused files | 0 | 0 | 0 | 0 | 0 |
| TOTAL              | 0 | 0 | 0 | 0 | 0 |

Assumptions: this work only shuffles internal Go code; it does not add new user-visible functionality or data movements, so COSMIC impact is nil.

## How to test
- `go test ./internal/controlplane/scheduler` (covers watchers, queue logic, and regression suite).
- If scheduler tests touch shared helpers, rerun `make test` after the package passes locally to keep the control-plane coverage target (≥60% overall, ≥90% runner packages).
