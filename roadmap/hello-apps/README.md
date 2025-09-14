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
  - Key takeaways:
    - API defaults needed alignment with orchestration to avoid invalid HCL for user apps.
    - Add unit tests alongside behavior changes; document in CHANGELOG.

- Cycle 2 (Deploy + E2E, cap timeouts to 5 min):
  - Deployed API to Dev, but Dev pulls from its own git remote. Local fixes are not yet present on the VPS because they haven’t been pushed upstream.
  - Retried E2E:
    - Lane E still fails server‑side Nomad job validation at a lane‑E template block (volume/conditional markers), so deploy aborts.
    - Lane C deploys, but HTTPS health does not come up publicly. Logs via API show: "No running allocations found" (likely OSv path not suited for this Scala app, or not exposed publicly on Dev).
  - Script improvements: capped health wait TIMEOUT to 5 minutes by default and fixed a bash array edge case for extra flags.
  - Next: land the renderer/template fixes on the VPS (push main, then deploy) so Lane E (Jib container) validates and exposes `https://<app>.dev.ployd.app/healthz`. Until then, Lane C will not be a reliable public HTTPS path for this app.
  - Key takeaways:
    - Cap test timeouts and surface failures quickly; fetch logs when health fails.
    - Ensure fixes are pushed and deployed to VPS; Dev cluster pulls from its own repo.

- Cycle 3 (Template simplification + validation fixes):
  - Deployed API with nested-conditional handling, but Lane E still produced invalid HCL due to nested `{{#if}}` inside the Connect service block.
  - Simplified `platform/nomad/lane-e-oci-kontain.hcl` for dev: removed the Consul Connect service block to avoid nested-conditional artifacts; reduced Nomad task logs retention to 10×10MB and removed the duplicate `ready` service check.
  - Result: Lane E job validation advances; subsequent failure was an HTTP client EOF during `ploy push` to the controller.
  - E2E script: now detects non‑JSON CLI failures (❌) and aborts quickly; applies a single global TIMEOUT budget across push+health to keep cycles under 5 minutes.
  - Next: stabilize controller POST `/apps/:app/builds` for larger payloads (EOF); then retry Lane E. If needed, set `TIMEOUT=180` for faster cycles during iteration.
  - Key takeaways:
    - Simplify templates for dev where enterprise features aren’t enabled.
    - After HCL fixes, focus moved to upload path stability.

- Cycle 4 (Retry with 3min cap; inspect):
  - E2E retried with `TIMEOUT=180`, Lane E. CLI reports `unexpected EOF` from POST `/v1/apps/ploy-scala-hello/builds` consistently (twice, with backoff). App logs show no running allocations, as deploy aborts pre-Nomad.
  - Platform logs endpoint exists (`/v1/platform/:service/logs`) but is a stub; cannot fetch controller alloc logs via API. Next step is VPS-side alloc logs via job manager wrapper or implement the platform logs handler.
  - Hypothesis: reverse proxy (Traefik) or upstream idle timeout during streaming upload; consider increasing `forwardingTimeouts`/`readTimeout` for POSTs to `/v1/apps/*/builds`, or switching to chunked/multipart with smaller chunks.
  - Key takeaways:
    - Controller route might be fine, but ingress could be closing connections.
    - Add server-side logs and endpoints to improve visibility.

- Cycle 5 (Platform logs + streaming uploads):
  - Implemented `/v1/platform/:service/logs` (dev helper) — for `service=api` it fetches Nomad alloc logs via the job manager wrapper with `--task api`. Verified logs return HTTP 200 with controller entries.
  - Hardened build upload path to stream the request body to disk (`io.Copy` from request body stream) instead of `c.Body()` to reduce buffering and avoid proxy-induced EOFs.
  - Deployed API; `ploy push` still reports `unexpected EOF` to `/v1/apps/:app/builds`. Controller logs show no explicit errors (only leader elections). Next likely step: adjust Traefik/ingress timeouts for large POSTs or switch the client to multipart/chunked uploads with retries.
  - Key takeaways:
    - Streaming read/multipart support on server alone doesn’t resolve ingress drops.
    - Need to collect proxy logs or isolate path-based behavior.

- Cycle 6 (Alias route + OPTIONS probe):
  - Added OPTIONS handler for `/v1/apps/:app/builds` → returns `204` via HTTP/2 (route reachable through Traefik).
  - Implemented alias route `/v1/apps/:app/upload` mapped to the same build handler; updated E2E script to allow `USE_UPLOAD=1` for multipart.
  - Traefik logs via Nomad are unavailable (Traefik runs as systemd in this environment). Controller logs still healthy.
  - Key takeaways:
    - Builds work internally (mods build gate) — controller/Nomad OK.
    - External POST with body likely blocked by ingress path rules; probing `/upload` next.

