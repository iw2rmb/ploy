# Node Maintenance

Worker node maintenance is host-owned. The node container executes jobs and
reports telemetry; host services refresh registry auth at pull boundaries,
update the node image on explicit rollout, and clean local cache state.

## Storage telemetry

Each heartbeat stores disk capacity from the most constrained configured storage
path, not only `/`. The node checks `/`, `DOCKER_ROOT_DIR`, `PLOYD_CACHE_HOME`,
`PLOY_BUILDGATE_CACHE_ROOT`, and `TMPDIR`, then uses the lowest-free successful
probe for `disk_free_bytes` and `disk_total_bytes`.

The full breakdown is uploaded as the `node` diagnostic under
`details.storage`. Each path entry includes the source env, path, free/total/used
bytes, used percent, inode counters, and any probe error.

## Registry auth

The node pulls job images directly. Private-registry auth is read only from
`PLOY_DOCKER_AUTH_CONFIG_FILE`, which must point to a Docker auth config JSON
file in `DOCKER_AUTH_CONFIG` format. The file is read for every Docker pull, so
host refreshes take effect without recreating the node container.

Do not inject `PLOY_DOCKER_AUTH_CONFIG` or `DOCKER_AUTH_CONFIG` into the node
container for job image pulls. Inline env values are immutable for the lifetime
of the container and can outlive refreshed credentials.

If a pull returns registry unauthorized and `PLOY_DOCKER_AUTH_REFRESH_CONTAINER`
is set, the node asks that helper container to run
`/usr/local/lib/ploy/ploy-node-auth-refresh refresh-for-pull <image-ref>`, then
retries the pull once. The helper owns DP credentials; the node container does
not.

## Host services

The deploy service bundle provides:

- `ploy-node-auth-refresh.service` — refreshes Artifactory Docker auth for a
  requested image and installs the new auth file only after a real pull
  validation succeeds.
- `ploy-node-update.service` — explicitly pulls the node image, drains the node,
  waits for active job containers, recreates the node service, and undrains.
- `ploy-node-cleanup.timer` — prunes exited containers, unused images, and old
  Ploy cache directories.

The tracked source for these host scripts lives in
`/Users/v.v.kovalev/@gitlab/ploy/deploy/services` in this workspace. The auth
refresh and node update services must not be enabled as arbitrary periodic
timers.

## CLI and API

Node maintenance actions are no longer enqueued through the control plane.

Useful read-only surfaces remain:

```sh
ploy cluster node actions <node-id> --limit 20
```

- `GET /v1/nodes/{id}/actions?limit=N` for historical action status.
- `GET /v1/nodes/{id}/diagnostics` for node diagnostics and storage details.
