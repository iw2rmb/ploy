# SIMPLE: Ploy Server/Node Pivot (Postgres, No IPFS)

This document outlines a simplified architecture for Ploy that splits the
daemon into a control-plane server and lightweight nodes, replaces etcd with
PostgreSQL for state, and removes IPFS entirely. Repositories are fetched
shallow on-demand by nodes and removed when jobs finish.

## Why This Is Simpler

- Fewer moving parts: no etcd cluster and no IPFS Cluster/Gateway to deploy,
  secure, observe, and back up.
- One trust boundary: CLI talks only to the server; nodes talk only to the
  server. No peer-to-peer storage or cross-node artifact pinning.
- Operationally standard: PostgreSQL is a familiar, operable, and strongly
  consistent store with first‑class tooling (backups, HA, observability).
- Lean nodes: nodes run jobs, stream logs, and send results; they keep no
  durable state or long-lived caches.

Trade‑offs (and mitigations):
- More network to Git remotes due to ephemeral clones (mitigate via shallow
  clones and sparse filters; no server‑side mirroring by design).
- Larger DB when storing diffs/logs as blobs (mitigate via gzip, table
  partitioning, and TTL purge jobs; object storage can be added later behind an
  interface if PostgreSQL growth becomes a concern).
- No content-addressed dedup (mitigate via per‑run compression; server will not
  mirror repositories).

## Components

- ployd-server
  - HTTP/JSON API for CLI
  - Scheduler + job queue (in-DB, advisory locks)
  - Node registry + heartbeats
  - Event stream (SSE or WebSocket)
  - Storage in PostgreSQL (state, events, logs, diffs, artifact bundles, run metadata)
  - Secrets brokering to nodes (scoped repo credentials); no tokens

- ployd-node
  - Long-poll or streaming subscriber to the server queue
  - Executes a job in a clean, ephemeral workspace
  - Fetches repo via shallow `git clone`/`git fetch` from the source URL
  - Streams logs and posts stage events/diffs back to the server
  - Deletes workspace on completion; retains nothing locally

- ploy CLI
  - Talks only to ployd-server
  - Submits mods, follows logs, downloads artifacts via server endpoints

## Implementation Notes

- Storage access: use `pgx` (v5) with `pgxpool` for pooled connections.
- Query layer: use `sqlc` to generate typed Go code from SQL.
  - Keep schema/migrations under `internal/store/migrations/`.
  - Keep read/write queries under `internal/store/queries/`.
  - Types/enums defined in SQL (see `SIMPLE.sql`) map to Go via `sqlc.yaml`.
- Reference schema: see `SIMPLE.sql` for an initial database outline.

## Identity and Names

- Server DNS name: `ployd-server <cluster-id>.ploy`.
- Node DNS name: `ploy-node <node-id>.<cluster-id>.ploy`.
- Connections use HTTPS over IP addresses; certificates include both the DNS
  names above and the node/server IP in Subject Alternative Names (SANs).

## Data Model (PostgreSQL)

 Aim for compact, indexed, append-only writes per run. Initial sketch (server
 never stores repository contents; only metadata like URLs):

- cluster(id, created_at)  -- singleton row; server is single‑cluster
- nodes(id, name, ip_address, version, concurrency, last_heartbeat,
        cpu_total_millis, cpu_free_millis,
        mem_total_bytes, mem_free_bytes,
        disk_total_bytes, disk_free_bytes)
- repos(id, url, branch, commit_sha, created_at)  -- metadata only; branch/commit are optional hints
- mods(id, repo_id, spec jsonb, created_by, created_at)
- runs(id, mod_id, status, reason, created_at, started_at, finished_at,
       node_id, base_ref, target_ref, commit_sha, stats jsonb)
- stages(id, run_id, name, status, started_at, finished_at, meta jsonb)
- events(id, run_id, stage_id, time, level, message, meta jsonb)
- diffs(id, run_id, stage_id, patch gzip bytea, summary jsonb)  -- max 1 MiB gzipped
- logs(id, run_id, stage_id, build_id, chunk_no, data gzip bytea)  -- max 1 MiB gzipped
- artifact_bundles(id, run_id, stage_id, build_id, name, bundle gzip bytea)

