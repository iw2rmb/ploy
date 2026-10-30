# Node cleanup

## Summary

Host maintenance cannot establish an idle interval while job admission remains
open, and age-only retention cannot prevent disk exhaustion.

The admission and maintenance boundary is implemented in
[Node maintenance](../docs/node-maintenance.md). The remaining delivery slices
add pressure-based cleanup (C3) and terminal-run retention (C4).

## Scope

The owning worktrees are `ploy` and `deploy`
(`/Users/v.v.kovalev/@gitlab/ploy/deploy`). Ploy owns claims, job execution,
run state, and artifact records. Deploy owns host maintenance and deletion.

Update the existing drain API, node lifecycle, storage probe, and deploy service
bundle. Cleanup, node updates, and Gradle dependency publication must use the
same maintenance boundary because they already share its lock.

Preserve job concurrency, sticky run placement, build results, independent auth
refresh, tool-owned cache internals, and the default prohibition on volume
pruning. Do not add a maintenance job queue, daemon, second scheduler, parallel
run-state database, or generic resource-reservation system. Docker upgrades,
automatic pull retries, disk expansion, and unrelated container ownership
changes are outside this design.

## Observed Constraint

- Cleanup applies a 24-hour age filter to Docker pruning. Run-directory
  deletion uses access time. It has no free-space target. Node storage probes
  already report configured paths, capacity, inode counts, and probe errors.
- `uploadRepoArtifactsIfPresent` selects durable files through
  `repoArtifactBundleEntries`, but discards successful artifact identifiers
  and only logs upload errors. `artifact_bundles` and the blob store already
  own uploaded bundles. Log-stream `retained` events do not prove that a
  complete durable run bundle exists.

## Target Contract

### Prerequisite: admission and local quiescence

Use the existing revision-owned drain and shared/exclusive host lock described
in [Node maintenance](../docs/node-maintenance.md). Preserve that ordering for
C3 and C4. Whole-run and broad TMPDIR deletion remain disabled until C4.

### C3: Free space controls admission and cleanup

Use one configured policy: a low free-byte threshold and a higher recovery
target. Deploy owns these values and supplies the same values to the node.
Validate `0 < low < target` against storage capacity. Derive production values
from measured extraction size, job growth, concurrency, and cleanup delay;
do not treat a percentage or this incident's reclaimed size as a reservation.

Extend `collectStorageDiagnostics` for both diagnostics and a fresh pre-claim
check. Any required-path probe failure or value below `low` blocks a new claim
before the request is sent. This is a local capacity check, not another drain
flag. Do not add a second threshold evaluator to SQL using stale heartbeats.

Use the existing cleanup timer and service to check pressure at a documented,
bounded interval. Host decisions use fresh node storage diagnostics; missing
or stale results cannot authorize admission restoration. A post-prune decision
requires a sample collected after that prune phase. Keep an explicit
normal age-based mode and a pressure mode in the same service.

Pressure mode enters the maintenance boundary, then removes stopped containers and unused images
without the normal age floor. Recheck actual filesystem free space after each
phase. Once every required path reaches `target`, cleanup can restore only its
own drain. If eligible data is exhausted below the target, leave the node
drained and report the constrained path and retained data classes. Do not loop
indefinitely or delete protected data to reach a target.

Before C4, pressure mode cannot delete whole run directories. After C4, it can
relax the age floor only for eligible terminal runs. Normal retention still
applies otherwise. Do not traverse live Gradle caches or enable volume pruning.

Already admitted jobs can consume their remaining disk space. This slice
prevents additional claims; it does not guarantee completion of unbounded jobs.
Capacity validation must include both configured concurrent jobs and their
largest expected pulls. Insufficient physical capacity remains an operator
action even after all eligible data has been removed.

### C4: Run deletion requires terminal state and retained evidence

Replace `run_has_recent_access` as the deletion authority. Under the maintenance boundary, a run
directory is eligible only when the control plane reports the current run
attempt terminal, no job of that run remains active, and all durable local
artifacts are represented by a confirmed downloadable bundle. Age is measured
from terminal completion, not directory access.

Reuse `repoArtifactBundleEntries` as the durable-file selector and
`UploadArtifactEntries` as the upload path. Extend the existing `repo-artifacts`
bundle format with a deterministic manifest of selected relative paths,
content digests, and run/attempt identity. Validate coverage against the local
selection after writers have stopped. An older partial bundle is insufficient.
Retain the existing artifact row and CID as remote authority; do not introduce
a separate retention table or trust a local `uploaded=true` marker.

Extend the existing node artifact uploader with an explicit error-returning
finalization operation for maintenance. It verifies an existing matching
bundle or uploads the complete durable selection through the same artifact
endpoint. The host invokes this operation through the node's existing
authenticated API while holding the exclusive lock. This operation must not
try to reacquire that lock. It performs no Docker work or directory deletion.
Define the operation as
`FinalizeRunArtifacts(ctx, runID) (RunCleanupEligibility, error)` and expose it
through `POST /v1/run/cleanup-eligibility` on the existing node server. The
request supplies only the run ID; paths come from the existing directory
helpers. Extend host credential provisioning to use the node API's existing
mTLS trust and authorization contract. Do not reuse a control-plane bearer
token as a node-client certificate or add another authentication mechanism.

