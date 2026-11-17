# Node Maintenance

The node owns local cleanup and reports storage telemetry. Host services own
registry auth, node updates, and Gradle dependency publication. Both owners use
the same maintenance lock.

## Storage telemetry

Each heartbeat stores disk capacity from the most constrained configured storage
path, not only `/`. The node checks `/`, `DOCKER_ROOT_DIR`, `PLOYD_CACHE_HOME`,
`PLOY_NODE_CACHE_ROOT`, and `TMPDIR`, then uses the lowest-free successful
probe for `disk_free_bytes` and `disk_total_bytes`.

The full breakdown is uploaded as the `node` diagnostic under
`details.storage`. Each path entry includes the source env, path, free/total/used
bytes, used percent, inode counters, and any probe error.

## Registry auth

The canonical host Docker auth file is
`/etc/ploy/docker-auth-config/config.json`. The node mounts
`/etc/ploy/docker-auth-config` read-only and sets
`PLOY_DOCKER_AUTH_CONFIG_FILE=/etc/ploy/docker-auth-config/config.json`.
Private-registry auth is read only from that file in `DOCKER_AUTH_CONFIG`
format. The file is read for every Docker pull, so host refreshes take effect
without recreating the node container.

Every 5 minutes, the host obtains a fresh credential and validates registry
manifest access. It then replaces the canonical auth file. Periodic refresh uses a dedicated auth lock,
so node updates, cleanup, and active jobs do not block credential rotation.

Job containers that mount `/var/run/docker.sock` also receive
`/etc/ploy/docker-auth-config` read-only as `/root/.docker`. Docker clients
inside those job containers therefore use the same host-refreshed credentials
as node-owned image pulls.

Do not inject `PLOY_DOCKER_AUTH_CONFIG` or `DOCKER_AUTH_CONFIG` into the node
container for job image pulls. Inline env values are immutable for the lifetime
of the container and can outlive refreshed credentials.

If a pull returns registry unauthorized, the node asks the host auth-refresh
socket to refresh registry-wide auth, then retries the same image pull once. The
node has no helper container or DP service-account key; the host remains the
only writer of `/etc/ploy/docker-auth-config/config.json`.

## Host services

The deploy service bundle provides:

- `ploy-node-update.service` — refreshes Docker auth only when the canonical
  auth file cannot pull the target node image, retries the pull once after
  refresh, recreates the node service under the maintenance boundary, verifies
  its lock participation, and restores admission.
- `ploy-node-auth-refresh.timer` — forces a fresh, validated registry
  credential every 5 minutes independently of other host maintenance.
- `ploy-node-auth-refreshd.service` — serves a local Unix socket for explicit
  auth-refresh requests after Docker reports unauthorized.

The tracked source for these host scripts lives in
`/Users/v.v.kovalev/@gitlab/ploy/deploy/services` in this workspace. The auth
refresh helper is invoked manually with
`sudo /usr/local/lib/ploy/ploy-node-auth-refresh refresh <image-ref>` or by the
periodic timer. `ploy-node-update.service` and
`ploy-node-auth-refreshd.service` also refresh after an auth failure.

## Maintenance boundary

Claims lock the selected node row before selecting a job. A committed drain
blocks new claims. Every drain or undrain, including an operator reassertion,
increments `drain_revision`. Conditional callers use `?expected_revision=N`.
A stale revision returns HTTP 409. Successful updates return HTTP 204 with
`X-Ploy-Drain-Revision`. Unconditional reassertions invalidate older maintenance
ownership.

The node record also exposes nullable `drain_owner`. The value `cleanup`
identifies a retained drain from the retired host cleanup service. Node cleanup
does not change server drains. An operator drain clears cleanup ownership.

The host provisions `/run/lock/ploy-node-maintenance.lock` through systemd
`tmpfiles.d`. The node bind-mounts the host lock directory at the same path.
Never unlink or replace the lock file. `PLOY_NODE_MAINTENANCE_LOCK_FILE` selects
the same path on both sides. A missing or inaccessible file rejects local work.

Each execution slot holds its own shared lock before the claim request. The
before-job hook releases that lock while retaining the concurrency permit.
The controller reacquires shared protection before workspace hydration or image
pulls. It retains protection through execution, deferred artifact uploads,
terminal reporting, and per-job runtime cleanup.

Direct starts wait for startup reconciliation before acquiring a permit.
Recovered-container monitors acquire their permits during reconciliation.
Startup reconciliation and runtime sweeps also hold shared protection.
Concurrent jobs can hold shared locks together.

Node updates and Gradle dependency publication use the shared deploy helper:

1. Verify a fresh diagnostic from the current node container, its protocol,
   lock inode, and host directory mount.
2. Conditionally acquire drain. Preserve existing drains.
3. Acquire the exclusive lock within `PLOY_NODE_MAINTENANCE_LOCK_TIMEOUT`
   seconds (default 300).
4. Recheck drain ownership.
5. Inspect job containers and job resources. Reject active or uncertain state
   and unaccounted containerd leases.
6. Perform and verify the operation. Conditionally restore admission before
   releasing the lock.

The lease check accounts for Docker containers and inactive BuildKit cache.
BuildKit is Docker's build engine. The helper reads cache ownership from the
local Docker Engine and accepts exact cache IDs and their `-view` and
`-variants` leases. Active cache, unknown leases, and unavailable inspection
block maintenance. These checks do not replace the exclusive lock.