Notes:
- Use `jsonb` for flexible metadata, with GIN indexes where needed.
- Partition `events`, `logs`, `artifact_bundles`, and `diffs` monthly; add TTL jobs to purge old data.
- Use advisory locks to assign runs to nodes atomically.
 - Indexes: add `runs(created_at)` for dashboards and `nodes(last_heartbeat)` for pruning. See `SIMPLE.sql`.
 - Convenience: `runs_timing` view provides `queue_ms` and `run_ms` per run.

## API Surface (MVP)

- POST /v1/mods: declare a mod (repo URL, base/target, options)
- POST /v1/runs: create a run from a mod (returns run_id)
- GET  /v1/runs/{id}: status summary
- GET  /v1/runs/{id}/events: server‑sent events stream
- GET  /v1/runs/{id}/diff: compressed unified diff for the run
- POST /v1/pki/sign: sign a CSR for a node certificate (admin‑only)
- POST /v1/nodes/{id}/events: stream/append events and logs
- POST /v1/nodes/{id}/stage/{stage}/diff: upload gzipped diff
- POST /v1/nodes/{id}/stage/{stage}/artifact: upload gzipped tar bundle(s)
- POST /v1/nodes/{id}/heartbeat: heartbeat with resource snapshot
  (cpu_free_millis, mem_free_bytes, disk_free_bytes) and version
  - Server validates caps: diff <= 1 MiB gzipped; each log chunk <= 1 MiB gzipped; each artifact bundle <= 1 MiB gzipped.

Server → Node (mTLS, initiated by server)
- POST https://ploy-node/<node-id>.<cluster-id>.ploy/v1/run/start: start a run
  with payload (run_id, repo_url, base_ref, target_ref, options).
- POST https://ploy-node/<node-id>.<cluster-id>.ploy/v1/run/stop: stop/cancel.

OpenAPI changes belong in `docs/api/OpenAPI.yaml` when implemented.
- Auth: remove bearer; mTLS only.
- Add server routes listed above; remove legacy IPFS/etcd artifacts.
- All submissions accept only repository URLs; the server never receives repo archives nor clones repos.

## Node Execution Contract (MVP)

- Inputs from server: repo URL (only a URL; no repo content is provided to the
  server), base ref, target ref, optional commit SHA,
  environment vars and secrets (scoped repo credentials), container image/runtime
  parameters if applicable.
- Steps on node:
  1) Create temp workspace (`mktemp -d`), `git init`, `git remote add origin`.
  2) Always shallow: `git fetch --depth=1 --filter=blob:none origin <base>`,
     branch/checkout; fetch additional refs as needed.
  3) Execute the Mod runner (Build Gate lives only on nodes).
  4) Collect unified diff: `git diff --binary --no-color <base>...HEAD`.
  5) Stream logs/events during execution; POST final diff + summary to server
     respecting caps (diff <= 1 MiB gzipped; log chunk <= 1 MiB gzipped).
  6) Delete workspace recursively.

Timing
- Node records `started_at`/`finished_at` and `duration_ms` for every stage.
- Node records `started_at`/`finished_at` and `duration_ms` for every build
  invocation (e.g., each Maven/Gradle/Bazel call), including tool and command.

## CLI Commands (MVP)

- `ploy server deploy --address <host-or-ip>`
  - Over SSH, installs `ployd-server`.
  - Generates a cluster Certificate Authority (CA), issues the server TLS
    certificate, and creates a `cluster_id` recorded in Postgres and on disk.
  - Bootstraps `ployd-server` systemd unit with `PLOY_SERVER_PG_DSN`.
  - If `--postgresql-dsn` is NOT provided, installs PostgreSQL on the VPS and
    provisions a database named `ploy`; derives the DSN for server config.

- `ploy node add --cluster-id <id> --address <host-or-ip>`
  - Over SSH, installs `ployd` and registers it with the server.
  - Generates a private key and CSR on the node; submits CSR to server for
    signing; installs the issued node certificate and CA bundle.
  - Records the node IP in the database.
  - Bootstraps `ployd` systemd unit; the endpoint derives from the cluster descriptor or `PLOY_CONTROL_PLANE_URL`.

## Scheduling

