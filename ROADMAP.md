# ROADMAP — Complete the SIMPLE Architecture

This checklist breaks the migration into the smallest verifiable slices to ship a fully working Ploy aligned with SIMPLE.md. Keep RED → GREEN → REFACTOR. After each slice, run `make test` and sync docs.

References: SIMPLE.md, SIMPLE.sql, docs/api/OpenAPI.yaml, docs/envs/README.md, docs/how-to/deploy-a-cluster.md.

## Ground Rules
- [x] Adopt RED → GREEN → REFACTOR for every slice (unit first; E2E later).
- [x] Maintain docs parity: update README.md, docs/api, docs/envs, how-to guides per slice.
- [x] Keep coverage ≥60% overall; ≥90% on scheduler/PKI/ingest critical paths.
- [x] Verify required envs exist or add TODOs in docs/envs/README.md.

## Naming & Build Surface
- [x] Standardize server binary name to `ployd` (keep current path `cmd/ployd`).
- [x] Sweep docs for `ployd-server` and replace with `ployd` or add a one-line alias note in README.md.
- [x] Ensure `make build` emits `dist/ployd{,-linux}` consistently (Makefile already supports; verify).

## Server Bootstrap (Unstub cmd/ployd)
- [x] Replace `cmd/ployd/main.go` stub with real main: parse config/env, init logging, graceful shutdown.
- [x] Wire Postgres store via `PLOY_SERVER_PG_DSN` or `internal/api/config` (fallback to env over file).
- [x] Initialize Authorizer (mTLS) from `internal/controlplane/auth` with `RoleControlPlane` default.
- [x] Add HTTP mux package `internal/api/httpserver` (new) to mount routes and middlewares.
- [x] Expose metrics listener `:9100` (plain HTTP) and API listener `:8443` (TLS/mTLS).
- [x] Start background Scheduler `internal/api/scheduler` and TTL workers.
- [x] Integrate PKI manager (renew loop) with config hot-reload stub.

## API: PKI
- [x] Implement `POST /v1/pki/sign` handler (admin-only): parse CSR, sign with cluster CA, persist node cert metadata via store.
- [x] Return PEM bundle according to docs/api/components/schemas/pki.yaml.
- [x] Add 503 path when CA not configured (`PLOY_SERVER_CA_CERT`/`PLOY_SERVER_CA_KEY` absent).

## API: Control (Repos/Mods/Runs)
- [x] `POST /v1/repos` + `GET /v1/repos` (sqlc calls exist; wire round-trip + JSON).
- [x] `POST /v1/mods/crud` + `GET /v1/mods/crud?repo_id=`.
- [x] `POST /v1/runs` (create run; status=queued) returns `{run_id}`.
- [x] `GET /v1/runs?id` (basic run view) + `DELETE /v1/runs/{id}`.
- [x] `GET /v1/runs?view=timing` to read from `runs_timing`.

## API: Events/SSE
- [x] Add in-memory log/Event hub using `internal/node/logstream` for SSE fanout.
- [x] `GET /v1/runs/{id}/events` (SSE) with Last-Event-ID support.
- [x] Wrap DB append so that server both persists events and fans out to SSE.

## API: Node Ingest/Heartbeat
- [x] `POST /v1/nodes/{id}/heartbeat`: update `nodes` snapshot (cpu/mem/disk + version) and `last_heartbeat`.
- [x] `POST /v1/nodes/{id}/events`: append structured events/log frames to DB (size cap checks) + SSE fanout.
- [x] `POST /v1/nodes/{id}/stage/{stage}/diff`: store gzipped diff in `diffs` (≤1 MiB), reject oversize.
- [x] `POST /v1/nodes/{id}/stage/{stage}/artifact`: store gzipped bundle in `artifact_bundles` (≤1 MiB), reject oversize.

## Scheduling & Assignment
- [x] Implement `ClaimRun` RPC: server assigns one queued run via `FOR UPDATE SKIP LOCKED` (sqlc: `ClaimRun`).
- [x] Add server endpoint for claims (pull) or server→node push client (choose pull first per SIMPLE.md).
- [x] On assign, set `started_at`, status `assigned` then `running` when node acknowledges.
- [x] On completion callbacks, set `finished_at` and terminal status; populate `runs.stats`.

## TTL & Partitions
- [x] Mount `internal/store/ttlworker` in server: periodic deletes for `logs/events/diffs/artifact_bundles` older than retention.
- [x] Add partition lister + dropper integration (monthly tables) guarded by feature flag.

## Node Agent (Unstub default build)
- [x] Make `cmd/ployd-node/main.go` compile by default (remove `legacy` build tag; gate stub under a `stub` tag).
- [x] Ensure config loader `internal/nodeagent/config.go` and server start wire TLS client/server correctly.
- [x] Implement `POST /v1/run/start` and `POST /v1/run/stop` handlers (already present under build tag; enable).
- [x] Heartbeat manager: confirm `internal/nodeagent/heartbeat.go` posts to server endpoint with mTLS.
- [x] Add basic backoff for server 5xx on heartbeat.