- Cycle 7 (Probe alias /upload under HTTP/2):
  - Sent a tiny tar (4KB) over HTTP/2 to `/v1/apps/:app/upload` with a 20s cap → timed out with 0 bytes received (no controller logs). OPTIONS still 204 on `/builds`.
  - Key takeaways:
    - Behavior is consistent across `/builds` and `/upload`: POST bodies aren’t reaching the controller; likely an ingress/policy filter on POSTs to the API service, independent of path and size, and specific to body-bearing requests under HTTP/2.
    - Next: collect Traefik/system logs from the node (systemd journal or /var/log/traefik/traefik.log) to confirm proxy handling; or temporarily add a dev-only controller endpoint that accepts a small JSON POST on `/v1/apps/:app/builds` to test POST-without-binary (to isolate content-type handling).

- Cycle 8 (JSON probe succeeds; add Traefik log fetch script):
  - Added dev-only JSON probe `POST /v1/apps/:app/builds/probe` → HTTP/2 200 with echoed headers; proves POST + body reaches controller when Content-Type is JSON.
  - Probed `/upload` with `application/octet-stream` → still times out with 0 bytes received; confirms binary uploads are blocked upstream.
  - Added `tests/lanes/fetch-traefik-logs.sh` to fetch Traefik logs via SSH (`journalctl` and `/var/log/traefik/*.log`).
  - Key takeaways:
    - Ingress is likely filtering/closing binary-bodied POSTs to the controller under HTTP/2.
    - Next: run `TARGET_HOST=<ip> ./tests/lanes/fetch-traefik-logs.sh` and inspect access/error logs for POST `/v1/apps/<app>/(builds|upload)` to confirm.

- Cycle 9 (Deeper diagnosis via VPS; pinpoint ingress behavior):
  - From VPS, JSON probe POST `/v1/apps/:app/builds/probe` → HTTP/2 200 (controller sees body). From VPS, tiny binary body (4 bytes) with `Content-Length` and without `Expect` reaches controller (500 untar failed, expected); but a small tar (~4 KB) still fails (connection timeout/close).
  - Traefik access.log does not record the failing external POST attempts (neither /builds nor /upload), even with log level DEBUG; successful internal controller POSTs (mods) and GETs appear normally. This indicates the failing requests are terminated before reaching Traefik’s router/service.
  - Key takeaways:
    - Route and HTTP/2 are fine (OPTIONS 204, JSON POST 200). The problem is specific to binary-bodied POSTs (application/x-tar, application/octet-stream, multipart/form-data) over HTTP/2 being terminated upstream of the controller and before Traefik logging.
    - Disabling `Expect: 100-continue` and using a trivial body can reach the controller; realistic tar payloads (even ~4 KB) still fail—so it’s not just `Expect`, nor purely size.
    - Logging to add next: (1) temporary iptables/ufw LOG rule for 443 to detect drops; (2) tcpdump capture during a failing POST to inspect TLS record/flow; (3) in Traefik, add a dev-only buffering middleware for `api.dev.ployman.app` to eagerly read request bodies, which can mitigate HTTP/2 framing/flow-control issues.
  - Next: add dev-only Traefik buffering middleware to the API router/service and retest small tar POSTs; keep capture tools ready if needed.

- Cycle 4 (Dev-only buffering at ingress → binary POSTs reach controller):
  - Implemented a reversible Traefik dynamic-config patch to add a buffering middleware for `api.dev.ployman.app` and attached it via a high‑priority file‑provider router targeting `ploy-api@consulcatalog`.
    - Script: `scripts/dev/add-traefik-buffering-mw.sh` (add|remove). Appends a marked block to `/opt/ploy/traefik-data/dynamic-config.yml`, restarts Traefik if systemd, or relies on file watch when Nomad‑managed.
    - Probe: `scripts/dev/probe-api-binary-post.sh` sends tiny tar via `application/x-tar` and `multipart/form-data`, plus JSON probe.
  - Result after enabling buffering:
    - Binary POSTs over HTTP/2 now return API responses (no upstream drop). Example:
      - `POST /v1/apps/probe-hello/upload?...` (binary tar) → `HTTP 500` with body: `{"error":"job validation failed: ... nonexistent namespace \"debug\""}`
      - JSON probe `POST /v1/apps/probe-hello/builds/probe` → `HTTP 200`.
    - Prior behavior (pre-buffering): binary POSTs were terminated before reaching controller and absent from Traefik access logs.
  - Takeaways:
    - Traefik’s buffering middleware mitigates the HTTP/2 binary body termination; requests now land at the controller reliably.
    - Remaining 500s are controller‑side validation issues (e.g., debug namespace), unrelated to the original ingress/body transport problem.
  - Next:
    - Clean up the controller’s debug lane/namespace path for probe routes so small tar uploads validate in dev.
    - Decide whether to keep buffering permanently for `api.dev.ployman.app` (dev‑only) or gate by feature flag.

