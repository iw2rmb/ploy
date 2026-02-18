# Garage-backed Object Storage Migration

Scope: Replace MinIO in local and server wiring with Garage while keeping a single S3-compatible backend contract. No backward compatibility, no dual-backend runtime.

Documentation: `design/garage.md`, `docs/envs/README.md`, `local/docker-compose.yml`, `scripts/deploy-locally.sh`, `cmd/ployd/main.go`, `internal/blobstore/s3/s3.go`.

Legend: [ ] todo, [x] done.

## Phase 1 - Backend Adapter Cleanup
- [x] Replace MinIO-specific package naming with provider-neutral S3 adapter - Removes provider lock in code architecture.
  - Repository: `ploy`
  - Component: `internal/blobstore`, `cmd/ployd`
  - Scope: Move `internal/blobstore/minio` to `internal/blobstore/s3`; rename package and symbols (`minio.New` call remains if SDK stays unchanged); update imports in `cmd/ployd/main.go`; update comments and error prefixes from `minio:` to `s3:`.
  - Snippets: `bss3 "github.com/iw2rmb/ploy/internal/blobstore/s3"`
  - Tests: `go test ./internal/blobstore/... ./cmd/ployd/...` - Build and unit tests must pass with renamed adapter.

- [x] Decide SDK strategy for Garage compatibility - Reduces integration risk before infra cutover.
  - Repository: `ploy`
  - Component: `internal/blobstore/s3`, `go.mod`
  - Scope: Replace `minio-go` with AWS SDK v2 S3 client in one step; remove unused MinIO dependency set.
  - Snippets: `go mod tidy`
  - Tests: `go test ./internal/blobstore/...` - Adapter tests must pass with the selected client.

## Phase 2 - Local Infrastructure Cutover
- [x] Replace `minio` and `minio-init` services with Garage services in local compose - Makes Garage the only local object store.
  - Repository: `ploy`
  - Component: `local/docker-compose.yml`
  - Scope: Remove MinIO services/volume/depends_on entries; add Garage service definitions, persistent volume, healthcheck, and init step for bucket and access keys; keep server object-store env wiring pointed at Garage endpoint.
  - Snippets: `PLOY_OBJECTSTORE_ENDPOINT=http://garage:3900`
  - Tests: `docker compose -f local/docker-compose.yml up -d --no-build` - Garage service must become healthy and reachable.

- [x] Update local deploy automation to Garage readiness and bootstrap flow - Keeps one-command local bring-up working.
  - Repository: `ploy`
  - Component: `scripts/deploy-locally.sh`
  - Scope: Replace any MinIO readiness assumptions with Garage readiness checks; ensure bucket/bootstrap completion happens before server start assumptions.
  - Snippets: `$COMPOSE_CMD ps`
  - Tests: `scripts/deploy-locally.sh` - Full local deployment must complete without manual object-store setup.

## Phase 3 - Documentation and Configuration Alignment
- [x] Rewrite object-store docs from MinIO-specific to Garage-specific local defaults - Keeps docs consistent with implementation.
  - Repository: `ploy`
  - Component: `docs/envs/README.md`
  - Scope: Update examples and wording from `http://minio:9000` and `minio-init` to Garage endpoint/bootstrap references; keep env var names unchanged unless explicitly changed in code.
  - Snippets: `PLOY_OBJECTSTORE_*`
  - Tests: Manual doc review - Env table and local stack description must match compose and script behavior.

- [x] Replace MinIO design/roadmap references with Garage migration docs - Removes stale architecture guidance.
  - Repository: `ploy`
  - Component: `design/`, `roadmap/`
  - Scope: Replace MinIO-specific docs with `design/garage.md`; remove stale MinIO design/roadmap docs; keep cross-references aligned with Garage local profile.
  - Snippets: `design/garage.md`
  - Tests: Manual doc review - Cross-references must be valid and non-contradictory.

## Phase 4 - End-to-End Validation and Cleanup
- [x] Validate blob write/read paths against Garage using exact local scenario - Confirms no runtime regression.
  - Repository: `ploy`
  - Component: server handlers, blobpersist, local cluster
  - Scope: Re-run local flow that writes logs/diffs/artifacts and downloads artifacts/diffs; verify object keys stored in DB and blobs exist in Garage bucket.
  - Snippets: `make test`
  - Tests: `make test` plus local smoke via `scripts/deploy-locally.sh` - Upload/download flows must succeed.
  - Evidence: `bash tests/e2e/garage/smoke.sh` verifies log/diff/artifact uploads, `ploy mod fetch`, `ploy run diff --download`, DB `object_key` rows, and Garage `HeadObject`.

- [x] Remove MinIO-only leftovers from code and dependencies - Completes cutover with no dead paths.
  - Repository: `ploy`
  - Component: `go.mod`, `go.sum`, `local/docker-compose.yml`, docs
  - Scope: Remove unused MinIO images, references, and Go deps if SDK changed; ensure no `minio` mentions remain except historical notes explicitly kept.
  - Snippets: `rg -n "minio|MINIO" cmd internal local docs design roadmap`
  - Tests: `make test` - Repository must compile and tests pass after cleanup.
  - Evidence: comment cleanup in blobpersist/store/query sources plus `internal/store/minio_reference_guard_test.go` to prevent regressions.
