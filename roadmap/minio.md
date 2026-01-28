# MinIO-backed Log + Artifact Storage

Scope: Move log/diff/artifact blob bytes out of PostgreSQL `BYTEA` and into MinIO (S3-compatible), keeping only metadata + deterministic object references in Postgres. This is a plan only.

Documentation: `design/minio.md`, `docs/envs/README.md`, `GOLANG.md`, `docs/testing-workflow.md`, `local/docker-compose.yml`, `scripts/deploy-locally.sh`, `internal/store/schema.sql`.

Legend: [ ] todo, [x] done.

## Database Schema + sqlc
- [x] Replace `logs.data BYTEA` with size + generated `object_key` — Removes DB blob storage for logs.
  - Repository: `ploy`
  - Component: `internal/store`
  - Scope:
    - Update `internal/store/schema.sql`:
      - `logs`: drop `data BYTEA ...`; add `data_size BIGINT NOT NULL CHECK (data_size > 0)` and `object_key TEXT GENERATED ALWAYS AS (...) STORED`.
      - Prefer deterministic key expression (from `design/minio.md`):
        - `'logs/run/' || run_id || '/job/' || COALESCE(job_id, 'none') || '/chunk/' || chunk_no::text || '/log/' || id::text || '.gz'`
    - Bump `internal/store/migrations.go` `SchemaVersion` (breaking change; no backwards compatibility).
    - Blast radius: schema + version bump; estimate: ~20–40 minutes.
  - Snippets:
    - `data_size BIGINT NOT NULL CHECK (data_size > 0)`
    - `object_key TEXT GENERATED ALWAYS AS ('logs/run/' || ...) STORED`
  - Tests: `make test` — Expect store + server packages compile after downstream query updates.

- [x] Replace `diffs.patch BYTEA` with size + generated `object_key` — Removes DB blob storage for diffs.
  - Repository: `ploy`
  - Component: `internal/store`
  - Scope:
    - Update `internal/store/schema.sql` `diffs` table:
      - drop `patch BYTEA ...`; add `patch_size BIGINT NOT NULL CHECK (patch_size > 0)` and generated `object_key`.
    - Choose a deterministic key (from `design/minio.md`):
      - `diffs/run/{run_id}/diff/{diff_uuid}.patch.gz`
    - Blast radius: schema only; estimate: ~10–20 minutes.
  - Snippets:
    - `patch_size BIGINT NOT NULL CHECK (patch_size > 0)`
  - Tests: `make test` — Expect compile after sqlc changes.

- [x] Replace `artifact_bundles.bundle BYTEA` with size + generated `object_key` — Removes DB blob storage for artifacts.
  - Repository: `ploy`
  - Component: `internal/store`
  - Scope:
    - Update `internal/store/schema.sql` `artifact_bundles` table:
      - drop `bundle BYTEA ...`; add `bundle_size BIGINT NOT NULL CHECK (bundle_size > 0)` and generated `object_key`.
    - Choose a deterministic key (from `design/minio.md`):
      - `artifacts/run/{run_id}/bundle/{bundle_uuid}.tar.gz`
    - Blast radius: schema only; estimate: ~10–20 minutes.
  - Snippets:
    - `bundle_size BIGINT NOT NULL CHECK (bundle_size > 0)`
  - Tests: `make test` — Expect compile after sqlc changes.

- [x] Update sqlc queries to stop reading/writing blob bytes — Keeps DB as metadata + references only.
  - Repository: `ploy`
  - Component: `internal/store` (sqlc)
  - Scope:
    - Update:
      - `internal/store/queries/logs.sql`
      - `internal/store/queries/diffs.sql`
      - `internal/store/queries/artifact_bundles.sql`
    - Changes:
      - “Create” queries: accept `*_size` and do not accept blob bytes.
      - “Get” queries: return `object_key` (and do not return blob bytes).
      - “Meta/list” queries: return `*_size` (stop using `octet_length(blob)`).
    - Regenerate sqlc outputs (committed generated files under `internal/store/*.sql.go`) via `sqlc` using `sqlc.yaml`.
    - Blast radius: 3 query files + regenerated sqlc output; estimate: ~30–60 minutes.
  - Snippets:
    - `sqlc generate -f sqlc.yaml`
  - Tests: `make test` — Expect store + server compile with updated query shapes.