## Node Execution Contract
- [x] Ephemeral workspace create/cleanup per run (tmpdir, unique prefix).
- [x] Shallow/sparse clone by repo URL; checkout `base_ref` then fetch `target_ref` or `commit_sha`.
- [x] Hook Build Gate (re-use lifecycle checker interfaces); capture per-stage/build timings.
- [x] Stream logs as gzipped chunks to server; enforce ≤1 MiB client-side cap.
- [x] Produce unified diff and summary; gzip and POST to server.
- [x] Upload artifact bundles (tar.gz) where configured.
- [x] Emit terminal status + cleanup workspace.

## Store & Migrations
- [x] Apply SIMPLE.sql as migrations under `internal/store/migrations/`; verify `sqlc` queries cover needed paths.
- [x] Add migration runner (server startup) that ensures schema present; log version.
- [x] Expand/adjust sqlc queries as endpoints require (e.g., list-by-since for events/logs).

## CLI Surfaces (Server/Node)
- [x] `ploy server deploy`: verify CA+server cert generation, DSN handling, and `deploy.ProvisionHost` call path.
- [x] `ploy node add`: implement full provisioning: upload `ployd-node`, CSR flow to `/v1/pki/sign`, install certs, start service.
- [x] Save/refresh local cluster descriptor in `~/.config/ploy/clusters/` after each deploy/add.

## Bootstrap Script (Replace stub)
- [x] Teach `internal/bootstrap.PrefixedScript` to render a functional body:
  - [x] Create `/etc/ploy` and `/etc/ploy/pki`; write CA/server certs from env.
  - [x] When `PLOY_INSTALL_POSTGRESQL=true`, install PostgreSQL packages.
  - [x] Create DB user/db `ploy` with password; derive DSN and export `PLOY_SERVER_PG_DSN`.
  - [x] Write server config `/etc/ploy/ployd.yaml` (postgres.dsn + TLS paths).
  - [x] Write node config `/etc/ploy/ployd-node.yaml` on non-primary bootstraps.
  - [x] Install systemd unit `ployd.service` (server) or `ployd-node.service` (node) with `Restart=always`.
  - [x] `systemctl daemon-reload && systemctl enable --now <unit>`.
  - [x] Echo final status and key paths.
- [x] Extend `internal/deploy/provision_test.go` to assert config/unit fragments exist in script output.

## OpenAPI & Docs
- [x] Ensure docs/api OpenAPI matches implemented endpoints (PKI, repos/mods/runs, SSE, ingest, heartbeat).
- [x] Add examples for heartbeat payload and log/diff upload boundaries.
- [x] Update docs/how-to/deploy-a-cluster.md to match real bootstrap behavior (what gets written where).
- [x] Update docs/envs/README.md for final env names and defaults encountered in code.

## Security
- [x] Enforce TLS 1.3 and client cert verification everywhere (node→server and CLI→server).
- [x] Validate roles via `Authorizer` middleware; restrict PKI to `cli-admin`.
- [x] Scrub PII from logs via node-side hooks (document a placeholder; no-op first).

## Tests (Unit → Integration → Lab)
- [x] Unit: PKI CSR sign success/error paths.
- [x] Unit: Authorizer role gates and insecure default off.
- [x] Unit: Repos/Mods/Runs handlers JSON and status codes.
- [x] Unit: SSE hub resume with Last-Event-ID and concurrent subscribers.
- [x] Unit: Ingest caps (oversize gzipped chunks rejected with 413).
- [x] Unit: TTL worker deletes rows older than horizon; partition dropper no-ops when none.
- [x] Integration (local Postgres via `PLOY_TEST_PG_DSN`): happy path create repo→mod→run; simulate node appends.
- [x] Integration: server start/stop with mTLS disabled under tests (authorizer `AllowInsecure` only in tests).
- [x] CLI: `server deploy` flag validation and path resolution; `node add` flag validation + dry-run scaffolding.
- [x] Lab script: minimal smoke (server + one node): submit run to public repo; assert logs/diff rows stored.

## Legacy & Dead Code Removal
- [x] Remove etcd/registry codepaths in `internal/deploy/*` and tests (or guard behind legacy build tag).
- [x] Remove IPFS references and scripts (already mostly gone; sweep `scripts/` and docs).
- [ ] Delete `cmd/ployd-node/stub.go` once default build is real (or keep under `-tags stub`).
- [x] Consolidate exploration docs into README/SIMPLE; remove `ARCHITECTURE_DIAGRAM.md` and related (`CODEBASE_EXPLORATION.md`, `EXPLORATION_INDEX.md`, `EXPLORATION_README.md`).

## Acceptance Checklist
- [ ] Server starts with `PLOY_SERVER_PG_DSN` and serves all documented endpoints over mTLS on `:8443`.
- [ ] Node starts by default (no build tags) and can run the end-to-end flow: start → stream logs → upload diff/artifacts → finish.
- [ ] `make test` green; coverage thresholds met; docs up to date.
- [ ] VPS lab walkthrough in docs executes successfully with the provided IPs and commands.