Failed host checks or operations retain drain and report the reason in the host
journal. On a lock timeout or cancellation while waiting, the helper releases
only its own drain revision. Lock timeouts exit with status 75 (deferred).

A crashed node can leave Docker activity alive. Startup recovery must finish
before maintenance can proceed. Heartbeats remain available while jobs wait.
Registry auth uses its separate lock and daemon-free manifest inspection.

## Job-boundary cleanup

The controller attempts cleanup immediately before execution and after
finalization. Recovered-container monitors use the after-job hook. Empty claim
polls and startup itself do not trigger cleanup.

Each hook releases its shared lock before trying the exclusive maintenance
lock. A busy lock skips the attempt. The controller permits one cleanup pass
per process at a time. A successful pass starts a one-hour cooldown measured
from completion. Restart clears the cooldown.

Cleanup has a five-minute timeout. Node shutdown cancels cleanup; completed job
cancellation does not. Failures and incomplete inspection preserve affected
data and do not start the cooldown. The node logs the error and retries at a
later job boundary. Continuous overlapping jobs can prevent cleanup indefinitely.

The node checks job containers, job resources, and containerd leases before
deletion. It removes expired `created`, `exited`, and `dead` containers without
force, including their anonymous volumes. Container retention uses creation
time. Build-cache pruning keeps its configured age filter. Old Git clones use
`.git` access time.
See [cleanup environment settings](envs/README.md) for age and volume options.

### Image retention

`PLOY_NODE_CLEANUP_AGE` measures image retention from the last Ploy-managed use.
The controller records immutable image IDs for direct job containers and nested
containers created through the per-job Docker socket proxy. It refreshes these
records at job completion.

An unknown image receives the full retention interval from its first observation.
Node restart clears the in-memory records, granting fresh grace when images are
first observed again. Existing containers always protect their referenced images.
Uncertain image identity also resets the records. Docker build, import, and load
requests receive this protection because their requests do not identify every image.
Cleanup removes eligible images individually without force or ancestor pruning,
the automatic deletion of parent images.

Manual ephemeral workloads outside the proxy do not refresh these records.
Docker-in-Docker runs a separate Docker daemon; this cleanup covers only the host
daemon.

### Workspace retention

The server authorizes workspace retention through `GET /v1/runs/{run_id}`.
The response includes `cleanup_eligible`, `status`, `attempt`, `resume_count`,
and `finished_at`. The server reads the run under the same row lock used by
restart, then applies its wall clock after acquiring that lock.

| Current run status | Retention from `finished_at` |
| --- | --- |
| `Success` | 6 hours |
| `Fail`, `Cancelled` | 24 hours |
| Nonterminal or unknown | Preserve |

The server requires nonempty, terminal current-attempt jobs. Immediately before
deletion, the node rereads eligibility and checks that run identity is unchanged.
Missing eligibility, unsafe paths, mounted descendants, and container references
preserve data.

Cleanup deletes only the run's `workspace` and `runtime-share` children.
It preserves `jobs`, `share`, logs, patches, outputs, artifact records, and remote
blobs. The per-job runtime sweeper retains ownership of abandoned job directories.
See [run resume](runs.md#inspect-and-control) for the retained-resume deadline.

## Deploy cleanup ownership

Deploy the server retention and resume contract before enabling node cleanup.
Protocol `2` requires the shared lock, containerd socket, and read-only host
mount information. The node image packages `ctr` for task and lease inspection.
The protocol `1` updater rejects protocol `2` images until coordinated deployment.

Use deploy's one-time `scripts/migrate-node-triggered-cleanup.sh <node-image>`
on existing nodes. Follow the deploy service instructions to stage the checkout.
The script resolves cleanup-owned drains through the installed old helper and
holds deployment maintenance while replacing runtime files. It retires host
cleanup before the node takes ownership. It releases its drain revision after
the replacement publishes a fresh protocol `2` diagnostic. Unresolved drains
or operator changes block the handoff.

Node replacement preserves the lock inode. The host maintenance helper remains
available for updates and dependency publication.
The new-node installer provisions the current runtime without running this migration.

## Cache deployment verification

The generic job storage and image cache contracts are documented in
[Job storage](migs-lifecycle.md#runtime-paths) and
[environment settings](envs/README.md). Deploy owns Gradle policy and cache
publication. Ploy owns job storage isolation and runtime-directory removal.

For a node or image rollout, verify the following operational outcomes:

- Two concurrent Gradle runs use distinct writable job caches and complete
  without cache deserialization errors.
- Each official image starts with an empty mounted job home and loads its
  defaults. Supplied read-only home inputs remain read-only.
- Old shared writable Gradle and Maven homes are no longer mounted. Quarantine
  them only after checking container references and active work.

Deploy provides `images/shared/gradle-read-only-dependency-cache_test.sh` and
`images/shared/gradle-task-output-cache_test.sh` for Docker integration checks.
The former verifies dependency reuse and isolated misses. The latter verifies
task-output reuse and rejection of anonymous writes. Record run IDs and image
revisions with the verification results.

## CLI and API

Node maintenance actions are not enqueued or listed through the control plane.
Use `GET /v1/nodes/{id}/diagnostics` for node diagnostics and storage details.

Node diagnostics only report the long-lived node daemon component. Host-owned
maintenance services such as `ploy-node-update.service` are inspected through
systemd/journald on the node host, not through a persistent diagnostics
component.
