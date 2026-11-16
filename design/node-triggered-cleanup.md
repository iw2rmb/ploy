# Node-triggered cleanup

## Summary

The host cleanup timer can miss idle intervals between jobs. Move cleanup into
node job hooks. Delete expired workspaces and limit successful cleanup to once
per hour in each node process.

## Scope

The owning worktrees are `ploy` and `deploy`
(`/Users/v.v.kovalev/@gitlab/ploy/deploy`). The node owns cleanup decisions and
execution. Deploy provisions storage and runtime permissions.

Replace host cleanup with hooks before job execution and after finalization.
Include workspace retention: six hours for successful runs and 24 hours for
failed or cancelled runs. Include the matching `--from-failed` deadline.

The server keeps run state, resume rules, and operator drains. Node cleanup
does not acquire or release server drains. Host updates and Gradle dependency
publication keep their existing maintenance protocol.

Preserve Docker and Git-clone deletion policies. Keep optional volume pruning
disabled by default. Preserve job concurrency, sticky placement, startup
recovery, registry auth, durable artifacts, and tool-owned cache internals.

[Disk pressure](cleanup.md) remains a separate follow-up. This slice adds no
free-space threshold or recovery target. Reuse existing disk-space diagnostics
and cleanup error logs. Reclaimed-byte reporting is outside both DDs.

Do not add UDP coordination, persisted cleanup state, a cleanup timer, a
scheduler, a host command service, or an active-run registry.

## Observed Constraint

- Deploy's `ploy-node-cleanup.timer` starts cleanup hourly. The host helper
  waits up to 300 seconds for the exclusive maintenance lock. Busy nodes can
  miss that interval.
- `runController.StartRun` wraps `executeRun` and releases the job's `RunSlot`
  afterward. Both claimed jobs and direct starts use this execution path.
- `RunSlot` holds a concurrency permit and a shared maintenance lock. Cleanup
  cannot acquire the exclusive lock while holding its own shared lock.
- `Agent.Run` starts the HTTP server before claim-loop startup reconciliation
  finishes. Direct starts need protection from incomplete reconciliation.
- Recovered-container monitors acquire shared locks. The node must attach
  these monitors before cleanup can classify local data as unused.
- Host cleanup removes aged successful-run `runtime-share`, but preserves
  workspaces and failed/cancelled run data.
- `prepareStickyWorkspace` needs retained workspace data for `--from-failed`.
  `resumeFailedJobs` has no deadline. `RestartRun` already locks the run row.
- `StopRun` removes job-map entries before execution necessarily finishes.
  The job map alone cannot prove that all writers have stopped.
- `repoArtifactBundleEntries` selects durable job files and `share`. It
  excludes workspace data. Stored patches are not a complete workspace backup.

## Target Contract

### Job hooks

Run the same cleanup function immediately before `executeRun` and after it
returns. The first hook precedes workspace hydration, image pulls, and job
writes. The second follows finalization and per-job runtime cleanup.

Both claimed jobs and direct starts use these hooks. Cleanup adds no hook to
the claim loop. Empty polls do not trigger cleanup.

New work waits for startup reconciliation before acquiring a concurrency
permit. The claim loop already waits for reconciliation. Direct starts must
wait before acquiring `RunSlot`, so recovery can obtain its own permits.
Recovery attachment bypasses this startup wait and the before-job hook.
Recovered jobs retain shared locks through finalization. Their monitors run the after-job hook when
finalization ends.

Startup itself does not trigger cleanup. The first job hook after restart
can attempt cleanup because the in-memory completion time is empty.

### In-memory state

The existing controller owns `cleanupRunning` and `lastCleanupSuccess`.
A mutex protects both fields. Check the fields and set `cleanupRunning` in
one critical section. Do not hold that mutex during cleanup I/O.

A hook skips its cleanup attempt when cleanup is already running or the last
successful cleanup ended less than one hour ago. At one hour, the next hook
can attempt cleanup. The interval creates no timer or deadline.

Clear `cleanupRunning` on every exit, including cancellation and failure.
Update `lastCleanupSuccess` only after a successful pass. A busy lock, failed
inspection, or incomplete cleanup does not start the cooldown. A complete
pass with no eligible data counts as success.

