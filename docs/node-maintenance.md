# Node Maintenance

Worker node maintenance is host-owned. The node container executes jobs and
reports telemetry; host services own registry auth, update the node image on
explicit rollout, and clean local cache state.

## Storage telemetry

Each heartbeat stores disk capacity from the most constrained configured storage
path, not only `/`. The node checks `/`, `DOCKER_ROOT_DIR`, `PLOYD_CACHE_HOME`,
`PLOY_BUILDGATE_CACHE_ROOT`, and `TMPDIR`, then uses the lowest-free successful
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

Job containers that mount `/var/run/docker.sock` also receive
`/etc/ploy/docker-auth-config` read-only as `/root/.docker`. Docker clients
inside those job containers, including Testcontainers, therefore use the same
host-refreshed credentials as node-owned image pulls.

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
  refresh, drains the node, waits for active job containers, recreates the node
  service, and undrains.
- `ploy-node-auth-refreshd.service` — serves a local Unix socket for explicit
  auth-refresh requests after Docker reports unauthorized. It has no timer.
- `ploy-node-cleanup.timer` — prunes exited containers, unused images, and old
  Ploy cache directories.

The tracked source for these host scripts lives in
`/Users/v.v.kovalev/@gitlab/ploy/deploy/services` in this workspace. The auth
refresh helper is invoked manually with
`sudo /usr/local/lib/ploy/ploy-node-auth-refresh refresh-for-pull <image-ref>`
or by `ploy-node-update.service` and `ploy-node-auth-refreshd.service` after an
auth failure. Do not install or enable standalone `ploy-node-auth-refresh.timer`
or `ploy-node-update.timer`.

## CLI and API

Node maintenance actions are not enqueued or listed through the control plane.
Use `GET /v1/nodes/{id}/diagnostics` for node diagnostics and storage details.

Node diagnostics only report the long-lived node daemon component. Host-owned
maintenance services such as `ploy-node-update.service` are inspected through
systemd/journald on the node host, not through a persistent diagnostics
component.
