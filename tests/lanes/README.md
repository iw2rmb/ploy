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
- Focus: validate ploy’s deploy path against the Dev API (not lane specifics).
- Test steps:
  1) Clone repo (shallow) on a temp workdir
  2) `ploy push -a <app>` (do not specify `-lane` by default)
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
- Stack readiness (Go E2E):
  - `PLOY_CONTROLLER=... go test ./tests/e2e -tags e2e -v -run TestStackReadiness`
- All configured repos via Go E2E (build tag `e2e`):
  - `PLOY_CONTROLLER=... go test ./tests/e2e -tags e2e -v -run TestLaneDeployments`
- Tail logs (API or VPS):
  - `APP_NAME=<app> PLOY_CONTROLLER=... ./tests/lanes/check-app-logs.sh`

Tip (GREEN baseline): For initial pass across all repos, force container lane:
- `LANE_OVERRIDE=E PLOY_CONTROLLER=... go test ./tests/e2e -tags e2e -v -run TestLaneDeployments`
  - Each repo ships a Dockerfile and /healthz endpoint; this ensures green while lane-specific scaffolding matures.

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

## Cycle Key Takeaways
- Async deploy path implemented for reliability:
  - `POST /v1/apps/:app/builds?async=true` returns 202 with `{accepted,id,status}`.
  - Background worker posts tar to local API and updates `/v1/apps/:app/builds/:id/status` (running/completed/failed) with raw build message for triage.
- Lane E (OCI) dev template fixes:
  - Removed Docker `healthcheck` (unsupported on this Nomad/Docker driver); added service-level health checks.
  - Corrected Traefik rule to `Host(<app>.<domain>)`; HTTPS fallback tries both preview and app host.
- E2E harness hardening:
  - TIMEOUT covers push + health wait; early failure detection + single retry; logs on failure.
  - Async-aware: polls status ID (up to half of TIMEOUT) before HTTPS checks.
- Current blocker for Lane E sample:
  - Nomad fails to pull image: `manifest unknown` for `registry.dev.ployman.app/ploy-lane-a-go:<sha>`.
  - Likely missing push and/or pull credentials for internal registry; image not present for Nomad to pull.
- Next steps:
  - Ensure API build host pushes to `registry.dev.ployman.app` and Nomad node can pull (credentials/whitelist).
  - Add push verification to build script and surface results in async status for faster triage.
  - Re-run Lane E to green, then proceed to lane-specific tests (A–G) per plan.

### Cycle: Push Verification + Async Status Enhancements
- Build pipeline:
  - Added push verification to `scripts/build/oci/build_oci.sh` using `docker manifest inspect` (non-fatal).
    - Emits a readable line: `Push verification: OK (digest sha256:...)` or a clear FAILED/SKIPPED note.
  - Server adds a registry check after builds for container lanes and includes it in response JSON:
    - New field `pushVerification`: `{ok,status,digest,message}`.
    - Async status now carries this JSON message, making missing-tag/auth issues obvious during polling.
- Registry access (Dev VPS):
  - Docker Registry v2 at `registry.dev.ployman.app` is deployed without auth in Dev (see `iac/dev/playbooks/docker-registry.yml`). Push/pull do not require username/password.
  - Ensure TLS trust and DNS resolution to `registry.dev.ployman.app` from the VPS. No docker login needed in Dev.
- Test harness:
  - Lane E E2E will now surface `pushVerification` in the status poll results before HTTPS checks, reducing time-to-diagnose `manifest unknown`.
  - Continue using async deploy mode to avoid ingress timeouts during tar upload.

### Cycle: Traefik as Nomad Job
- Ingress topology:
  - Traefik now runs as a Nomad job; discovery uses Consul Catalog tags.
  - Lane E template already sets Consul service tags for Traefik: `traefik.enable=true`, router rule `Host(<app>.<domain>)`, TLS certresolver `dev-wildcard`, and service healthcheck path `/healthz`.
- Diagnostics:
  - Fetch Traefik logs via Dev API: `curl -sS "$PLOY_CONTROLLER/platform/traefik/logs?lines=200"`.
  - Our `tests/lanes/check-app-logs.sh` now also prints Traefik logs after app logs when `PLOY_CONTROLLER` is set.
  - Verify router creation and 404s in Traefik logs if HTTPS fails while allocations are healthy.
- Next validation pass:
  - Ensure image exists in internal registry so Nomad allocates the task; once healthy, Traefik should route per tags above.

Temporary Dev workaround (no in-API Docker):
- The API runs as a Nomad job without Docker, so Lane E image builds must occur via a builder job (future) or manual push.
- Manual push via SSH (Dev only):
  - ssh root@$TARGET_HOST
  - su - ploy
  - mkdir -p ~/tmp/ploy-lane-e-go && cd ~/tmp/ploy-lane-e-go
  - Create Dockerfile and app (or git clone your lane repo), then:
    docker build -t registry.dev.ployman.app/<app>:<sha> .
    docker push registry.dev.ployman.app/<app>:<sha>
  - Verify: curl -I https://registry.dev.ployman.app/v2/<app>/manifests/<sha>
  - Re-run the Lane E test to confirm /healthz is green.
