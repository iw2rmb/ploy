# Node Cleanup Follow-Up

## Summary

The current maintenance release intentionally keeps two recovery paths active:
the node-updater performs autonomous cleanup/self-update, and the control plane
can still enqueue emergency node actions. After the updated node-updater proves
it can update itself and recover disk pressure, the control-plane emergency path
should be reduced.

## Keep

- Hourly node-updater cleanup, including cleanup on service start.
- Node-updater self-update before node image update.
- Node storage diagnostics for `/`, `DOCKER_ROOT_DIR`, `PLOYD_CACHE_HOME`,
  `PLOY_BUILDGATE_CACHE_ROOT`, and `TMPDIR`.
- `GET /v1/nodes/<node-id>/diagnostics` and daemon logs for no-SSH operations.
- `ploy cluster node actions` while queued maintenance actions still exist.

## Remove After Success

- Remove `node.update_updater` from the control-plane action queue once every
  active node reports an updater version with self-update support.
- Remove most Docker-exec updater control from the node agent. The updater
  should own updater recreation and regular cleanup itself.
- Keep `node.cleanup_disk` only as a short-lived emergency action until hourly
  cleanup has recovered disk on the affected nodes for several cycles.
- Remove emergency Docker volume pruning from the regular path. It should remain
  opt-in only, because volumes can be unrelated to Ploy run caches.

## Success Criteria

- The affected node reports current `node-updater` diagnostics after a service
  restart.
- `details.storage.paths` shows nonzero free space for the Ploy run cache and
  Build Gate cache mounts.
- Hourly updater daemon logs show cleanup cycles without manual SSH or Docker
  commands.
- New `pre_gate` jobs no longer fail during workspace hydration because of
  `No space left on device`.