A skipped hook does not authorize execution without a shared maintenance
lock. That lock keeps jobs from executing during cleanup.

The cooldown belongs to one process. A restart or another process can cause
an extra cleanup pass. The host lock still prevents overlap. Different hosts
clean their own storage independently.

### Maintenance lock and failures

Use the existing host maintenance lock for the complete cleanup pass,
including workspace deletion. Keep its inode unchanged.

Before attempting cleanup, release only the hook's own shared lock. Retain
the concurrency permit. Try the exclusive lock without waiting. If another
job or host operation holds the lock, skip this attempt.

Before job execution, reacquire the shared lock after the hook finishes or
skips. If reacquisition fails, do not execute the job. Preserve the existing
job failure and cancellation paths. Keep shared protection throughout job
execution, finalization, and per-job runtime cleanup.

After finalization, release the shared lock before the cleanup attempt.
Release the concurrency permit after the hook finishes. Use a bounded cleanup
context that is independent of the completed job's cancellation. Node shutdown
still cancels cleanup.

Under the exclusive lock, check containers, job resources, and inactive
BuildKit leases. Unknown activity or failed inspection prevents deletion.
Never remove containerd leases directly.

Cleanup errors preserve affected data and produce an error log. They do not
mark the pass successful or create a lasting block on job execution. A later
job hook can retry. Free-space enforcement belongs to the follow-up DD.

Continuous overlapping jobs can prevent cleanup indefinitely. This slice
accepts that limit to preserve concurrency and reuse the existing lock.

### Workspace retention

Eligibility belongs to the current run because successive jobs share its
workspace. A completed job does not make an unfinished run eligible.

| Current run status | Retention from `finished_at` |
| --- | --- |
| `Success` | 6 hours |
| `Fail`, `Cancelled` | 24 hours |
| Nonterminal or unknown | Preserve |

Delete only canonical `workspace` and `runtime-share` children of the run
directory. Preserve `jobs`, `share`, logs, patches, outputs, artifact records,
and remote blobs. Whole-run deletion remains outside this DD.

Preserve data when completion time or run state is unavailable. Reject
symlinks, path escapes, mounted descendants, and container references.
Directory access and modification times do not determine retention.

The exclusive maintenance lock excludes hydration, execution, recovery, and
finalization during deletion. Cancellation must retain shared protection until
the writer exits. Pending jobs cannot write before acquiring their shared lock.

Immediately before deletion, recheck status, attempt, `resume_count`, and
`finished_at`. A changed run identity invalidates the earlier observation.
Keep existing current-attempt terminal-job checks.

### Retained resume and restart

One policy defines retention and retained-resume expiry. Extend the existing
run read API to return cleanup eligibility with run identity. The server
computes eligibility in a short transaction under the same run-row lock as
`RestartRun`. Use server wall-clock time after acquiring the lock.

An in-progress resume must commit or roll back before the eligibility read.
An unlocked read or transaction-start timestamp cannot authorize deletion.
The node uses the returned eligibility; its local clock cannot shorten
retention. Release the database lock before filesystem deletion.

Inside `RestartRun`, reject `--from-failed` at or beyond 24 hours after run
completion. Check expiry before changing jobs or logs. Apply the deadline
even when workspace files still exist. Direct the caller to full restart.
A resume accepted before expiry changes run identity and invalidates cleanup
eligibility.

Full restart remains available. If it commits after the final eligibility
read, new execution waits for the maintenance lock. Full restart reconstructs
missing workspace data through the existing snapshot path.

Keep the existing missing-workspace error for retained resume. Do not silently
convert retained resume to full restart. Add no expiration row, artifact
manifest, or artifact-finalization endpoint.

### Existing deletion policies and deployment

Transfer Docker pruning and old Git-clone removal into native node code.
Preserve their existing eligibility checks, age filters, and volume settings.
Replace successful-only `runtime-share` retention with the fixed policy above.
The per-job sweeper keeps ownership of abandoned job runtime directories.

