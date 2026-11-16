# Workspace retention and disk pressure

## Summary

Retained workspaces accumulate while cleanup can restore admission without
recovering sufficient free space. Extend node-triggered cleanup with workspace
expiry and capacity checks before new work.

## Scope

Requires [Node-triggered cleanup](node-triggered-cleanup.md). That DD owns the
node admission checkpoint, global maintenance boundary, transferred deletion
policies, and retirement of the host cleanup service and timer.

The owning worktrees are `ploy` and `deploy`
(`/Users/v.v.kovalev/@gitlab/ploy/deploy`). Ploy owns retention and capacity
checks. Deploy supplies capacity configuration through the node deployment.

Delete only `workspace` and `runtime-share`: six hours after successful run
completion, or 24 hours after failed/cancelled run completion. Preserve
`jobs`, `share`, logs, patches, outputs, artifact records, and remote blobs.
Whole-run deletion and durable-artifact retention remain deferred and require
a separate preservation contract.

Preserve job concurrency, sticky placement, startup recovery, tool-owned cache
internals, and the default prohibition on broad volume pruning. Do not add
another cleanup owner, active-run registry, scheduler, or per-job disk
reservation system.

## Observed Constraint

- `collectStorageDiagnostics` reports capacity but does not gate claims.
  Existing cleanup has no recovery target or aggregate reclaimed-byte result.
- `cleanupJobRuntime` and the abandoned-runtime sweep remove only job
  `cache`, `home`, `staging`, and `tmp`. Current run retention removes only
  aged successful `runtime-share`; whole workspaces remain.
- `prepareStickyWorkspace` requires local state for retained resume.
  `resumeFailedJobs` has no retention deadline and reuses the attempt and job
  IDs. `RestartRun` serializes state changes with the existing run-row lock.
- `StopRun` removes job-map entries before execution necessarily finishes.
  The job map alone cannot prove that a workspace has no remaining writer.
- `repoArtifactBundleEntries` excludes workspace data and selects durable
  job files plus `share`. Preserving those paths avoids whole-run artifact
  finalization in this slice. Stored patches are not a complete workspace
  backup.

## Target Contract

### Workspace retention

Eligibility belongs to the current run, because successor jobs share its
workspace. A completed job does not make an unfinished run eligible.

| Current run status | Workspace retention from `finished_at` |
| --- | --- |
| `Success` | 6 hours |
| `Fail`, `Cancelled` | 24 hours |
| Nonterminal or unknown | Preserve |

The node deletes only canonical `workspace` and `runtime-share` children of
its configured run directory. Missing completion time, unavailable run state,
symlinks, path escapes, mounts, or container references preserve the data.
Directory access and modification times do not determine eligibility.

Local admission and execution ownership exclude pending claims, hydration,
recovery, and finalization for the selected run during deletion. Cancelling a
job does not release that protection before its execution exits. Unrelated
jobs can continue while eligible workspace data is deleted. Extend the
existing controller synchronization; do not maintain another active-run list.
A shared maintenance lock excludes conflicting host maintenance during this
local operation.

Immediately before deletion, recheck current status, attempt, `resume_count`,
and `finished_at`. Retained resume can change execution identity without
changing attempt or job ID. A changed identity invalidates the observation.

### Retained resume and restart

One retention policy governs cleanup eligibility and retained-resume expiry.
The control plane evaluates expiry through its existing run read and restart
boundaries. Cleanup eligibility uses a short transaction with the same run-row
lock as `RestartRun`. Both decisions evaluate server wall-clock time after
acquiring that lock. Transaction-start time and an unlocked metadata read
cannot authorize deletion. An in-progress retained resume must commit or roll
back before cleanup observes eligibility.

The node consumes that eligibility with the run identity; it does not shorten
retention using its local clock. Release the database lock before filesystem
deletion. Do not persist a second expiration timestamp or eligibility row.

The `RestartRun` transaction rejects retained resume of a failed or cancelled
run at or beyond 24 hours after completion. This applies even if the files
have not yet been removed. The response directs the caller to full restart.
A retained resume accepted before expiry changes the current run state and
invalidates cleanup eligibility. Expiry must be enforced inside the existing
transaction before jobs or logs are changed.

A full restart remains available after expiry. Local admission prevents new
hydration from overlapping deletion. If a full restart commits after the last
eligibility observation, its execution waits for deletion and reconstructs
missing workspace data through the existing snapshot path. Retained resume
cannot win that expired-run race because the server rejects it.

Keep the existing missing-workspace error for retained resume. Do not silently
convert retained resume into a full restart. Deploy the server expiry contract
before enabling node deletion of failed or cancelled workspace data.

### Capacity and pressure recovery

Extend the admission checkpoint defined by the prerequisite DD. Use
`collectStorageDiagnostics` for fresh observations of every required path.
Configure one low free-byte threshold and one higher recovery target through
node deployment. Validate `0 < low < target` against storage capacity. Size
these values from concurrent job growth, image extraction, and cleanup delay.
Do not add a second SQL threshold based on stale heartbeat data.

