# Node-triggered cleanup

## Summary

The host cleanup timer can miss idle intervals between jobs. The node will
attempt cleanup before admitting new work, using the existing maintenance
boundary and deletion policies.

## Scope

The owning worktrees are `ploy` and `deploy`
(`/Users/v.v.kovalev/@gitlab/ploy/deploy`). Transfer cleanup decisions and
execution to the node. Deploy provisions storage and runtime permissions.
Retire the host cleanup service, timer, script, and installer start calls.

This DD has no dependency on [workspace retention and disk pressure](cleanup.md).
That DD extends this boundary with the 6h/24h workspace policy, retained-resume
expiry, capacity thresholds, recovery targets, and reclamation measurements.
Those changes are not part of this slice.

Preserve the existing `PLOY_NODE_CLEANUP_AGE` policy, successful-run
`runtime-share` eligibility, and optional volume-pruning setting. The default
prohibition on broad volume pruning remains. Preserve host maintenance for
node updates and Gradle dependency publication, job concurrency, sticky
placement, startup recovery, and independent registry auth.

Do not add a cleanup daemon, timer, job queue, scheduler, or host command
service. Do not add another deletion owner or active-run registry.

## Observed Constraint

- Deploy's `ploy-node-cleanup.timer` starts cleanup hourly. The host helper
  waits up to 300 seconds for the exclusive maintenance lock. A busy node can
  miss that window and continue processing work until the next timer event.
- `ClaimManager.claimAndExecute` acquires a `RunSlot` before requesting work.
  `runController.AcquireSlot` takes a shared maintenance lock. Exclusive
  cleanup inside that slot would wait on its own lock.
- The direct-start handler also acquires a `RunSlot`. Startup reconciliation
  attaches monitors to existing work. Cleanup cannot infer inactivity before
  that reconciliation.
- `ploy-node-cleanup` owns Docker pruning, old Git-clone removal, and aged
  successful-run `runtime-share` removal. It preserves whole workspaces,
  failed/cancelled run data, and durable artifacts.
- The host script requires tools unavailable in the node image. The node has
  Docker clients, path helpers, controller state, and maintenance-lock access,
  but no host command-dispatch path.

## Target Contract

### Trigger and ordering

The node runs a cleanup admission checkpoint before every new claim and before
acquiring that job's `RunSlot`. Direct job starts use the same boundary.
Startup recovery attaches to existing work before normal cleanup can classify
local data as unused. Attaching to recovered work must not invoke the new-work
checkpoint or wait for cleanup of that same work.

The checkpoint serializes cleanup with new local admission. Existing jobs
retain their shared locks through finalization and runtime cleanup. Host
updates and dependency publication retain the same shared/exclusive lock
contract. Never create or replace the lock inode.

A node attempts cleanup through the existing claim loop, including its normal
no-work polling. It does not introduce a separate periodic trigger. Existing
claim-loop retry and backoff handle transient failures.

### Normal cleanup and busy nodes

Normal cleanup uses the existing revision-owned drain before attempting the
exclusive maintenance lock. Preserve preflight validation and operator-owned
drains. A normal checkpoint does not wait for running jobs to finish.

If the exclusive lock is unavailable, perform no deletion. Conditionally
release only a fresh drain acquired by this checkpoint, then allow normal
admission checks to continue. If drain ownership changed, preserve the newer
state. Direct starts must not bypass an operator or maintenance drain.

If the exclusive lock is acquired, recheck drain ownership and runtime
activity before deletion. Restore only the operation's own drain after a
successful pass. Preserve existing cleanup ownership on failure so a later
checkpoint can recover it.

This checkpoint must not serialize all jobs behind a full global prune.
Sustained concurrent activity can still defer normal cleanup. Retained
workspaces and low-space admission remain limitations until the follow-up DD.

### Recovery of failed cleanup

An existing cleanup-owned drain is recovery work, not a normal busy skip.
Recover it before admitting new work. Observe the revision, obtain the
exclusive lock, renew the revision, and recheck ownership and runtime activity.
Only a successful existing-policy cleanup pass can release that drain.

On timeout or cancellation while waiting, a fresh acquisition releases only
its own revision. A resumed operation preserves the inherited drain. Reuse
the existing cleanup ownership and revision-renewal protocol across node
restart. Preserve operator reassertions and other host maintenance ownership.

