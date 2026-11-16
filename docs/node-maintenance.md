# Node Maintenance

Worker node maintenance is host-owned. The node container executes jobs and
reports telemetry; host services own registry auth, update the node image on
explicit rollout, and clean local cache state.

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
  its lock participation, and prunes old node images before restoring admission.
- `ploy-node-auth-refresh.timer` — forces a fresh, validated registry
  credential every 5 minutes independently of other host maintenance.
- `ploy-node-auth-refreshd.service` — serves a local Unix socket for explicit
  auth-refresh requests after Docker reports unauthorized.
- `ploy-node-cleanup.timer` — prunes exited containers, unused images, and old
  Git clones. Under the maintenance lock, it also removes `runtime-share` from
  successful runs after the configured retention period. Whole-run directories
  and broad TMPDIR children are preserved.

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

The node record also exposes nullable `drain_owner`. Cleanup acquires its drain
with `?expected_revision=N&owner=cleanup`. Other callers omit `owner`.
An owner-less drain or any undrain clears ownership. Existing drains have no
owner after migration; cleanup cannot adopt them.

A cleanup retry can renew an existing cleanup-owned drain with the same query.
This conditional same-state update advances the revision. Other conditional
same-state updates return HTTP 409. Cleanup requires the ownership field before
changing admission.

The host provisions `/run/lock/ploy-node-maintenance.lock` through systemd
`tmpfiles.d`. The node bind-mounts the host lock directory at the same path.
Never unlink or replace the lock file. `PLOY_NODE_MAINTENANCE_LOCK_FILE` selects
the same path on both sides. A missing or inaccessible file rejects local work.

Each execution slot holds its own shared lock before the claim request. The
controller retains that lock through image pulls, execution, deferred artifact
uploads, terminal reporting, and runtime cleanup. Direct run starts, startup
reconciliation, recovered-container monitors, and runtime sweeps use the same
boundary. Concurrent jobs can hold shared locks together.

Cleanup, node updates, and Gradle dependency publication use the shared deploy
helper in this order:

1. Verify a fresh diagnostic from the current node container, its protocol,
   lock inode, and host directory mount.
2. Conditionally acquire drain, or observe an existing cleanup-owned drain for
   a cleanup retry. Preserve other existing drains.
3. Acquire the exclusive lock within `PLOY_NODE_MAINTENANCE_LOCK_TIMEOUT`
   seconds (default 300).
4. Recheck drain ownership. A cleanup retry renews its revision under the lock.
5. Inspect job containers and job resources. Reject active or uncertain state
   and unaccounted containerd leases.
6. Perform and verify the operation. Conditionally restore admission before
   releasing the lock.

Docker inspection determines container state. Cleanup enumerates all containers
before checking their inspected state and age. A stale Docker listing cannot
hide an eligible stopped container. Removal remains non-forced.

The lease check accounts for Docker containers and inactive BuildKit cache.
BuildKit is Docker's build engine. The helper reads cache ownership from the
local Docker Engine and accepts exact cache IDs and their `-view` and
`-variants` leases. Active cache, unknown leases, and unavailable inspection
block maintenance. These checks do not replace the exclusive lock.

Failed checks or operations retain drain and report the reason in the host
journal. Cleanup retains `drain_owner=cleanup`, so its next hourly attempt can
retry. Revision renewal prevents an older waiter from releasing that retry's
drain. Operator reassertions clear ownership and invalidate pending cleanup.

On a lock timeout or cancellation while waiting, a fresh acquisition releases
only its own drain revision. A retry preserves the inherited drain. Lock
timeouts exit with status 75 (deferred). Other maintenance operations preserve
their existing failure behavior and cannot adopt cleanup-owned drains.

A crashed node can leave Docker activity alive. Startup recovery must finish
before maintenance can proceed. Heartbeats remain available while jobs wait.
Registry auth uses its separate lock and daemon-free manifest inspection.

Deploy the control plane and lock-participating nodes before enabling this host
protocol. Old nodes fail preflight. For the first upgrade, stop maintenance,
drain admission, and let old node activity finish before replacing the node
with the shared mount. An offline bootstrap uses the same conditional drain and
exclusive lock and rejects any surviving job activity. Wait for the new node's
`details.maintenance` diagnostic before enabling maintenance. Node replacement
preserves the lock inode.

For cleanup ownership, deploy the control-plane migration and API before the
updated host helper and cleanup script. Verify that `/v1/nodes` includes
`drain_owner`, including explicit `null` values. Existing lock-participating
node images need no protocol change. Resolve legacy retained drains from their
host journals; do not assign cleanup ownership based only on `drained=true`.

## CLI and API

Node maintenance actions are not enqueued or listed through the control plane.
Use `GET /v1/nodes/{id}/diagnostics` for node diagnostics and storage details.

Node diagnostics only report the long-lived node daemon component. Host-owned
maintenance services such as `ploy-node-update.service` are inspected through
systemd/journald on the node host, not through a persistent diagnostics
component.