- Cycle 5 (Traefik as Nomad job; refine router rule):
  - Traefik now runs as a Nomad system job (`traefik-system`). File-provider `watch=true` reloads dynamic config; no systemd restart needed.
  - Updated helper to prefer Nomad detection and rely on file watch; refined the router rule to `Host("api.dev.ployman.app") && PathPrefix(`/v1`)` to ensure the buffering middleware applies to all versioned API routes.
  - Probes after refinement:
    - Binary POSTs to both `/v1/apps/:app/builds` and `/v1/apps/:app/upload` return fast controller responses (HTTP 500 expected for dummy tar: missing Dockerfile/Jib), and JSON probe returns 200.
    - Confirms ingress/body transport issues are addressed for HTTP/2 with Nomad-managed Traefik.
  - Key takeaways:
    - With Traefik under Nomad, dynamic config changes propagate automatically via file watch; target `/opt/ploy/traefik-data/dynamic-config.yml` which mounts to `/etc/traefik/dynamic-configs/dynamic-config.yml` in the job.
    - Router specificity matters: adding `PathPrefix(`/v1`)` ensured the dev-only router/middleware consistently handled API requests including the `/upload` alias.
  - Next:
    - Re-run Lane E E2E for `ploy-scala-hello` with multipart enabled (`PLOY_PUSH_MULTIPART=1`) to confirm end-to-end build, deploy, HTTPS health, destroy, status.
    - Keep the dev buffering middleware in place during validation; consider a toggle to enable/disable via a small CLI or API if needed.

- Cycle 6 (E2E attempt for Lane E with multipart → blocked by repo build config):
  - Ran E2E: `HELLO_APP_REPO=...ploy-scala-hello.git APP_NAME=ploy-scala-hello LANE=E USE_MULTIPART=1 HEALTH_PATH=/healthz ./tests/e2e/test-deploy-github-app.sh`.
  - Result: `HTTP 500` with `{"error":"oci build failed: ... No Dockerfile or Jib; cannot build OCI ..."}` on both attempts; no allocations/logs for the app as the build never completed.
  - Improvements (controller): mapped common builder error to a clearer client error (400) for missing Dockerfile/Jib prerequisites. Requires promoting API to VPS (ployman api deploy) to take effect.
  - Key takeaways:
    - The external repo currently lacks a Dockerfile or Jib configuration; OCI build cannot proceed. This is independent of the earlier ingress/body issue.
    - The dev-only buffering middleware remains necessary for reliable HTTP/2 binary requests but is now functioning as intended.
  - Next:
    - Update the `ploy-scala-hello` repo to include a minimal Dockerfile or migrate to Gradle with Jib plugin, then rerun the E2E.
    - Alternatively, enhance the controller’s OCI builder with a dev fallback (generate a simple Dockerfile for JVM apps) for demos, gated to dev only.
    - Deploy the updated API to VPS: `./bin/ployman api deploy --monitor` so the improved 400 error mapping is visible in Dev.

- Cycle 7 (Warm cache + dev serversTransport → build completes):
  - Added a multi-stage Dockerfile and .dockerignore to ploy-scala-hello and pushed to main.
  - On the VPS, pre-pulled base images: `gradle:8.8-jdk21`, `eclipse-temurin:21-jre`.
  - Extended Traefik dev dynamic config to include:
    - `serversTransports.dev-slow` with generous forwarding timeouts.
    - A dev-only file-provider service `dev-ploy-api-slow` (http://127.0.0.1:8081) using `serversTransport: dev-slow`.
    - Updated dev router to use `service: dev-ploy-api-slow` (still behind buffering middleware).
  - Result:
    - Multipart POST of the full repo tar to `/v1/apps/ploy-scala-hello/builds?lane=E&env=dev` returned `HTTP 200` with JSON: `status":"deployed"`, image tag `registry.dev.ployman.app/ploy-scala-hello:dev`, and push verification/digest OK.
  - Key takeaways:
    - With ingress buffering and extended servers transport timeouts, long builds can complete over HTTP/2 without framing/idle timeouts.
    - Warming base images significantly reduces first-build latency and avoids spurious timeouts.
  - Next:
    - Run full E2E (ploy push + HTTPS health + destroy + status) for `ploy-scala-hello` and capture timings.
    - If stable, consider codifying dev serversTransport block in managed config, or expose a dev toggle to enable it when needed.
