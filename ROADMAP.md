# ROADMAP — Ploy Server/Node Pivot (Postgres, No IPFS)

Key references to keep aligned during implementation:
- SIMPLE.md — architecture, API surface, caps, security
- SIMPLE.sql — canonical schema, constraints, indexes, timing view
- docs/api/OpenAPI.yaml — control-plane + node RPC (switch to mTLS, add new paths)
- docs/envs/README.md — server/node TLS + PG env vars; remove IPFS/etcd
- docs/how-to/deploy-a-cluster.md — deployment playbook changes
- README.md — high-level overview pointing to SIMPLE.md
- AGENTS.md — VPS lab scope and constraints

Guidance: follow RED → GREEN → REFACTOR. Keep steps small, verifiable, and update docs with each slice.

## Minimal Checklist

- [x] Add Postgres store scaffolding
  - [x] Introduce `sqlc.yaml`, `internal/store/migrations/` from SIMPLE.sql, and `internal/store/queries/`.
  - [x] Wire `pgx/v5` + `pgxpool` in server startup; inject store via interfaces.

- [x] PKI (mTLS) foundation
  - [x] Implement cluster CA generation and server cert issuance.
  - [x] Add `/v1/pki/sign` to sign node CSRs; persist node cert metadata (serial/fingerprint/notBefore/notAfter).

- [x] Core control-plane models and endpoints
  - [x] CRUD: `repos`, `mods`, `runs`; create-run returns `run_id`.
  - [x] SSE: `/v1/runs/{id}/events` (basic log/event fanout only).

- [x] Scheduler (minimal)
  - [x] Implement `FOR UPDATE SKIP LOCKED` claim on `runs.status='queued'`.
  - [x] Record `started_at`/`finished_at`; expose `runs_timing` view.

- [x] Node agent (ployd-node) skeleton
  - [x] HTTPS server with mTLS; endpoints: `/v1/run/start`, `/v1/run/stop`.
  - [x] Heartbeat POST with resource snapshot; update `nodes` row.

- [x] Node execution contract
  - [x] Ephemeral workspace + shallow/sparse clone from URL (branch/commit optional, default to remote default/HEAD).
  - [x] Execute Build Gate (node-only), stream logs; measure stage/build durations.
  - [x] Generate unified diff and upload; enforce client-side caps (≤1 MiB gz per diff/log chunk/bundle).

- [x] Artifact, diff, and log ingestion (server)
  - [x] POST endpoints to accept gzipped diffs, log chunks, and artifact bundles.
  - [x] Enforce DB constraints (1 MiB gziped) and reject oversize payloads.

- [x] TTL and partitions
  - [x] Add TTL worker for `logs`, `events`, `diffs`, `artifact_bundles` (default 30 days for bundles).
  - [x] Optional: daily partition dropper based on naming scheme (see SIMPLE.md snippet).

- [x] CLI commands
  - [ ] `ploy server deploy --address`: install server, create CA, issue server cert, create `cluster_id`, configure `PLOY_SERVER_PG_DSN`.
    - [ ] If `--postgresql-dns` is not provided, install PostgreSQL on the VPS and create DB `ploy`; derive DSN.
  - [ ] `ploy node add --cluster-id --address`: install node, generate key+CSR, call `/v1/pki/sign`, record node IP, configure mTLS.

- [ ] Remove legacy systems (code + scripts + docs)
  - [ ] Purge IPFS Cluster codepaths, health checks, installers, and envs.
  - [ ] Remove etcd clients/publishers and embedded etcd tests.
  - [ ] Drop node labels from APIs/CLI; replace with resource-snapshot scheduling.
  - [ ] Remove token-based auth; update OpenAPI to mTLS-only.
  - [ ] Remove `pkg/sshtransport` (no SSH tunnels in new architecture).

- [ ] Knowledge Base (in scope)
  - [ ] Keep `ploy knowledge-base ingest/evaluate` working with new layout.
  - [ ] Ensure Mods advisor consumes `configs/knowledge-base/catalog.json` and surfaces recommendations in runs.
  - [ ] Update docs: `configs/knowledge-base/README.md`, CLI reference.

- [ ] OpenAPI + docs pass
  - [ ] Update `docs/api/OpenAPI.yaml` to new endpoints and mTLS-only auth.
  - [ ] Refresh `docs/envs/README.md` with new envs; mark IPFS/etcd as legacy and remove.
  - [ ] Update how-to deploy and README; point operators to the new CLI flows.

- [ ] Tests and coverage
  - [ ] Unit: scheduler claim fairness/backoff; diff/log ingestion; PKI/CSR flow; resource-cap rejects.
  - [ ] Integration: one server + one node; submit run with a public repo; assert logs/diff stored and TTL job deletes old rows.
  - [ ] Target coverage: ≥60% overall; ≥90% critical runner packages.

## Acceptance criteria
- Server starts with Postgres DSN and mTLS; `/v1/runs` + SSE work.
- Node can receive `/v1/run/start`, fetch repo shallowly, run a build, stream logs, upload diff and an artifact bundle.
- DB enforces size caps; TTL worker prunes data; indices support basic dashboards via `runs_timing`.
- No remaining references to IPFS, etcd, tokens, or labels in code or docs.
- Works on VPS lab: one host as server+Postgres, two hosts as nodes.
- Migration note honored: no backward compatibility or data migration required; redeploy lab fresh.
- Knowledge Base CLI and advisor recommendations function as before (catalog from `configs/knowledge-base/catalog.json`).
