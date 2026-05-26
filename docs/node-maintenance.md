# Node Maintenance Actions

Nodes can run control-plane queued maintenance actions without SSH. The action
claim path accepts only hardcoded `node.*` action types; it is not a remote shell.

## Actions

- `node.cleanup_disk` runs inside the `node-updater` container. It waits for
  active job containers, runs the updater cleanup cycle with an emergency age,
  prunes Docker containers/images/build cache/volumes, and removes mounted
  Build Gate cache entries when that cache path is visible in the updater.
- `node.update_updater` runs inside the current `node-updater` container and
  launches a detached `docker compose pull/up` for the `node-updater` service.

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
