CHECKPOINT — Ploy refactor status (2025‑11‑04)

Summary
- Goal: align code and docs with the Postgres/mTLS pivot; remove legacy APIs; make CLI+server+node surfaces consistent; prepare for submodule regrouping.
- Status: ROADMAP.md implemented in full; simplified `/v1/mods` facade live; docs updated; next step is internal/ folder regrouping.

Key Decisions
- Artifacts/logs/diffs now live behind the runs API; the old /v2 and legacy jobs endpoints are gone.
- CLI “jobs” namespace is renamed to “runs” (behavior unchanged; only the command name changed).
- The server exposes /v1/mods/* for read/stream and legacy write endpoints at /v1/runs/{id}/*, plus /v1/nodes/*; all /v1/jobs* and /v1/mods/{ticket}/logs/stream have been removed.
- PKI: implemented a working certificate renewal (rotator) using the server CA from env.
- Config: removed obsolete ControlPlane job endpoints and other legacy per‑path knobs.

Completed Changes (code + docs)
1) Legacy removal
   - Deleted all //go:build legacy files and tests across deploy/node/lifecycle and CLI stubs.
   - Removed legacy job write endpoints and their tests.

2) CLI HTTPS transfers
   - Rewired: `ploy upload` → POST /v1/runs/{id}/artifact_bundles (JSON base64 body, 1 MiB gz cap).
   - Removed: `ploy report` (no GET artifact endpoint exists).

3) Runs/logs API and CLI rename
   - Streaming: CLI now hits GET /v1/mods/{id}/events for both:
     - `ploy runs follow <run-id>` (was: jobs follow)
     - `ploy mods logs <run-id>` (was: /v1/mods/{ticket}/logs/stream)
   - Inspect: `ploy runs inspect <run-id>` (was: jobs inspect)
   - Removed CLI subcommands: jobs ls, jobs retry.
   - Updated auto‑completions and help goldens.

4) Server endpoints (cmd/ployd)
   - Present: /v1/mods, /v1/mods/{id}, /v1/mods/{id}/events, /v1/runs/{id}/timing, /v1/runs/{id}/logs|diffs|artifact_bundles, /v1/pki/sign,
              /v1/nodes/{id}/heartbeat|claim|ack|complete|events|logs|stage/{stage}/diff|stage/{stage}/artifact.
   - Removed: all /v1/jobs*, and /v1/mods/{ticket}/logs/stream.

5) OpenAPI + docs
   - OpenAPI reflects `/v1/mods` facade; removed `/v1/mods/crud` and `/v1/runs/*` reads/streams.
   - Docs updated: deployment guide uses /v1/mods/* for submit/status/events; README API overview lists `/v1/mods` + artifacts endpoints.

6) PKI rotation TODO completed
   - internal/server/pki/rotator.go: implements on‑disk renewal when cert expiry enters `pki.renew_before`, issuing a new cert with the same Subject/SANs, using CA from env:
     - `PLOY_SERVER_CA_CERT`, `PLOY_SERVER_CA_KEY`.

7) Config cleanup
   - Removed ControlPlaneConfig fields: job_* endpoints, health/config/assignments/node_status, log_stream/artifact/metrics/admin, control_plane_ca_cache_path.
   - Defaults adjusted accordingly; config tests updated and passing.

Validation Snapshot
- Build: `make build` → dist/ploy, dist/ployd OK.
- Tests: `make test` → all green.
- Coverage (indicative): overall ≈ 65%; critical server/node packages ≥ 60%; PKI manager ≈ 76%.

Current CLI Surface (top‑level)
- mod …
- mods logs <run-id>
- runs follow <run-id>
- runs inspect <run-id>
- upload …
- cluster, config, manifest, knowledge‑base, server, node

Current API Highlights
- Logs/Events: `GET /v1/mods/{id}/events` (SSE)
- Artifacts: `POST /v1/runs/{id}/artifact_bundles`
- Diffs/Logs ingest: `POST /v1/runs/{id}/diffs|logs`
- Node control: `POST /v1/nodes/{id}/heartbeat|claim|ack|complete`, plus node logs/diffs/artifact routes.
- PKI: `POST /v1/pki/sign`

Removed/Breaking
- All `/v1/jobs*` and `/v1/mods/{ticket}/logs/stream` endpoints.
- CLI commands: `jobs follow/ls/inspect/retry` → replaced by `runs follow/inspect`.
- `report` CLI command.
- ControlPlane job_* and other legacy endpoint configs.

Repo Notes (examples of deletions/moves)
- Deleted: cmd/ployd/handlers_jobs_legacy.go and tests referencing legacy endpoints.
- Deleted OpenAPI jobs/mods logs path files.
- Rewired CLI streaming/tests under runs.
- Implemented PKI rotator + test: internal/server/pki/rotator_renew_test.go.

Proposed Folder Regrouping (next steps)
Objective: simplify internal/ by role without duplicating domain packages.

1) Rename internal/api → internal/server (no behavior change)
   - httpserver → server/http
   - events → server/events
   - metrics → server/metrics
   - status → server/status
   - scheduler → server/scheduler
   - api/pki (manager/rotator) → server/pki (keep crypto CA in internal/pki)

2) Move server HTTP handlers out of cmd/ployd into internal/server/handlers
   - Leave cmd/ployd/main.go as a thin bootstrap that composes server.Run.

3) Keep domain/shared packages where they are
   - internal/store, internal/workflow/*, internal/pki, internal/server/auth, …

4) Optional follow‑ups
   - Rename internal/cli/jobs package to internal/cli/runs (types still used by `runs` commands).
   - Add a depguard/staticcheck rule to prevent imports from internal/{node|cli} → internal/server.
   - Increment runner package coverage toward ≥90% by adding targeted tests.

Risk/Compatibility
- External clients/scripts using /v1/jobs* will break. All repo scripts and docs have been updated; audit any out‑of‑tree consumers.

Quick Commands
- Build CLI: `make build && dist/ploy help`
- Follow a run (SSE): `dist/ploy runs follow <run-id>`
- Upload artifact bundle: `dist/ploy upload --run-id <run-uuid> /path/to/file`
- Run tests: `make test`

Ready for Next Slice
- Completed on 2025-11-04:
  - internal/api → internal/server rename (httpserver → http).
  - Moved HTTP handlers out of cmd/ployd into internal/server/handlers and added RegisterRoutes.
  - Adjusted imports across repo; updated docs and coverage scripts.
  - Moved handler tests under `internal/server/handlers`; pruned handler tests from `cmd/ployd` and added minimal test helpers in `internal/server/handlers/`.
  - Removed temporary back‑compat aliases and deleted exported handler shims; tests now use in‑package (unexported) handlers.
  - make build, make test: green.

- Housekeeping:
  - Removed `ROADMAP.md` after final verification; replaced references with `CHECKPOINT.md` across docs.

- Next proposals:
  1) Rename internal/cli/jobs → internal/cli/runs (types preserved; keep command names aligned with runs).
  2) Add depguard/staticcheck rule to prevent imports from internal/{node|cli} → internal/server.
  3) Increment runner package coverage toward ≥90% with targeted tests.