The node's authenticated identity may operate only its own cleanup drain
through the existing control-plane API. Preserve authorization for host
maintenance callers. Job containers receive no administrator token.

### Deletion policy and ownership

The node becomes the sole owner of the current host cleanup operations:

- Remove eligible stopped containers without force. Preserve the existing
  anonymous-volume removal behavior.
- Prune unused images and build cache using the configured age filter.
- Preserve the existing optional volume-pruning setting and its disabled
  default.
- Remove old Git clones using the existing eligibility policy.
- Remove aged successful-run `runtime-share` only after the existing run,
  current-attempt job, path, mount, container-reference, and completion checks.

Keep the existing per-job runtime sweeper as the owner of abandoned job
runtime directories. Preserve workspace directories, failed/cancelled
`runtime-share`, durable job files, `share`, remote artifacts, and tool-owned
cache internals in this slice.

Use native node storage and Docker APIs. Preserve authoritative container
inspection, job-resource checks, and inactive BuildKit lease accounting.
Deploy must expose the runtime inspection required by those checks. Unknown
activity or unavailable inspection blocks deletion. Never remove containerd
leases directly.

Log whether the checkpoint executed cleanup, deferred because of activity, or
failed. A deferred pass must not be reported as completed reclamation.

## Enforcement

| Concern | Existing authority to extend |
| --- | --- |
| New-work checkpoint | `ClaimManager`, `runController`, direct-start handler |
| Local activity | `RunSlot`, startup recovery, existing controller synchronization |
| Global exclusion | Existing drain API, cleanup ownership, maintenance lock |
| Deletion | Node Docker clients and path helpers; transferred host policies |
| Deployment | Node image/Compose permissions and host service installer |

Retire the host deletion implementation when node cleanup takes ownership.
Keep the host maintenance helper for updates and dependency publication.
No fallback may leave both host and node cleanup enabled.

## Implementation Slice

Deliver the ownership and trigger transfer across both worktrees as one
self-sufficient slice:

- Add the node checkpoint before claims and direct starts, outside the new
  job's shared lock. Preserve recovery attachment and concurrency.
- Transfer current deletion policies and activity checks to native node code.
- Extend existing authorization so the node can manage only its own cleanup
  drain. Preserve failed-cleanup recovery and revision fencing.
- Update the node image, Compose access, and installer. Quiesce active host
  cleanup before replacement. Disable and remove its timer, service, script,
  and installer calls when the participating node takes ownership.
- Update shipped maintenance and deployment documentation.

The healthy exit is an independently deployable node checkpoint with the same
eligible deletion set, protected active work, and no timed host cleanup owner.
The 6h/24h workspace policy and disk-pressure controls remain deferred to the
follow-up DD.

## Completion

The Ploy and deploy changes must build and pass focused lifecycle, API,
maintenance, and service tests. Verify these boundaries:

- A claim and a direct start pass through the checkpoint before their shared
  lock. Recovery attachment cannot deadlock on the checkpoint.
- An idle node performs existing-policy cleanup before new work starts.
- A busy node performs no global deletion and retains job concurrency. A
  skipped fresh checkpoint leaves no owned drain behind.
- Failed-cleanup recovery renews its revision under the exclusive lock.
  Timeout, cancellation, restart, and operator reassertion preserve ownership.
- Existing Docker, Git-clone, and successful `runtime-share` eligibility
  remains unchanged. Workspaces and durable artifacts remain protected.
- Disposable Docker integration proves exclusion from pulls and finalization,
  authoritative state/lease checks, and retired host cleanup wiring.

Do not validate destructive concurrency behavior on production nodes.

## References

- [Node maintenance](../docs/node-maintenance.md).
- [Claim lifecycle](../internal/nodeagent/claimer_loop.go),
  [controller](../internal/nodeagent/controller.go),
  [direct starts](../internal/nodeagent/handlers.go).
- [Maintenance lock](../internal/nodeagent/maintenance.go),
  [startup recovery](../internal/nodeagent/crash_reconcile_startup.go),
  [runtime sweeper](../internal/nodeagent/job_runtime_cleanup.go).
- Deploy: `services/bin/ploy-node-cleanup`, `ploy-node-maintenance`, cleanup
  service/timer, Compose, installer, and service tests.
