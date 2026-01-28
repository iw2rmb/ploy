# MinIO-backed Log + Artifact Storage

Status: draft (design only)

## Problem

Today Ploy stores “blob” data directly in PostgreSQL:

- `ploy.logs.data` (`BYTEA`, gzipped chunks, cap: 1 MiB per chunk)
- `ploy.diffs.patch` (`BYTEA`, gzipped unified diff, cap: 1 MiB)
- `ploy.artifact_bundles.bundle` (`BYTEA`, gzipped tar, cap: 1 MiB)

This inflates the DB, makes backups expensive, and puts pressure on Postgres I/O for data that is better suited for an object store.

## Goals

- Store **all log/artifact/diff bytes** in MinIO (S3-compatible).
- Keep only **metadata + object references** in Postgres.
- Local dev stack (`local/docker-compose.yml`) includes MinIO and auto-creates the required bucket.
- No backwards compatibility is required (schema + code + API can be breaking).

## Non-goals

- Data migration from existing databases (explicitly out of scope).
- Supporting multiple storage backends simultaneously.
- Changing the core workflow semantics (only storage layer changes).

## Current State (code paths)

### Upload (writes)

- Logs:
  - Node → server: `POST /v1/nodes/{id}/logs` (`internal/server/handlers/nodes_logs.go`)
  - Run-scoped ingestion: `POST /v1/runs/{id}/logs` (`internal/server/handlers/runs_events.go`)
  - Persistence: `events.Service.CreateAndPublishLog` → `store.CreateLog` (`internal/server/events/service.go`)
- Diffs:
  - Node → server: `POST /v1/runs/{run_id}/jobs/{job_id}/diff` (`internal/server/handlers/jobs_diff.go`)
  - Persistence: `store.CreateDiff`
- Artifacts (includes build-gate logs uploaded as artifact bundles):
  - Node → server: `POST /v1/runs/{run_id}/jobs/{job_id}/artifact` (`internal/server/handlers/jobs_artifact.go`)
  - Persistence: `store.CreateArtifactBundle`

All three currently base64-decode bytes in the handler and store them into `BYTEA` columns.

### Download / read paths

- Artifacts:
  - Metadata listing: `GET /v1/artifacts?cid=...` → `ListArtifactBundlesMetaByCID` (DB-only)
  - Bytes download: `GET /v1/artifacts/{id}?download=true` reads `artifact_bundles.bundle` from DB (`internal/server/handlers/artifacts_download.go`)
- Diffs:
  - Metadata listing: `GET /v1/runs/{run_id}/repos/{repo_id}/diffs` uses meta query (DB-only)
  - Bytes download: `GET /v1/runs/{run_id}/repos/{repo_id}/diffs?download=true&diff_id=...` reads `diffs.patch` from DB (`internal/server/handlers/diffs.go`)
- Logs:
  - SSE streaming reads log bytes from the persisted DB row to decode and fanout:
    `events.Service.publishLogToHub` gunzips `store.Log.Data` (`internal/server/events/service.go`)

## Proposed Design

### Overview

Introduce an **object store** dependency (MinIO/S3) used by the server to store blob bytes.

- PostgreSQL keeps metadata and references to objects.
- MinIO stores the actual bytes.
- Nodes and CLI continue to talk only to the server; they do not need direct MinIO credentials.

### Object store interface

Add a small internal abstraction, e.g.:

- `internal/blobstore`:
  - `Put(ctx, key, contentType, data) (etag string, err error)`
  - `Get(ctx, key) (io.ReadCloser, size int64, err error)`
  - `Delete(ctx, key) error`

Implementation:

- `internal/blobstore/minio` using `github.com/minio/minio-go/v7` (S3 API).

### Object naming (keys)

Store keys deterministically in the DB so the DB row itself fully identifies the object.

Recommended prefixes:

- Logs: `logs/run/{run_id}/job/{job_id|none}/chunk/{chunk_no}/log/{log_id}.gz`
- Diffs: `diffs/run/{run_id}/diff/{diff_uuid}.patch.gz`
- Artifacts: `artifacts/run/{run_id}/bundle/{bundle_uuid}.tar.gz`

Notes:

- Including `run_id` in the key makes ad-hoc debugging easy.
- Keys should be stable (no dependence on runtime configuration like “prefix” unless that prefix is also persisted).

### Postgres schema changes (initial migration)

Ploy uses a single embedded “initial migration” (`internal/store/schema.sql`) gated by `internal/store/migrations.go:SchemaVersion`.
This design assumes we change that schema directly (breaking change) and bump `SchemaVersion`.

#### Logs

In `internal/store/schema.sql`, update the `CREATE TABLE logs (...)` definition:

- Remove `data BYTEA NOT NULL ...`
- Add:
  - `data_size BIGINT NOT NULL CHECK (data_size > 0)`
  - `object_key TEXT GENERATED ALWAYS AS (...) STORED`

Recommended `object_key` expression:

```sql
'logs/run/' || run_id ||
'/job/' || COALESCE(job_id, 'none') ||
'/chunk/' || chunk_no::text ||
'/log/' || id::text || '.gz'
```

#### Diffs

In `internal/store/schema.sql`, update the `CREATE TABLE diffs (...)` definition:

- Remove `patch BYTEA NOT NULL ...`
- Add:
  - `patch_size BIGINT NOT NULL CHECK (patch_size > 0)`
  - `object_key TEXT GENERATED ALWAYS AS (...) STORED`

#### Artifact bundles

In `internal/store/schema.sql`, update the `CREATE TABLE artifact_bundles (...)` definition:

- Remove `bundle BYTEA NOT NULL ...`
- Add:
  - `bundle_size BIGINT NOT NULL CHECK (bundle_size > 0)`
  - `object_key TEXT GENERATED ALWAYS AS (...) STORED`

#### SQLC query changes

Update SQL in:

- `internal/store/queries/logs.sql`
- `internal/store/queries/diffs.sql`
- `internal/store/queries/artifact_bundles.sql`

So that:

- “meta” queries return `*_size` columns instead of `octet_length(blob)`.
- “Get” queries return `object_key` (and do not return blob bytes).
- “Create” queries accept `*_size` and no longer accept blob bytes.

This will regenerate `internal/store/*.sql.go` via `sqlc`.

### Server code changes

#### Centralize blob persistence

Add a server-side service (example):

- `internal/server/blobpersist` (or similar)
  - `CreateLog(ctx, params, data)`:
    - Insert metadata into DB (`data_size = len(data)`), read `object_key`
    - Upload `data` to MinIO at `object_key`
  - `CreateDiff(ctx, params, patch)`
  - `CreateArtifactBundle(ctx, params, bundle)`

Handlers and `events.Service` call this service instead of writing `BYTEA` to Postgres.

#### Logs SSE fanout must not read bytes from DB

Change `events.Service.CreateAndPublishLog` / `publishLogToHub` so that fanout uses the **incoming bytes** (request payload),
not `store.Log.Data` from Postgres (since that column no longer exists).

Concretely:

- Persist metadata + upload to MinIO.
- Decode and publish using `params.Data` (already gzipped bytes).

#### Downloads stream from MinIO

Update:

- `internal/server/handlers/diffs.go` download mode to:
  - read `diff.object_key` from DB
  - `blobstore.Get` and stream bytes to response
- `internal/server/handlers/artifacts_download.go` download mode similarly.

### Config surface

Add a new server config section and env overrides:

- YAML (ployd):
  - `object_store.endpoint`
  - `object_store.bucket`
  - `object_store.access_key`
  - `object_store.secret_key`
  - `object_store.secure` (bool; default `false` for local MinIO)
  - optional: `object_store.region`

Env vars (preferred for secrets):

- `PLOY_OBJECTSTORE_ENDPOINT`
- `PLOY_OBJECTSTORE_BUCKET`
- `PLOY_OBJECTSTORE_ACCESS_KEY`
- `PLOY_OBJECTSTORE_SECRET_KEY`
- `PLOY_OBJECTSTORE_SECURE`

Update documentation:

- `docs/envs/README.md` (new variables)
- `docs/api/...` only if API payloads change (this design keeps API payloads unchanged).

### Retention / TTL

DB retention is already handled by the TTL worker (`internal/store/ttlworker`), which deletes rows older than a cutoff and can drop partitions.

For MinIO objects, prefer **bucket lifecycle rules** (server-side GC):

- Configure MinIO bucket lifecycle to expire objects after the same TTL as `scheduler.ttl` (default 30 days).
- This keeps `drop_partitions=true` safe (dropping DB partitions will not leak objects forever).

If we need exact deletion semantics (optional follow-up):

- Add TTL-worker integration that lists expired object keys before deleting rows and deletes them via `blobstore.Delete`.
- This requires additional queries to retrieve keys for the expired set and an explicit stance on `drop_partitions` (likely disable drop-partitions when using key-based deletion).

## Local Deployment (docker-compose)

Add MinIO to `local/docker-compose.yml`:

- Example sketch (names/ports can be adjusted):

```yaml
  minio:
    image: minio/minio:latest
    command: ["server", "/data", "--console-address", ":8999"]
    environment:
      - MINIO_ROOT_USER=ploy
      - MINIO_ROOT_PASSWORD=ploy-unsafe-local
    ports:
      - "9000:9000"
      - "8999:8999"
    volumes:
      - minio-data:/data
    healthcheck:
      test: ["CMD-SHELL", "curl -fsS http://localhost:9000/minio/health/live >/dev/null || exit 1"]
      interval: 5s
      timeout: 3s
      retries: 20

  minio-init:
    image: minio/mc:latest
    depends_on:
      minio:
        condition: service_healthy
    entrypoint:
      [
        "/bin/sh",
        "-lc",
        "mc alias set local http://minio:9000 ploy ploy-unsafe-local && mc mb -p local/ploy && exit 0",
      ]

volumes:
  minio-data:
```

- `minio` service:
  - image `minio/minio`
  - ports:
    - `9000` (S3 API) exposed to localhost for debugging
    - `8999` (console) optional
  - volume `minio-data:/data`
  - env `MINIO_ROOT_USER`, `MINIO_ROOT_PASSWORD`
  - healthcheck on `/minio/health/live`
- `minio-init` one-shot service (recommended):
  - image `minio/mc`
  - depends_on `minio: healthy`
  - creates bucket (e.g. `ploy`)
  - optionally sets lifecycle expiration

Wire the server container to MinIO:

- add `depends_on: minio`
- add `PLOY_OBJECTSTORE_*` env vars pointing to `http://minio:9000`

Update `scripts/deploy-locally.sh` to wait for MinIO health (if needed).

## Follow-up work (implementation slice)

1. Update `internal/store/schema.sql` + bump `internal/store/migrations.go:SchemaVersion`.
2. Update SQLC queries to remove blob columns and add `*_size` + `object_key`.
3. Introduce `internal/blobstore` + MinIO implementation.
4. Update handlers + `events.Service` to use blobstore.
5. Update local stack (`local/docker-compose.yml`) + bucket init.
6. Update/adjust tests that assert blob columns exist or are excluded from meta queries.