Deploy supplies the runtime access needed for container and lease inspection.
Deploy the server retention and resume contract before enabling node workspace
deletion.

Stop host cleanup before the node takes ownership. Resolve legacy
cleanup-owned server drains through the old host maintenance helper. Preserve
its revision checks and operator changes. Report unresolved drains and block
the handoff until they are resolved.

Remove the host cleanup service, timer, script, and installer calls at handoff.
Keep the host maintenance helper for updates and dependency publication.
Host and node cleanup must never act as competing owners.

## Enforcement

| Concern | Existing authority to extend |
| --- | --- |
| Job hooks and cooldown | `runController`, `StartRun`, recovered-container monitors |
| Startup protection | Startup reconciliation and direct-start execution |
| Exclusion and concurrency | `RunSlot`, shared/exclusive maintenance lock |
| Retention and resume | Run read API, `RestartRun`, `resumeFailedJobs` |
| Deletion | Node Docker clients and path helpers; transferred host checks |
| Deployment | Node image, Compose permissions, host service installer |

The node owns cleanup. Server run state defines retention eligibility; it does
not schedule cleanup or control local free space.

## Implementation Slice

Deliver hooks and retention across both worktrees as one slice:

- Add shared before/after job hooks and the in-memory one-hour cooldown.
- Adjust `RunSlot` lock handling without losing its concurrency permit.
- Protect startup reconciliation and recovered-job finalization.
- Transfer Docker and Git-clone cleanup with existing activity checks.
- Add 6h/24h workspace retention and the matching retained-resume deadline.
- Deploy the server contract before node workspace deletion.
- Resolve legacy cleanup drains and retire host cleanup.
- Update run-resume, maintenance, API, and deployment documentation.

The completed slice attempts cleanup at job boundaries, preserves active work,
and deletes eligible workspace data. It needs no cleanup calls from the claim
loop and no node drain API permission.

## Completion

Run focused node lifecycle, store, API, and deploy tests. Verify these outcomes:

- Claimed jobs and direct starts run both hooks. Empty polls run neither hook.
- Simultaneous hooks start at most one local cleanup pass.
- Success starts the one-hour cooldown. Failure, cancellation, and a busy lock
  do not. Restart clears the cooldown without allowing overlapping cleanup.
- Hooks never wait for an exclusive lock while holding their own shared lock.
  Jobs reacquire shared protection before execution. Concurrency permits remain balanced.
- With capacity one, a direct start during reconciliation cannot take the
  permit needed by a recovered-container monitor. Recovery bypasses the startup wait.
- Recovered finalization retains writer protection and reaches the after-job hook.
- Retention starts at six hours for success and 24 hours for failure/cancellation.
  Nonterminal runs, unknown state, changed identity, and unsafe paths preserve data.
- An uncommitted resume spanning expiry cannot race deletion. Expired resume
  fails before mutation. Full restart can reconstruct a deleted workspace.
- Durable job files, `share`, artifact records, and remote blobs survive cleanup.
- Cleanup leaves operator drains unchanged. Deployment resolves legacy cleanup
  drains without overriding operator changes.
- Disposable Docker tests verify pull/finalization exclusion, resource/lease
  checks, cross-process locking, and retired host cleanup wiring.

Do not run destructive concurrency tests on production nodes.

## References

- [Node maintenance](../docs/node-maintenance.md), [run resume](../docs/runs.md).
- [Controller](../internal/nodeagent/controller.go),
  [job execution](../internal/nodeagent/execution.go),
  [direct starts](../internal/nodeagent/handlers.go).
- [Startup reconciliation](../internal/nodeagent/crash_reconcile_startup.go),
  [recovered containers](../internal/nodeagent/crash_reconcile_running.go),
  [maintenance lock](../internal/nodeagent/maintenance.go).
- [Workspace hydration](../internal/nodeagent/workspace_hydration.go),
  [retained resume](../internal/store/resume_run.go),
  [restart transaction](../internal/store/store.go).
- Deploy: `services/bin/ploy-node-cleanup`, `ploy-node-maintenance`, cleanup
  service/timer, Compose, installer, and service tests.
