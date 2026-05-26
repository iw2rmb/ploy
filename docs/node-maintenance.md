# Node Maintenance

Worker node maintenance is host-owned. The node container executes jobs and
reports telemetry; host `systemd` timers refresh registry auth, update the node
image, and clean local cache state.

## Storage telemetry

Each heartbeat stores disk capacity from the most constrained configured storage
path, not only `/`. The node checks `/`, `DOCKER_ROOT_DIR`, `PLOYD_CACHE_HOME`,
`PLOY_BUILDGATE_CACHE_ROOT`, and `TMPDIR`, then uses the lowest-free successful
probe for `disk_free_bytes` and `disk_total_bytes`.

The full breakdown is uploaded as the `node` diagnostic under
`details.storage`. Each path entry includes the source env, path, free/total/used
bytes, used percent, inode counters, and any probe error.

## Registry auth

The node pulls job images directly. Auth config precedence is:

1. `PLOY_DOCKER_AUTH_CONFIG`
2. `PLOY_DOCKER_AUTH_CONFIG_FILE`
3. `DOCKER_AUTH_CONFIG`

`PLOY_DOCKER_AUTH_CONFIG_FILE` must point to a Docker auth config JSON file in
`DOCKER_AUTH_CONFIG` format. Host maintenance should refresh this file
atomically and keep it readable by the node container.

## Host services

The deploy service bundle provides:

- `ploy-node-auth-refresh.timer` — refreshes Artifactory Docker auth.
- `ploy-node-update.timer` — pulls the node image, drains the node, waits for
  active job containers, recreates the node service, and undrains.
- `ploy-node-cleanup.timer` — prunes exited containers, unused images, and old
  Ploy cache directories.

## CLI and API

Node maintenance actions are no longer enqueued through the control plane.

Useful read-only surfaces remain:

```sh
ploy cluster node actions <node-id> --limit 20
```

- `GET /v1/nodes/{id}/actions?limit=N` for historical action status.
- `GET /v1/nodes/{id}/diagnostics` for node diagnostics and storage details.
