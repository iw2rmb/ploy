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

Do not inject `PLOY_DOCKER_AUTH_CONFIG` or `DOCKER_AUTH_CONFIG` into the node
container for job image pulls. Inline env values are immutable for the lifetime
of the container and can outlive refreshed credentials.

If a pull returns registry unauthorized, the pull fails with the Docker error.
The node has no helper container, DP service-account key, or hidden auth refresh
retry path. Repair auth on the host, then rerun the job.

## Host services

The deploy service bundle provides:

- `ploy-node-update.service` — refreshes Docker auth only when the canonical
  auth file cannot pull the target node image, explicitly pulls the node image
  with `docker --config /etc/ploy/docker-auth-config`, drains the node, waits
  for active job containers, recreates the node service, and undrains.
- `ploy-node-cleanup.timer` — prunes exited containers, unused images, and old
  Ploy cache directories.

The tracked source for these host scripts lives in
`/Users/v.v.kovalev/@gitlab/ploy/deploy/services` in this workspace. The auth
refresh helper is invoked manually with
`sudo /usr/local/lib/ploy/ploy-node-auth-refresh refresh-for-pull <image-ref>`
or by `ploy-node-update.service` when needed. Do not install or enable
standalone `ploy-node-auth-refresh.service`, `ploy-node-auth-refresh.timer`, or
`ploy-node-update.timer`.

## CLI and API

Node maintenance actions are no longer enqueued through the control plane.

Useful read-only surfaces remain:

```sh
ploy cluster node actions <node-id> --limit 20
```

- `GET /v1/nodes/{id}/actions?limit=N` for historical action status.
- `GET /v1/nodes/{id}/diagnostics` for node diagnostics and storage details.
