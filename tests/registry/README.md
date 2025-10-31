Registry Roundtrip Test

- Purpose: validate pushing, fetching, and deleting OCI blobs and manifests via the Ploy control-plane registry using only the CLI over HTTPS.
- Script: `tests/registry/registry-roundtrip.sh`

What it does
- Builds a tiny OCI image from scratch (1 layer + config) without Docker.
- Pushes both blobs via `ploy registry push-blob`.
- Creates and uploads a valid OCI manifest referencing the blobs via `ploy registry put-manifest` (tagged).
- Fetches the manifest and layer back via `get-manifest`/`get-blob` and verifies sha256 digests.
- Deletes the tag, manifest (by digest), and both blobs to leave the registry clean.

Prerequisites
- A cluster descriptor configured for HTTPS: includes `api_endpoints`, `api_server_name`, and a `ca_bundle` (see `dist/ploy cluster https --help`).
- Or set `PLOY_CONTROL_PLANE_URL` to an HTTPS base and ensure the CLI trusts the CA.
- `jq`, `tar`, and `sha256sum` or `shasum` available on the workstation.

Usage
- Run: `tests/registry/registry-roundtrip.sh`
- Optional env vars:
  - `PLOY_E2E_REGISTRY_REPO` — repo path to use (default: `e2e/mods-sample`).
  - `PLOY_E2E_TAG` — tag to create (default: `e2e-<timestamp>`).
