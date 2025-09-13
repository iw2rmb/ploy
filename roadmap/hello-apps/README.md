# Hello Apps Migration Plan

Goal: Move example apps from `tests/apps/` to dedicated GitHub repositories and exercise them via E2E using `ploy` CLI against the Dev API.

Phases

1) E2E Test (ploy → HTTPS → destroy)
- Add a CI/VPS-friendly E2E script that:
  - Clones a GitHub repo (HELLO_APP_REPO, BRANCH)
  - Runs `ploy push -a <app> -env <env>`
  - Waits for HTTPS health (default: `/<health>` on `https://<app>.<env>.ployd.app`)
  - Destroys the app via `ploy apps destroy --name <app> --force`
  - Verifies `/v1/apps/<app>/status` → HTTP 404
- Implemented: `tests/e2e/test-deploy-github-app.sh`
  - Env vars: `HELLO_APP_REPO`, `APP_NAME` (optional), `BRANCH` (default: main), `ENV_NAME` (default: dev), `PLOY_CONTROLLER` (Dev API)

2) Migrate first app (Scala)
- Source: `tests/apps/scala-catalogsvc`
- Target GitHub repo: `https://github.com/iw2rmb/ploy-scala-hello`
- Actions:
  - Publish repo with Jib-enabled Scala sample (hello endpoint)
  - Remove `tests/apps/scala-catalogsvc` from repo (done)
  - Run E2E:
    ```bash
    HELLO_APP_REPO=https://github.com/iw2rmb/ploy-scala-hello.git \
    APP_NAME=ploy-scala-hello \
    PLOY_CONTROLLER=https://api.dev.ployman.app/v1 \
    ./tests/e2e/test-deploy-github-app.sh
    ```

3) Update lane detection templates
- Parameterize Scala test to use external repo if available:
  - `iac/common/templates/test-lane-picker-enhanced.sh.j2` now uses `SCALA_HELLO_REPO` when set.
  - `iac/common/templates/test-lane-detection.sh.j2` clones `SCALA_HELLO_REPO` if provided; otherwise skips Scala.

4) Future migrations (create repos, remove local examples)
- Node: `ploy-node-hello` (from `tests/apps/node-hello`)
- Go: `ploy-go-hello` (from `tests/apps/go-hellosvc`)
- Python: `ploy-python-hello` (from `tests/apps/python-apisvc`)
- Java: `ploy-java-hello` (from `tests/apps/java-ordersvc`)
- .NET: `ploy-dotnet-hello` (from `tests/apps/dotnet-ordersvc`)
- Rust: `ploy-rust-hello` (from `tests/apps/rust-hellosvc`)
- WASM samples: `ploy-wasm-*` (from `tests/apps/wasm-*`)

5) CI integration (optional)
- Add a self-hosted runner job to run `test-deploy-github-app.sh` for selected hello apps against the Dev API.
- Gate on `HELLO_APP_REPO` secrets/inputs per job.

Notes
- Ensure hello apps expose a simple `/health` HTTP 200 endpoint.
- Keep repos minimal with clear README and build files.
- Use Jib for JVM hello apps to validate Lane E.
- For Scala test in templates, set `SCALA_HELLO_REPO=https://github.com/iw2rmb/ploy-scala-hello.git`.

## Cycle Log

- Cycle 1 (Lane E → fix defaults):
  - Attempted E2E deploy of `ploy-scala-hello` with `LANE=E` via `tests/e2e/test-deploy-github-app.sh`.
  - Result: Nomad job validation failed due to volume/consul blocks present for a user app (dev cluster). Root cause: API renderer defaulted `VolumeEnabled`/`ConsulConfigEnabled` to true for non‑platform apps.
  - Action: Updated API renderer defaults to match orchestration — disable Volumes and Consul Config by default for non‑platform apps (Vault/Connect already off). Added unit tests and CHANGELOG note.
  - Next: Deploy API (`./bin/ployman api deploy --monitor`) and rerun E2E with `LANE=E` and `URL_OVERRIDE=https://ploy-scala-hello.dev.ployd.app/healthz` (Lane E host rule includes `dev.`).

- Cycle 2 (Deploy + E2E, cap timeouts to 5 min):
  - Deployed API to Dev, but Dev pulls from its own git remote. Local fixes are not yet present on the VPS because they haven’t been pushed upstream.
  - Retried E2E:
    - Lane E still fails server‑side Nomad job validation at a lane‑E template block (volume/conditional markers), so deploy aborts.
    - Lane C deploys, but HTTPS health does not come up publicly. Logs via API show: "No running allocations found" (likely OSv path not suited for this Scala app, or not exposed publicly on Dev).
  - Script improvements: capped health wait TIMEOUT to 5 minutes by default and fixed a bash array edge case for extra flags.
  - Next: land the renderer/template fixes on the VPS (push main, then deploy) so Lane E (Jib container) validates and exposes `https://<app>.dev.ployd.app/healthz`. Until then, Lane C will not be a reliable public HTTPS path for this app.

- Cycle 3 (Template simplification + validation fixes):
  - Deployed API with nested-conditional handling, but Lane E still produced invalid HCL due to nested `{{#if}}` inside the Connect service block.
  - Simplified `platform/nomad/lane-e-oci-kontain.hcl` for dev: removed the Consul Connect service block to avoid nested-conditional artifacts; reduced Nomad task logs retention to 10×10MB and removed the duplicate `ready` service check.
  - Result: Lane E job validation advances; subsequent failure was an HTTP client EOF during `ploy push` to the controller.
  - E2E script: now detects non‑JSON CLI failures (❌) and aborts quickly; applies a single global TIMEOUT budget across push+health to keep cycles under 5 minutes.
  - Next: stabilize controller POST `/apps/:app/builds` for larger payloads (EOF); then retry Lane E. If needed, set `TIMEOUT=180` for faster cycles during iteration.

- Cycle 4 (Retry with 3min cap; inspect):
  - E2E retried with `TIMEOUT=180`, Lane E. CLI reports `unexpected EOF` from POST `/v1/apps/ploy-scala-hello/builds` consistently (twice, with backoff). App logs show no running allocations, as deploy aborts pre-Nomad.
  - Platform logs endpoint exists (`/v1/platform/:service/logs`) but is a stub; cannot fetch controller alloc logs via API. Next step is VPS-side alloc logs via job manager wrapper or implement the platform logs handler.
  - Hypothesis: reverse proxy (Traefik) or upstream idle timeout during streaming upload; consider increasing `forwardingTimeouts`/`readTimeout` for POSTs to `/v1/apps/*/builds`, or switching to chunked/multipart with smaller chunks.

- Cycle 5 (Platform logs + streaming uploads):
  - Implemented `/v1/platform/:service/logs` (dev helper) — for `service=api` it fetches Nomad alloc logs via the job manager wrapper with `--task api`. Verified logs return HTTP 200 with controller entries.
  - Hardened build upload path to stream the request body to disk (`io.Copy` from request body stream) instead of `c.Body()` to reduce buffering and avoid proxy-induced EOFs.
  - Deployed API; `ploy push` still reports `unexpected EOF` to `/v1/apps/:app/builds`. Controller logs show no explicit errors (only leader elections). Next likely step: adjust Traefik/ingress timeouts for large POSTs or switch the client to multipart/chunked uploads with retries.
