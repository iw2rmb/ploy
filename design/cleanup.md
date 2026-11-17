# Disk pressure

## Summary

Job-boundary cleanup can skip busy nodes and cannot guarantee enough free
space for the next job. Add local capacity checks and pressure recovery.

## Scope

Extend [job-boundary cleanup](../docs/node-maintenance.md#job-boundary-cleanup).
The existing controller owns job hooks, the one-hour cooldown, maintenance
locking, and workspace deletion. The server owns 6h/24h retention and
retained-resume expiry.

The owning worktrees are `ploy` and `deploy`
(`/Users/v.v.kovalev/@gitlab/ploy/deploy`). The node owns capacity decisions.
Deploy supplies capacity configuration. The server keeps run state and
operator drains.

Extend the before-job hook. Add no cleanup hook to the claim loop. Preserve
the existing retention periods, deletion guards, and artifact boundary.
Whole-run deletion, disk reservations, and reclaimed-byte reporting remain
outside this DD. Reuse existing disk-space diagnostics.

## Observed Constraint

- `collectStorageDiagnostics` reports capacity but does not block execution.
- Job-boundary cleanup skips attempts during its cooldown or when the exclusive
  maintenance lock is busy. Overlapping jobs can prevent cleanup indefinitely.
- `RunSlot` combines a concurrency permit with a shared maintenance lock.
  A job waiting for pressure recovery must release its shared lock so cleanup
  can acquire exclusive access.

## Target Contract

### Capacity check

Before job execution, read fresh capacity through `collectStorageDiagnostics`.
Check every required storage path. A failed probe blocks new execution.
Keep the check in the before-job hook for claimed jobs and direct starts.

Deploy configures a low free-byte threshold and a higher recovery target.
Validate `0 < low < target` against storage capacity. Size these values from
concurrent job growth, image extraction, and cleanup delay.

At or above the low threshold, normal execution can continue unless pressure
recovery is pending. Below the threshold, the controller starts pressure
recovery. Server heartbeat values do not control this decision.

### Pressure recovery

The existing controller owns one local pressure-recovery operation. Waiting
jobs retain their concurrency permits but release their shared maintenance
locks. They perform no hydration, pulls, or writes. Occupied permits limit
further claims through the existing concurrency mechanism.

Running jobs retain their shared locks through finalization. Pressure recovery
waits for the exclusive lock while those jobs finish. New execution remains
blocked. Operator drains and host maintenance ownership remain unchanged.

Pressure recovery bypasses the one-hour cleanup cooldown. It uses the
existing cleanup owner and maintenance lock. It may relax Docker age
filters for otherwise unused data. It never shortens workspace retention or
changes container, resource, lease, and volume safeguards.

Read fresh free space after each cleanup phase. Resume waiting jobs only when
all required paths reach the recovery target and required cleanup succeeds.
Each job reacquires its shared lock before execution.

If eligible data is exhausted, report the constrained path and keep execution
blocked. Retry with bounded backoff while waiting jobs remain. Use their
existing cancellation handling to stop waiting. Do not loop without delay or
delete protected data to reach the target. Release every cancelled job's
concurrency permit through the existing lifecycle.

On process restart, require the recovery target at the first before-job check.
This avoids a persisted pressure flag. It can delay execution after a healthy
restart when free space lies between the low threshold and recovery target.
Startup reconciliation still precedes new execution.

Already running jobs can consume more space. These checks do not guarantee
completion of jobs with unbounded disk use.

### Reporting

Use existing disk-space diagnostics and error logs. Report the constrained
path and why execution remains blocked. Add no reclaimed-byte counters or
before/after reclamation reports.

## Enforcement

| Concern | Existing authority to extend |
| --- | --- |
| Capacity checks and waiting jobs | Controller and before-job hook |
| Free-space observations | `collectStorageDiagnostics` |
| Recovery exclusion | Existing cleanup owner and maintenance lock |
| Policy values | Node configuration and deploy configuration |

The node owns pressure recovery. Server diagnostics report capacity; they do
not authorize cleanup or resume execution.

## Implementation Slice

Deliver capacity enforcement across both worktrees:

- Add validated low-threshold and recovery-target configuration.
- Extend the before-job hook with fresh storage checks.
- Coordinate waiting jobs through the existing controller and cleanup owner.
- Bypass cooldown during pressure recovery while preserving deletion guards.
- Preserve recovery-target enforcement across cancellation and restart.
- Update node and deployment documentation.

The completed slice blocks job execution while storage remains insufficient.
It adds no cleanup trigger to claims and no server drain changes.

## Completion

Run focused node lifecycle, storage, and deploy tests. Verify these outcomes:

- Claimed jobs and direct starts check capacity before execution.
- Required-path probe failure blocks execution, including across separate filesystems.
- Pressure recovery bypasses cooldown and waits for active jobs without deadlock.
- Waiting jobs hold no shared lock and retain balanced concurrency permits.
- Cleanup below the recovery target does not release waiting jobs.
- Cancellation releases waiting jobs. Retry backoff prevents a tight loop.
- Restart requires the recovery target even between the low threshold and target.
- Recovery preserves 6h/24h retention, operator drains, and the existing
  container, resource, lease, volume, and artifact safeguards.
- Existing diagnostics report free space without new reclaimed-byte reporting.

Use disposable storage and Docker instances for destructive tests.

## References

- [Node maintenance](../docs/node-maintenance.md).
- [Storage observations](../internal/nodeagent/storage.go),
  [controller](../internal/nodeagent/controller.go),
  [maintenance lock](../internal/nodeagent/maintenance.go).
- Deploy: node configuration, Compose, installer, and service tests.
