# Reuse CA/Cluster ID + Rollout

Scope: Implement an idempotent control‑plane deploy that reuses an existing cluster CA and server identity when present on the VPS, plus first‑class rollout commands for server and nodes with proper draining and health checks.

Documentation: docs/how-to/deploy-a-cluster.md, docs/how-to/update-a-cluster.md, docs/api/OpenAPI.yaml

Legend: [ ] todo, [x] done.

## Phase 1 — Reuse & Admin Cert Refresh
- [ ] Detect existing cluster on target host — enable idempotent deploy
  - Change: add `internal/deploy/detect.go: DetectExisting(ctx, runner, ProvisionOptions)` to probe `/etc/ploy/pki/ca.crt`, `/etc/ploy/ployd.yaml`; parse server cert CN → `ployd-<clusterID>`.
  - Test: `internal/deploy/detect_test.go` — stub runner to return sample files; expect `found=true`, correct cluster ID.
- [ ] Server deploy flags to control reuse vs. new CA
  - Change: `cmd/ploy/server_command.go` — add flags `--reuse` (default true), `--force-new-ca`, `--refresh-admin-cert`; branch logic to call `DetectExisting` and skip CA/server mint & bootstrap PKI when reusing.
  - Test: `cmd/ploy/server_command_test.go` — table tests covering reuse/new; ensure bootstrap env omits `PLOY_CA_*` when reusing.
- [ ] PKI: admin CSR signing endpoint (mTLS, cli-admin only)
  - Change: `internal/server/handlers/handlers_pki_admin.go` — `POST /v1/pki/sign/admin`; enforce EKU=ClientAuth and OU=“Ploy role=cli-admin”. Wire in `register.go`.
  - Test: handlers unit tests — accept valid CSR under cli-admin; reject missing role / wrong OU / wrong EKU.
- [ ] CLI admin cert refresh via server
  - Change: `cmd/ploy/server_command.go` — when `--refresh-admin-cert` set (or default if descriptor lacks TLS), generate local CSR and call `/v1/pki/sign/admin`; write `~/.config/ploy/certs/<cluster>-{ca,admin}.{crt,key}`; update descriptor CAPath/CertPath/KeyPath.
  - Test: `cmd/ploy/server_command_test.go` — stub HTTP server; expect files written and descriptor updated.
- [ ] Bootstrap must not clobber existing PKI on primary
  - Change: `internal/deploy/bootstrap/bootstrap.go` — if `/etc/ploy/pki/ca.key` exists, skip writing any PKI files; log reuse.
  - Test: `internal/deploy/bootstrap_test.go` — script contains reuse branch and omits CA/server writes.
- [ ] Docs update (deploy)
  - Change: `docs/how-to/deploy-a-cluster.md` — add “Reuse existing cluster” section; document `--reuse/--force-new-ca/--refresh-admin-cert` and expected outputs.
  - Test: run `markdownlint` and sanity‑read links.

## Phase 2 — Server Rollout
- [ ] Rollout command for control plane
  - Change: `cmd/ploy/rollout_server.go` — `ploy rollout server --address <A> [--binary <path>] [--timeout 60s]` → scp, restart, poll active, verify `ss -tlnp` and metrics.
  - Test: use recording runner to assert command sequence & retries; `-short` guard for timing.
- [ ] Docs update (update guide)
  - Change: `docs/how-to/update-a-cluster.md` — replace ad‑hoc scp/ssh with `ploy rollout server`; keep old commands in an appendix (“Backdoor”).
  - Test: lint docs; check cross‑references.

## Phase 3 — Node Drain + Rollout
- [ ] DB: add drained flag for nodes
  - Change: `internal/store/migrations/00X_nodes_drain.sql` — `ALTER TABLE ploy.nodes ADD COLUMN drained BOOLEAN NOT NULL DEFAULT false;` + indexes.
  - Test: migration unit test passes; ensure idempotency.
- [ ] API: drain/undrain endpoints + list nodes
  - Change: `internal/server/handlers/handlers_worker_drain.go` — `POST /v1/nodes/{id}/drain` and `/undrain` (RoleControlPlane), `GET /v1/nodes` (read‑only); wire in `register.go`.
  - Test: handler tests — state toggles, bad IDs, 404/409 paths.
- [ ] Scheduler: exclude drained nodes from claims
  - Change: store query (`internal/store/queries/*.sql` and generated code) to append `AND nodes.drained=false` in claim path.
  - Test: unit tests to verify drained nodes don’t claim runs.
- [ ] Rollout command for nodes (batched)
  - Change: `cmd/ploy/rollout_nodes.go` — `ploy rollout nodes [--all|--selector <pattern>] [--concurrency N] [--binary <path>] [--timeout 90s]` → drain → wait idle → update binary → restart → heartbeat OK → undrain. Write a resume file under `~/.config/ploy/rollout/state.json`.
  - Test: recording runner + fake API; ensure batch ordering, retries, resume.
- [ ] Docs update (update guide)
  - Change: add “Rolling update of nodes” section with examples and concurrency guidance.
  - Test: docs lint.

## Phase 4 — Hardening & UX
- [ ] Dry‑run probes and verbose output
  - Change: `ploy server deploy --reuse --dry-run` prints detected cluster, cert subjects, and no changes applied. `ploy rollout --dry-run` prints planned actions per host.
  - Test: CLI golden tests for messages.
- [ ] Resume & backoff tuning
  - Change: rollout retry policy (exponential backoff; max attempts configurable); resume file schema and integrity.
  - Test: unit tests simulate failures, assert resume continues remaining hosts.
- [ ] Metrics & logs
  - Change: emit structured logs for rollout steps; add basic counters exposed via CLI (optional).
  - Test: integration‑style smoke with `-short` guards.

## Acceptance & Cutover
- [ ] Replace “backdoor” steps in docs with first‑class `ploy rollout` flows; keep the legacy script as an appendix.
  - Change: docs PR; link from README “Updating a cluster”.
  - Test: manual smoke in VPS lab following the new commands end‑to‑end (server + two nodes).

