# Node Maintenance Actions

Nodes can run control-plane queued maintenance actions without SSH. The action
claim path accepts only hardcoded `node.*` action types; it is not a remote shell.

## Storage telemetry

Each heartbeat stores disk capacity from the most constrained configured storage
path, not only `/`. The node checks `/`, `DOCKER_ROOT_DIR`, `PLOYD_CACHE_HOME`,
`PLOY_BUILDGATE_CACHE_ROOT`, and `TMPDIR`, then uses the lowest-free successful
probe for `disk_free_bytes` and `disk_total_bytes`.

The full breakdown is uploaded as the `node` diagnostic under
`details.storage`. Each path entry includes the source env, path, free/total/used
bytes, used percent, inode counters, and any probe error.

## Actions

- `node.cleanup_disk` runs inside the `node-updater` container. It waits for
  active job containers, runs the updater cleanup cycle with an emergency age,
  prunes Docker containers/images/build cache/volumes, and removes mounted
  Build Gate cache entries when that cache path is visible in the updater.
- `node.update_updater` runs inside the current `node-updater` container and
  delegates to the updater's own self-update function.

The updater also runs a cleanup cycle on start and then every hour by default.
It restores registry auth with `dp auth service-acc` when the service account
key is mounted, checks its own image before the node image, and recreates the
`node-updater` service when a newer updater image is available.

## CLI

```sh
ploy cluster node cleanup <node-id> --wait
ploy cluster node update-updater <node-id> --wait
ploy cluster node actions <node-id> --limit 20
```

The server API is:

- `POST /v1/nodes/{id}/actions` with `{"action_type":"node.cleanup_disk"}` or
  `{"action_type":"node.update_updater"}`.
- `GET /v1/nodes/{id}/actions?limit=N` for recent action status and result.
- `GET /v1/nodes/{id}/diagnostics` for node and node-updater diagnostics,
  including storage details and updater image check status.