Return a typed eligibility result containing run ID, attempt, terminal time,
and verified artifact CID, or a precise ineligibility reason. An empty durable
selection is an explicit verified result, not an upload failure disguised as
success. Recheck state and local coverage before host deletion. Unknown state,
API failure, missing blobs, incomplete upload, or an attempt mismatch preserves
the directory. Reuse the existing run and artifact read APIs and store queries
inside this operation; do not implement a second run-status parser in shell.

Run restart continues to use the existing `RestartRun` transaction and snapshot
materialization. The exclusive lock prevents local claims and hydration while
deletion is in progress. A restart invalidates eligibility for that attempt;
any later attempt must rebuild missing local state from the existing durable
sources. If a restart commits after the final eligibility check, deletion can
only remove that frozen, retained older attempt's local data; hydration of the
new attempt must wait for the lock and reconstruct it. Verify that path rather
than adding a distributed filesystem-deletion transaction. Never delete remote
artifacts as part of host cleanup.

The host deletes only validated children of its configured runs root. Reject
symlinks, path escapes, and mounted or referenced directories. The node remains
the sole owner of job-runtime cleanup. Whole-run deletion must not duplicate
that sweeper or change dependency-cache generation ownership.

## Enforcement

| Boundary | Reuse and extension | Competing machinery removed |
| --- | --- | --- |
| Capacity | `storage.go`, heartbeat diagnostics, node config, existing cleanup timer | Independent shell path lists and stale-heartbeat admission rules |
| Retention | `repoArtifactBundleEntries`, uploader, `artifact_bundles`, blob reads, existing node API | Access time as liveness; duplicate retention registry |

Introduce only the artifact coverage manifest needed for retention. Derive active
work from the existing controller and recovery machinery. Do not add another
active-job counter. Missing runtime evidence blocks maintenance.

## Implementation Slice

### C3 — Pressure-based cleanup

- Requires the maintenance boundary. Extend `storage.go`, node pre-claim checks, config validation,
  deploy configuration, and the existing cleanup service/timer together.
- Use the same path observations and thresholds for the capacity check and
  cleanup target. Document the monitoring interval and production sizing data.
- Verify low-space rejection, probe failure, recovery-target hysteresis,
  insufficient reclaimable data, preserved operator drain, and multiple
  filesystems. Measure filesystem bytes, not Docker's reclaimable estimate.
- Healthy exit: pressure blocks new work and triggers bounded safe cleanup;
  recovery resumes only after the target is met. Keep whole-run deletion off.
  Defer terminal-run eligibility to C4, and defer per-job reservations entirely.

### C4 — Terminal-run retention

- Requires the maintenance boundary and C3. Extend the existing node API, artifact selection/upload,
  bundle coverage validation, and deploy run pruning as one delivery boundary.
  Reuse the existing store/blob contracts and runtime sweeper.
- Enable whole-run deletion only through typed eligibility. Handle old bundles
  without manifests by verifying/re-uploading through the same finalizer, or
  preserving the directory. Unknown legacy directories remain protected.
- Verify an unfinished run between jobs, a terminal run with failed upload,
  an incomplete older bundle, a missing blob, cancellation, restart/attempt
  changes, mounted directories, and a complete retained terminal run.
- Healthy exit: only terminal runs with confirmed evidence are deleted; age
  affects retention duration rather than safety. Pressure cleanup can reclaim
  eligible runs early. Remote retention and sidecar ownership remain unchanged.

For each non-trivial slice, establish its final signatures and one end-to-end
path before filling the remaining bodies. If stubs are useful, mark them
`DD:cleanup Cn`, fail explicitly, and remove all active-slice markers before
completion. Do not stub deferred slices or retain the old path as a fallback.

## Completion

Each slice must build and pass the relevant existing Go/store and deploy
service tests in its owning worktrees. Cross-worktree slices have one combined
green exit. Add deterministic concurrency tests at the enforcing boundaries.
Validate the pull/prune exclusion on a disposable Docker host, not production.

The final acceptance run must show: admission stopped before pruning; no
overlap with pulls or finalization; measured free space reaching the target or
an explicit drained failure; and preservation of unfinished or unretained runs.
Logs identify the drain revision, operation, constrained path, before/after
bytes, and reason for any preserved directory without exposing credentials.

Update `docs/node-maintenance.md` and deploy service documentation as each slice
ships. Remove shipped slice details while preserving prerequisites for the
remaining slices.

## References

- [Node maintenance](../docs/node-maintenance.md).
- [Claims](../internal/store/queries/jobs.sql),
  [node updates](../internal/store/queries/nodes.sql),
  [drain handlers](../internal/server/handlers/nodes.go).
- [Claim lifecycle](../internal/nodeagent/claimer_loop.go),
  [execution lifecycle](../internal/nodeagent/execution.go),
  [runtime sweeper](../internal/nodeagent/job_runtime_cleanup.go),
  [startup recovery](../internal/nodeagent/crash_reconcile_startup.go).
- [Storage observations](../internal/nodeagent/storage.go),
  [artifact selection](../internal/nodeagent/execution_artifacts.go),
  [artifact uploader](../internal/nodeagent/uploaders.go),
  [artifact records](../internal/store/queries/artifact_bundles.sql),
  [node API](../internal/nodeagent/server.go).
- Deploy: `services/bin/ploy-node-cleanup`, `ploy-node-update`, and
  `ploy-node-gradle-dependency-cache`; `services/tests/test_services.sh`;
  service environment and Compose lock mounts.