- In-DB queue table or `runs.status = "queued"` with `FOR UPDATE SKIP LOCKED`.
- Node asks for work with declared concurrency limits; server can also filter by
  resource snapshot (e.g., cpu_free_millis >= X and mem_free_bytes >= Y).

Optional pull vs push
- Server initiates run via Node RPC. Nodes still push heartbeats/events/logs to
  the server for simplicity and firewall friendliness.

## Components Split

- ployd-server
  - API + SSE, auth via mTLS only (no tokens)
  - PKI CA and CSR signing
  - Scheduler and run orchestration
  - Storage adapter (pgx/sqlc) for runs, stages, events, diffs, logs, artifact bundles
  - Log/diff size enforcement (<= 1 MiB gzipped each chunk or diff)
  - Node inventory and metrics snapshots + history (node_metrics)
  - Server→Node RPC client (mTLS)

- ployd-node
  - Build Gate (tooling adapters; Maven/Gradle/NPM/etc.)
  - Repo clone (URL only), workspace lifecycle, diff generation
  - Log streaming and artifact bundle packaging/upload
  - Stage/build timing and metrics collection
  - Heartbeat with resource snapshot (CPU/Mem/Disk)
  - mTLS endpoint for run/start + run/stop
- Backoff policies tracked per run; retries are server‑orchestrated.

## Security

- TLS: mutual TLS everywhere (no tokens).
- Git credentials are short‑lived, scoped to repo and run; delivered only to
  the selected node.
- Postgres creds managed via env/secret store on the server node only.
- Logs/diffs compressed in transit and at rest; PII scrubbing hooks in node.

Certificate issuance
- `ploy server deploy` creates a cluster CA (`<cluster-id>-ca.key/.crt`) and a
  server TLS cert with SANs for the server IP and `ployd-server` DNS name.
- `ploy node add` generates the node key on the node, builds a CSR with CN
  `node:<node-id>` and SANs for the node IP and `ploy-node` DNS name, submits
  it to the server’s PKI endpoint, and installs the signed cert and CA.
- The same CA is trusted by both sides for mTLS. Rotation is performed by
  re‑issuing node certs via the CSR flow.
- Node certificates carry EKUs for both `serverAuth` and `clientAuth` so they
  can be used for node→server and server→node HTTPS.
 - The CLI can use an operator client certificate minted during `ploy server deploy`
  (saved on the workstation) for admin routes; no tokens are required.

## Migration Checklist (from etcd/IPFS)

- Remove IPFS Cluster clients, publishers, health checks, and bootstrap installers.
- Remove etcd clients and publishers; replace with Postgres store using pgx/sqlc.
- Drop node labels from APIs, CLI descriptors, and scheduler filters.
- Update OpenAPI to mTLS‑only auth and new endpoints; remove bearer.
- Update `docs/envs/README.md` to new server/node TLS and PG env vars; remove IPFS/etcd and `PLOY_NODE_TOKEN`.
- Replace old artifact flows with DB‑backed diffs/logs/artifact bundles.
- Remove `pkg/sshtransport` (CLI-to-server is HTTPS/mTLS without SSH tunnels).
- Knowledge Base stays in scope: retain CLI commands and advisor integration; no
  server persistence is required for MVP (catalog remains workstation-side at
  `configs/knowledge-base/catalog.json`).
## Operations

- Ports: server `8443` (API/SSE), `9100` (metrics); Postgres `5432` (private).
- Backups: logical dumps for schema; WAL + base backups for point‑in‑time.
- Retention: TTL jobs purge `logs`, `diffs`, `events`, and `artifact_bundles`.
  Default TTL for artifact bundles is 30 days.
- Observability: Prometheus metrics, structured logs.