Every admission attempt checks free space after workspace retention. A probe
failure blocks new work. If all required paths are healthy and above the low
threshold, admission can continue without global Docker pruning only when no
cleanup-owned recovery is pending. Resume existing recovery before admitting
new work, including after node restart.

Under pressure, stop new admission and enter the prerequisite's revision-owned
cleanup drain and exclusive maintenance boundary. Release any local shared
lock before waiting. Existing jobs finish while admission remains stopped.
Unlike normal opportunistic cleanup, a busy pressure pass does not reopen
admission below the recovery target. Preserve operator and host maintenance
ownership.

Pressure mode can relax the Docker age floor for otherwise unused data. It
never shortens the six-hour or 24-hour workspace retention periods. Preserve
the prerequisite's container/resource/lease checks and existing cache owners.

Measure filesystem free space after each global cleanup phase. Restore only
the operation's own drain after every required path reaches the recovery
target. If eligible data is exhausted, keep admission blocked and report the
constrained path and preserved data classes. Do not loop indefinitely or
remove protected data to reach the target.

Already admitted jobs can consume additional space. This contract limits new
admission; it does not guarantee completion of unbounded jobs.

### Observable result

Each cleanup pass reports free bytes before and after, reclaimed bytes by
phase, and preserved candidates with reasons. An empty eligible set reports
zero reclamation. Report a capacity result separately from successful execution
of the deletion procedure. Identify the run execution identity or drain
revision without exposing credentials.

## Enforcement

| Concern | Existing authority to extend |
| --- | --- |
| Admission | Node checkpoint from the prerequisite DD |
| Retention eligibility | Current run state, run read API, `RestartRun`, `resumeFailedJobs` |
| Local execution exclusion | Controller synchronization, startup recovery, maintenance lock |
| Capacity | `collectStorageDiagnostics`, node configuration, deploy configuration |
| Reclamation | Node directory helpers and existing cleanup phases |

Preserve one owner for each deletion path and the job-runtime sweeper's
ownership. Do not add artifact manifests, artifact finalizers, or a
cleanup-eligibility node endpoint for this workspace-only scope.

## Implementation Slice

After the prerequisite ships, deliver retention and capacity enforcement
across both worktrees as one self-sufficient slice:

- Define shared retention eligibility and enforce the 24-hour retained-resume
  deadline in the existing server transaction and API response.
- Extend local execution protection for workspace deletion, including direct
  starts, cancellation, recovery, and full restart.
- Add workspace and `runtime-share` expiry with the fixed 6h/24h policy.
- Extend storage observations into admission and pressure recovery. Preserve
  recovery-target enforcement after timeout, cancellation, and node restart.
- Deploy the server expiry contract before enabling failed/cancelled workspace
  deletion. Supply validated capacity configuration through deploy.
- Update shipped API, run-resume, deployment, and maintenance documentation.

The healthy exit is bounded workspace retention and admission blocked when
capacity remains insufficient. No duplicate cleanup owner remains.
Durable-artifact and whole-run retention remain deferred.

## Completion

The Ploy and deploy changes must build and pass focused lifecycle, store, API,
and service tests. Verify these boundaries:

- Success at six hours; failure and cancellation at 24 hours; missing or
  changed status preserves workspace data.
- A locally idle run between jobs remains protected. Other runs can execute
  concurrently with eligible workspace deletion.
- Cancellation and recovery retain writer protection until execution exits.
- Retained resume before expiry invalidates eligibility. An uncommitted resume
  spanning the deadline cannot race an unlocked eligibility observation.
  Expired resume fails before state mutation. A racing full restart
  reconstructs missing data.
- `jobs`, `share`, logs, patches, outputs, and remote artifacts survive cleanup.
- Pressure blocks new claims, waits for existing jobs without self-deadlock,
  and preserves operator drains. Probe failure and insufficient reclaimable
  data do not reopen admission. Restarting recovery between the low threshold
  and recovery target preserves the recovery target.
- Required-path checks work across separate filesystems. Cleanup reports zero
  bytes when nothing is eligible and actual free space after deletion.
- Disposable Docker integration verifies pressure recovery while preserving
  the prerequisite's pull/finalization exclusion and resource/lease checks.

Do not validate destructive concurrency behavior on production nodes.

## References

- [Node-triggered cleanup prerequisite](node-triggered-cleanup.md).
- [Node maintenance](../docs/node-maintenance.md).
- [Run resume and restart](../docs/runs.md).
- [Controller](../internal/nodeagent/controller.go),
  [workspace hydration](../internal/nodeagent/workspace_hydration.go),
  [runtime sweeper](../internal/nodeagent/job_runtime_cleanup.go).
- [Storage observations](../internal/nodeagent/storage.go),
  [artifact selection](../internal/nodeagent/execution_artifacts.go),
  [retained resume](../internal/store/resume_run.go),
  [restart transaction](../internal/store/store.go).
- Deploy: node configuration, Compose, installer, and service tests.