## Blobstore (MinIO)
- [x] Introduce `internal/blobstore` abstraction + MinIO implementation — Centralizes S3 I/O behind a small interface.
  - Repository: `ploy`
  - Component: `internal/blobstore`
  - Scope:
    - Add package `internal/blobstore`:
      - `Put(ctx, key, contentType string, data []byte) (etag string, err error)`
      - `Get(ctx, key string) (rc io.ReadCloser, size int64, err error)`
      - `Delete(ctx, key string) error`
    - Add implementation `internal/blobstore/minio` using `github.com/minio/minio-go/v7`.
    - Wire config inputs required by `design/minio.md`:
      - endpoint, bucket, access_key, secret_key, secure, (optional) region.
    - Blast radius: new packages + go.mod dependency; estimate: ~45–90 minutes.
  - Snippets:
    - `minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4(...), Secure: secure})`
  - Tests: `make test` — Expect compile; prefer unit tests with mocked interface (no MinIO required).

## Server Writes (Upload paths)
- [x] Add a server-side “blob persistence” service to make DB+MinIO writes consistent — Single call sites for logs/diffs/artifacts.
  - Repository: `ploy`
  - Component: `internal/server`
  - Scope:
    - Add `internal/server/blobpersist` (or equivalent) with methods:
      - `CreateLog(ctx, params, data []byte) (store.Log, err)`
      - `CreateDiff(ctx, params, patch []byte) (store.Diff, err)`
      - `CreateArtifactBundle(ctx, params, bundle []byte) (store.ArtifactBundle, err)`
    - Each method:
      - Inserts metadata row (`*_size=len(data)`), reads returned `object_key`.
      - Uploads bytes to MinIO (`blobstore.Put`).
    - Callers switch from `store.Create*` to this service.
    - Blast radius: new server package + handler/service call sites; estimate: ~1–2 hours.
  - Snippets:
    - `row, err := s.store.CreateLog(ctx, store.CreateLogParams{..., DataSize: int64(len(data))})`
  - Tests: `make test` — Expect handler/service tests updated to validate DB writes + blobstore calls via mock.

- [x] Update log ingestion handlers to pass gzipped bytes to SSE fanout without reading from DB — Required because `logs.data` column is removed.
  - Repository: `ploy`
  - Component: `internal/server/handlers`, `internal/server/events`
  - Scope:
    - Update log upload handlers:
      - `internal/server/handlers/nodes_logs.go`
      - `internal/server/handlers/runs_events.go`
    - Ensure the handler decodes base64 to gzipped bytes, then:
      - persists metadata + uploads bytes via blobpersist
      - fans out to SSE hub using the incoming bytes
    - Update `internal/server/events/service.go`:
      - Replace `publishLogToHub(ctx, runID, log store.Log)` with a version that accepts the gzipped bytes (and still enriches by job metadata).
      - Remove all use of `log.Data` (no longer exists).
    - Blast radius: 3–4 files + tests; estimate: ~1–2 hours.
  - Snippets:
    - `zr, err := gzip.NewReader(bytes.NewReader(gzipped))`
  - Tests: `go test ./internal/server/events -run CreateAndPublishLog` — Expect log fanout works without DB blob bytes.

- [x] Update diff + artifact upload handlers to store bytes in MinIO — Completes write-path migration.
  - Repository: `ploy`
  - Component: `internal/server/handlers`
  - Scope:
    - Update:
      - `internal/server/handlers/jobs_diff.go` (diff upload)
      - `internal/server/handlers/jobs_artifact.go` (artifact bundle upload)
    - Replace direct `store.CreateDiff` / `store.CreateArtifactBundle` blob writes with blobpersist calls.
    - Blast radius: 2 files + tests; estimate: ~45–90 minutes.
  - Snippets:
    - `_, err := blobpersist.CreateDiff(ctx, params, gzippedPatch)`
  - Tests: `go test ./internal/server/handlers -run 'Diff|Artifact'` — Expect upload endpoints accept payloads and persist metadata.