TTL enforcement (example)
- Prefer time‑based partitioning and drop whole partitions daily for speed.
- Example daily job (pseudocode using `psql` on the server):

  ```bash
  # Drop monthly partitions older than 30 days; fallback delete for stray rows
  psql "$PLOY_SERVER_PG_DSN" <<'SQL'
  -- Logs
  DO $$
  DECLARE part RECORD;
  BEGIN
    FOR part IN SELECT inhrelid::REGCLASS AS name
                FROM pg_inherits
                WHERE inhparent = 'ploy.logs'::REGCLASS LOOP
      EXECUTE format('DROP TABLE IF EXISTS %s CASCADE', part.name)
      WHERE now() - interval '30 days' > (
        SELECT to_timestamp(regexp_replace(part.name::text, '.*_(\d{4})_(\d{2})$', '\1-\2-01')::timestamp) + interval '1 month');
    END LOOP;
  END$$;
  DELETE FROM ploy.logs WHERE created_at < now() - interval '30 days';

  -- Events
  DELETE FROM ploy.events WHERE time < now() - interval '30 days';

  -- Diffs
  DELETE FROM ploy.diffs WHERE created_at < now() - interval '30 days';

  -- Artifact bundles
  DELETE FROM ploy.artifact_bundles WHERE created_at < now() - interval '30 days';
  SQL
  ```

  Replace with your partition naming scheme; for production use, implement a
  small Go worker in the server to manage partitions and TTL consistently.

## Deployment Topology (VPS Lab)

- Use the VPS lab for initial deployment and smoke:
  - One host runs `ployd-server` and PostgreSQL (if `--postgresql-dsn` is not
    provided during `ploy server deploy`, the installer sets up Postgres locally;
    DB name: `ploy`).
  - Two hosts run `ployd-node`.
- No backward compatibility or data migration is required when migrating VPS
  lab nodes to this version — redeploy fresh.

## Env Vars (proposed)

- Server
  - `PLOY_SERVER_HTTP_LISTEN` (default `:8443`)
  - `PLOY_SERVER_METRICS_LISTEN` (default `:9100`)
  - `PLOY_SERVER_PG_DSN` (e.g., `postgres://…`)
  - `PLOY_SERVER_CLUSTER_ID`
  - `PLOY_SERVER_TLS_CERT` / `PLOY_SERVER_TLS_KEY`
  - `PLOY_SERVER_CA_CERT` / `PLOY_SERVER_CA_KEY`

- Node
  - `PLOY_CA_CERT_PEM` — Cluster CA presented to the node for mTLS trust.
  - `PLOY_SERVER_CERT_PEM` / `PLOY_SERVER_KEY_PEM` — The node’s TLS certificate and key (CSR-signed by the control plane). Despite the name, bootstrap uses these variables for both server and node flows and writes to `/etc/ploy/pki/node.pem` and `/etc/ploy/pki/node-key.pem`.
  - `PLOY_NODE_CONCURRENCY` (default `1`)

- CLI
  - `PLOY_CONTROL_PLANE_URL` (optional override; descriptors remain preferred)

Legacy IPFS and etcd envs become no‑ops then removed after migration.

## Minimal Delivery Plan (RED → GREEN → REFACTOR)

- M0 (RED): Add failing tests describing server queueing, node assignment,
  run lifecycle, and diff upload; add OpenAPI stubs.
- M1 (GREEN): Implement ployd-server with Postgres migrations and minimal APIs;
  add ployd-node that executes a shell step and uploads a diff; CLI keeps
  current UX but targets new endpoints behind a feature flag.
- M2 (REFACTOR): Remove etcd/IPFS code paths; flip the feature flag on; update
  env docs and OpenAPI; partition large tables and add TTL jobs.

## Test Strategy

- Unit: scheduler (assignment fairness and backoff), diff ingestion, SSE fanout,
  PKI/CSR flows; Postgres migrations forward/backward.
- Integration (local): server + ephemeral node with a sample repo; assert logs,
  diff content, and cleanup of workspace.
- E2E (lab): deploy one server + N nodes; submit multiple concurrent mods and
  verify throughput and stability.

Performance/Storage
- Logs: stored in Postgres as gzipped chunks keyed by (run, stage, build,
  chunk_no). Hard cap per chunk: 1 MiB gzipped. Reject larger writes.
- Diffs: gzipped, hard cap: 1 MiB. Reject larger writes.
- Artifact bundles: gzipped tar stored per run/stage/build. Hard cap per bundle:
  1 MiB gzipped. Partitioned table with default TTL of 30 days.

## Open Questions

- Artifact bundle maximum size and retention policy — default TTL and cap TBD.

---

If we follow this plan, we remove two external systems (etcd, IPFS), collapse
all persistence and coordination into PostgreSQL, and cut node responsibilities
to the minimum. That reduces operational risk and day‑2 work while keeping a
clear path to add a mirror cache or object storage later if required.
