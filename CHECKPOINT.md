# Checkpoint — Mods ORW Artifacts (Nov 4, 2025, 13:46 local)

## Current Issue
- Artifacts (diff bundle) are not downloaded by the CLI after the ORW pass scenario. The artifact manifest at `./tmp/mods/mod-orw/<YYMMDDHHmmss>/manifest.json` shows an empty list.
- A direct mTLS call to the lab server `GET /v1/mods/{ticket}` returns the legacy, slim shape (no `stages`/`artifacts`), so the CLI has nothing to resolve/download.

## What Was Done (Code + Infra)
- Implemented stage-aware pipeline with no backward compatibility:
  - Server
    - Create a Stage on ticket submit.
    - Include `stage_id` in claim (merged into `spec`).
    - Update Stage to `running` on ACK; close Stage to terminal on complete; compute duration.
    - Return mods-style `TicketStatusResponse` with `Stages` and their `Artifacts` (via `ListArtifactBundlesByRunAndStage`).
  - Node
    - Use `Options["stage_id"]` for uploads.
    - Upload the diff both as a diff (existing endpoint) AND as an artifact bundle named `diff` (contains `diff.patch`).
    - NEW: Always bundle `/out` contents and upload as an artifact bundle named `mod-out` regardless of `artifact_paths`. Added unit tests (`internal/nodeagent/outdir_bundle_test.go`).
  - CLI
    - Unchanged; already resolves CIDs via `/v1/artifacts` endpoints and downloads to `--artifact-dir`.
- Built fresh binaries: `dist/ploy`, `dist/ployd-linux`, `dist/ployd-node-linux`.
- Deployed control-plane to A (45.9.42.212) and added node agents on B (46.173.16.177) and C (81.200.119.187).
- Ran scenario multiple times; tickets succeed; artifact manifest remains empty.

## Observations
- `curl` (with admin mTLS) against `https://45.9.42.212:8443/v1/mods/<ticket>` returns the old response shape. This strongly indicates the running server binary is still the pre-change build.
- We installed the new `ployd-linux` via `ploy server deploy`, but a subsequent restart attempt reported a transient failure; journal shows the service kept running and processed tickets, but apparently with the prior binary.

## Expected Solution
- Ensure the new server binary (with the updated `GET /v1/mods/{id}` handler) is running on A, then re-run the scenario:
  1) Replace `/usr/local/bin/ployd` on A with the freshly built `dist/ployd-linux`.
  2) Restart `ployd.service`, confirm `systemctl is-active` is `active`.
  3) Verify `GET /v1/mods/{ticket}` returns `TicketStatusResponse` (mods-style) with a `Stages` map and at least one `Artifacts` entry named `diff`.
  4) Run `tests/e2e/mods/scenario-orw-pass.sh`; confirm `./tmp/mods/mod-orw/<YYMMDDHHmmss>/manifest.json` lists the diff artifact and the .bin was fetched.

## Rollback Plan
- If the new binary fails to start, restore the previous binary from `/usr/local/bin/ployd` backup or re-run `ploy server deploy` with a known-good build; service logs via `journalctl -u ployd -f` should reveal config errors.

## Next Actions (Now)
- Stop control-plane safely, replace binary, and set a literal DSN:
  1) `systemctl stop ployd.service`
  2) Set/confirm a working DSN in `/etc/ploy/ployd.yaml` (literal, not `${...}`), for example:
     - `postgres://ploy:<PASSWORD>@localhost:5432/ploy?sslmode=disable`
     - If password unknown, reset: `sudo -u postgres psql -c "ALTER USER ploy WITH PASSWORD '<PASSWORD>';"`
  3) Copy the new server binary: `install -m0755 dist/ployd-linux /usr/local/bin/ployd`
  4) `systemctl daemon-reload && systemctl start ployd.service` and verify `systemctl is-active --quiet ployd.service`.
 - Verify `/v1/mods/{ticket}` returns mods-style TicketStatus (Stages+Artifacts).
 - Re-run `tests/e2e/mods/scenario-orw-pass.sh`; confirm `./tmp/mods/mod-orw/<YYMMDDHHmmss>/manifest.json` lists a `diff` artifact and `mod-out` bundle with a downloaded `.bin`.

## Update — DSN Standardization (Nov 4, 2025, 16:15)

- Server DSN env simplified: `PLOY_POSTGRES_DSN` only (removed `PLOY_SERVER_PG_DSN`).
- Bootstrap changes:
  - When PostgreSQL is installed on the server, bootstrap derives `PLOY_POSTGRES_DSN` and writes a literal `postgres.dsn` into `/etc/ploy/ployd.yaml` using an unquoted heredoc, so no runtime env is required.
  - For provided DSNs, bootstrap expands `${PLOY_POSTGRES_DSN:-}` into the file at bootstrap time, again producing a literal DSN.
- Code changes:
  - `cmd/ployd/config_dsn.go` — precedence is now `PLOY_POSTGRES_DSN` → `config.postgres.dsn` (placeholders `${...}` are treated as unset).
  - CLI/server deploy updated to pass `PLOY_POSTGRES_DSN` to the bootstrap when a DSN is provided.
  - Tests and docs updated accordingly.

## Ops — Applied on A (45.9.42.212)

- Reset Postgres password for role `ploy` and updated `/etc/ploy/ployd.yaml` with a literal DSN.
- Replaced server binary with freshly built `dist/ployd-linux` and restarted `ployd.service` (status: active).
