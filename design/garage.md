# Garage-backed Log + Artifact Storage

Status: implemented (current-state design)

## Scope

Use a single S3-compatible object store backend for blob bytes, with Garage as the
local profile. Keep metadata and deterministic object keys in PostgreSQL.

No backward compatibility and no dual-backend runtime are required.

## Architecture

Blob types:
- Logs
- Diffs
- Artifact bundles

Storage split:
- PostgreSQL stores metadata (`*_size`, IDs, relations, generated `object_key`).
- S3-compatible backend stores bytes at deterministic keys.

Server integration:
- Adapter package: `internal/blobstore/s3`
- Server wiring: `cmd/ployd/main.go`
- Upload orchestration: `internal/server/blobpersist`

Key behavior:
- Upload paths write metadata to DB and bytes to object storage.
- Download paths read object keys from DB and stream bytes from object storage.
- Log SSE fanout decodes incoming gzipped payload bytes, not DB blob columns.

## Object Key Profiles

Deterministic key prefixes (DB-generated object keys):
- Logs: `logs/run/{run_id}/job/{job_id|none}/chunk/{chunk_no}/log/{log_id}.gz`
- Diffs: `diffs/run/{run_id}/diff/{diff_uuid}.patch.gz`
- Artifacts: `artifacts/run/{run_id}/bundle/{bundle_uuid}.tar.gz`

## Local Garage Profile

Local stack source of truth:
- Compose: `local/docker-compose.yml`
- Bootstrap/readiness automation: `scripts/deploy-locally.sh`

Local service model:
- Garage S3 API endpoint: `http://garage:3900`
- Bootstrap service: `garage-init`
- Local defaults:
  - bucket: `ploy`
  - access key: `ploylocal`
  - secret key: `ploy-unsafe-local`

The server container receives object-store settings via `PLOY_OBJECTSTORE_*` env vars.

## Configuration Surface

Server object-store env vars:
- `PLOY_OBJECTSTORE_ENDPOINT`
- `PLOY_OBJECTSTORE_BUCKET`
- `PLOY_OBJECTSTORE_ACCESS_KEY`
- `PLOY_OBJECTSTORE_SECRET_KEY`
- `PLOY_OBJECTSTORE_SECURE`
- `PLOY_OBJECTSTORE_REGION` (optional)

These can also be configured via `object_store.*` in ployd config, with env vars
taking precedence.

See also:
- `docs/envs/README.md`
- `roadmap/garage.md`

## Non-goals

- Data migration from historical DB blob columns.
- Runtime support for multiple object-store providers at once.
- Reintroduction of provider-specific local defaults.
