# Node Maintenance Simplification

## Summary

Node maintenance moves from the `node-updater` sidecar to host `systemd`
services. The node pulls job images directly using a Docker auth config file,
and the host owns registry auth refresh, node image updates, and cleanup.

## Keep

- Node storage diagnostics for `/`, `DOCKER_ROOT_DIR`, `PLOYD_CACHE_HOME`,
  `PLOY_BUILDGATE_CACHE_ROOT`, and `TMPDIR`.
- `GET /v1/nodes/<node-id>/diagnostics` for read-only operational state.
- Historical `GET /v1/nodes/<node-id>/actions` while old action rows may exist.
- Host cleanup with Docker volume pruning disabled unless explicitly enabled.

## Remove

- `node-updater` service, image build, self-update, and diagnostics.
- Delegated node image pulls through `docker exec` into `node-updater`.
- Control-plane creation of `node.cleanup_disk` and `node.update_updater`.
- Node claim priority for node-scoped maintenance actions.

## Success Criteria

- `PLOY_DOCKER_AUTH_CONFIG_FILE` exists on the host, is mounted into the node,
  and direct node pulls work without the updater container.
- `ploy-node-update.timer` can drain, recreate, and undrain the node.
- `ploy-node-cleanup.timer` recovers disk without manual Docker commands.
- New `pre_gate` jobs no longer fail during workspace hydration because of
  `No space left on device`.
