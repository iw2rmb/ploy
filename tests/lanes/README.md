# Lanes E2E Test Plan (A–G)

Purpose: Validate end-to-end deployment for all lanes (A–G) using minimal, reproducible GitHub hello apps and the `ploy` CLI against the Dev API.

This plan mirrors roadmap/hello-apps and extends it per lane with concrete repos, envs, and steps. Follow AGENTS.md (TDD cycle + VPS rules) at all times.

## Scope
- Lanes: A (Unikraft Minimal), B (Unikraft POSIX), C (OSv Java/Scala), D (FreeBSD Jails), E (OCI+Kontain), F (Full VMs), G (WASM Runtime)
- Environments: Workstation invoking Dev API (PLOY_CONTROLLER). No local Nomad/Consul.
- Artifacts: HTTPS health, destroy cleanup, and basic log retrieval per app.

## Hello App Repositories
Created under the ploy dev GitHub account using `scripts/lanes/create-lane-repos.sh`:
- Lane A: `ploy-lane-a-go` (Go)
- Lane B: `ploy-lane-b-node` (Node.js)
- Lane C: `ploy-lane-c-java` (Java)
- Lane D: `ploy-lane-d-python` (Python)
- Lane E: `ploy-lane-e-go` (Go, OCI)
- Lane F: `ploy-lane-f-dotnet` (.NET)
- Lane G: `ploy-lane-g-rust` (Rust WASM)

Notes
- Initial creation is empty/minimal. Each repo must expose an HTTP 200 health endpoint at `/healthz` and be lane-appropriate.
- For existing examples, migrate from `tests/apps/*` to these repos, similar to `ploy-scala-hello` in roadmap/hello-apps.

## Environment Variables
- Required (Dev API): `PLOY_CONTROLLER=https://api.dev.ployman.app/v1`
- Lane repos (set per available hello app):
  - `LANE_A_REPO`, `LANE_B_REPO`, …, `LANE_G_REPO` (e.g., `https://github.com/<user>/ploy-lane-a-go.git`)
- Optional:
  - `TARGET_HOST` (VPS SSH for log tailing via job manager)
  - `APP_NAME` override, `BRANCH` (default `main`), `LANE` (force detection), `HEALTH_PATH` (default `/healthz`)

## Test Matrix & Success Criteria
- Per-lane test steps:
  1) Clone repo (shallow) on a temp workdir
  2) `ploy push -a <app>` (optionally `-lane <A-G>`)
  3) Wait HTTPS: `https://<sha>.<app>.dev.ployd.app<HEALTH_PATH>` within 180s
  4) Destroy: `ploy apps destroy --name <app> --force`
  5) Verify 404: `GET /v1/apps/<app>/status` → 404
  6) Retrieve logs: `GET /v1/apps/<app>/logs` or VPS job-manager logs

- A: Go unikernel boots, tiny image; health returns 200
- B: Node.js POSIX unikernel via musl, health 200
- C: Java/Scala on OSv (Jib→Capstan), health 200
- D: FreeBSD jail tar rootfs, health 200
- E: OCI image (Kontain runtime), health 200
- F: VM image (qcow2/VM), health 200
- G: WASM (wasi, Rust or Go), health 200

## How To Run
- Scripted E2E per lane:
  - `LANE=A LANE_A_REPO=... PLOY_CONTROLLER=... ./tests/lanes/test-lane-deploy.sh`
- All configured lanes via Go E2E (build tag `e2e`):
  - `PLOY_CONTROLLER=... go test ./tests/e2e -tags e2e -v -run TestLaneDeployments`
- Tail logs (API or VPS):
  - `APP_NAME=<app> PLOY_CONTROLLER=... ./tests/lanes/check-app-logs.sh`

## Repo Creation
Use the helper script (requires `GITHUB_PLOY_DEV_PAT`, `GITHUB_PLOY_DEV_USERNAME`):
- `./scripts/lanes/create-lane-repos.sh`
The script creates the 7 repos if missing and sets descriptions.

## TDD Workflow
- RED: enable at least one lane’s env var (e.g., `LANE_C_REPO`) and run E2E → expect failures until hello app content exists.
- GREEN: add minimal hello app to the corresponding repo, re-run until health succeeds and cleanup verified.
- REFACTOR (VPS): harden with log checks, lane detection assertions, and timing budgets.

## Operational Tips
- Prefer `APP_NAME` equal to repo name (`basename -s .git` logic in scripts).
- If preview URL `<sha>.<app>.dev.ployd.app` is unavailable, set `URL_OVERRIDE` to `https://<app>.dev.ployd.app`.
- For lane forcing during bring-up, export `LANE=<A-G>`.
- Log retrieval via API is non-streaming; for deep debugging on VPS use `TARGET_HOST` + job-manager wrapper.

## Cleanup & Idempotency
- Scripts destroy the app post validation and tolerate 404 on status checks.
- Avoid reusing app names across concurrent runs; env `APP_NAME` can include a suffix.