## Server Reads (Download paths)
- [x] Stream diff + artifact bytes from MinIO in download handlers — Removes DB reads of blob bytes.
  - Repository: `ploy`
  - Component: `internal/server/handlers`
  - Scope:
    - Update:
      - `internal/server/handlers/diffs.go` (download=true mode)
      - `internal/server/handlers/artifacts_download.go` (download=true mode)
    - Replace DB blob reads with:
      - query DB for `object_key` (+ size if needed)
      - `blobstore.Get` and stream to `http.ResponseWriter`
    - Preserve existing response headers/behavior (content type, attachment naming) where applicable.
    - Blast radius: 2 files + tests; estimate: ~45–90 minutes.
  - Snippets:
    - `rc, _, err := bs.Get(ctx, diff.ObjectKey); defer rc.Close()`
  - Tests: `go test ./internal/server/handlers -run 'Diffs|ArtifactsDownload'` — Expect identical bytes returned as before, sourced via mock blobstore.

## Config + Local Stack
- [x] Add server config and env vars for MinIO credentials — Makes object store configurable and secrets env-driven.
  - Repository: `ploy`
  - Component: `internal/server` config + `docs/envs`
  - Scope:
    - Add config fields (YAML + env) per `design/minio.md`:
      - `object_store.endpoint`, `bucket`, `access_key`, `secret_key`, `secure`, optional `region`.
      - Env vars: `PLOY_OBJECTSTORE_ENDPOINT`, `PLOY_OBJECTSTORE_BUCKET`, `PLOY_OBJECTSTORE_ACCESS_KEY`, `PLOY_OBJECTSTORE_SECRET_KEY`, `PLOY_OBJECTSTORE_SECURE`.
    - Update `docs/envs/README.md` to document new variables.
    - Blast radius: config + docs; estimate: ~30–60 minutes.
  - Snippets:
    - `PLOY_OBJECTSTORE_ENDPOINT=http://minio:9000`
  - Tests: `make test` — Expect config parsing unit tests updated/added if present.

- [x] Add MinIO services + bucket init to local docker-compose — Enables local dev + E2E without extra setup.
  - Repository: `ploy`
  - Component: `local/` + `scripts/`
  - Scope:
    - Update `local/docker-compose.yml`:
      - Add `minio` service (S3 API `:9000`, optional console `:8999`) + `minio-data` volume.
      - Add `minio-init` one-shot service using `minio/mc` to create the bucket (e.g. `ploy`).
      - Wire server container env vars `PLOY_OBJECTSTORE_*` to point at `http://minio:9000`.
    - Update `scripts/deploy-locally.sh` to wait for MinIO health if required.
    - Blast radius: compose + script; estimate: ~30–60 minutes.
  - Snippets:
    - `mc alias set local http://minio:9000 ploy ploy-unsafe-local && mc mb -p local/ploy`
  - Tests: `scripts/deploy-locally.sh` — Expect local cluster comes up with MinIO healthy and bucket present.

## Retention / TTL
- [x] Decide and document MinIO object GC strategy (lifecycle vs TTL-worker deletes) — Prevents unbounded object growth.
  - Repository: `ploy`
  - Component: `local/docker-compose.yml`, docs
  - Scope:
    - Preferred (from `design/minio.md`): bucket lifecycle expiration aligned with `scheduler.ttl` (default 30 days).
    - For local: optionally apply lifecycle rules via `minio-init` (`mc ilm`).
    - If exact deletion semantics are required later: plan a follow-up slice integrating deletes into `internal/store/ttlworker` (explicitly optional).
    - Blast radius: compose + docs; estimate: ~20–40 minutes.
  - Snippets:
    - `mc ilm add --expire-days 30 local/ploy`
  - Tests: Manual local verification (`mc ilm ls local/ploy`) — Expect expiration rule present.
