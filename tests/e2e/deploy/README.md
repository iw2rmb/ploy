# Consolidated E2E: Language → Lane → Stack Detection and Deployment

Purpose: exercise the full detection/build/deploy pipeline across all supported languages and major versions using a single, unified harness. This replaces prior lane‑specific app tests.

Core flow (always reused):
- Language detection: read repo markers (build files) to infer language and version (Java/Scala, Node, Python, Go, .NET, Rust).
- Lane detection: run lane picker and apply explicit overrides for targeted runs.
- Stack detection/build: choose the appropriate builder (e.g., Jib vs Kaniko; OSv pack vs full compose) and build the right image/artifact.
- Deploy + health: submit Nomad jobs and verify HTTPS /healthz.

Test repos
- Generated and pushed to GitHub using GITHUB_PLOY_DEV_USERNAME and GITHUB_PLOY_DEV_PAT.
- Naming: `ploy-lane-<x>-<lang>-<ver>` (e.g., `ploy-lane-c-scala-21`, `ploy-lane-e-node-20`).
- Script: `tests/e2e/deploy/generate-test-repos.sh` creates repos, scaffolds minimal apps and pushes them.

How to generate repos
- ./tests/e2e/deploy/generate-test-repos.sh

How to run E2E
- Ensure the ploy CLI is available. Either:
  - Build locally: `mkdir -p bin && GOCACHE=$(mktemp -d) go build -o ./bin/ploy ./cmd/ploy`
  - Or set `PLOY_CMD=/absolute/path/to/ploy`
- Run: `go test ./tests/e2e/deploy -tags e2e -v -timeout 10m`
- Results are appended to:
  - JSONL: tests/e2e/deploy/results.jsonl (one JSON per run)
  - Markdown rows: tests/e2e/deploy/results.md (includes image size and build time)

Quick filters
- Single subtest: `go test ./tests/e2e/deploy -tags e2e -v -run 'TestDeployMatrix/E-node-20' -timeout 6m`
- Explicit case (Java 17, no Jib): `go test ./tests/e2e/deploy -tags e2e -v -run TestDeploy_Java17_NoJib -timeout 10m`

Log collection tips
- Controller logs: `curl -sS "$PLOY_CONTROLLER/platform/api/logs?lines=200"`
- Traefik logs: `curl -sS "$PLOY_CONTROLLER/platform/traefik/logs?lines=200"`
- Aggregated app/platform/builder logs: `APP_NAME=<name> [LANE=<A|C|E>] [SHA=<sha12>] [LINES=200] [TARGET_HOST=<ip>] ./tests/e2e/deploy/fetch-logs.sh`

Test matrix (seed)
| Lane | Stack   | Version | Repo                                         | Image Size | Build Time | Current State |
| ---- | ------- | ------- | -------------------------------------------- | ---------- | ---------- | ------------- |
| A    | Go      | 1.22    | https://github.com/<u>/ploy-lane-a-go-1.22   | —          | —          | pending       |
| B    | Node    | 20      | https://github.com/<u>/ploy-lane-b-node-20   | —          | —          | pending       |
| B    | Python  | 3.12    | https://github.com/<u>/ploy-lane-b-python-3.12 | —        | —          | pending       |
| C    | Scala   | 21      | https://github.com/<u>/ploy-lane-c-scala-21  | —          | —          | pending       |
| C    | Java    | 8       | https://github.com/<u>/ploy-lane-c-java-8    | —          | —          | pending       |
| D    | Python  | 3.12    | https://github.com/<u>/ploy-lane-d-python-3.12 | —        | —          | pending       |
| D    | Node    | 20      | https://github.com/<u>/ploy-lane-d-node-20   | —          | —          | pending       |
| E    | Node    | 20      | https://github.com/<u>/ploy-lane-e-node-20   | —          | —          | pending       |
| E    | Go      | 1.22    | https://github.com/<u>/ploy-lane-e-go-1.22   | —          | —          | pending       |
| E    | Python  | 3.12    | https://github.com/<u>/ploy-lane-e-python-3.12 | —        | —          | pending       |
| E    | .NET    | 8.0     | https://github.com/<u>/ploy-lane-e-dotnet-8  | —          | —          | pending       |
| G    | Rust    | 1.79    | https://github.com/<u>/ploy-lane-g-rust-1.79 | —          | —          | pending       |

Notes
- Start with major current versions; expand matrix incrementally.
- Detection and image building must be precise:
  - Java/Scala: detect major version (e.g., 8/11/17/21) from Gradle/Maven and use it in templates/builders.
  - Node: detect engines.node and select appropriate base in autogen when applicable.
  - Go/.NET/Python/Rust: detect project version/SDK and propagate to builders as needed.
- OSv (Lane C) requires compatible base images per Java version; configure mappings on Dev and track status here.
- Keep this table updated after each cycle.

Cycle Key Takeaways
- After every cycle, add a short note here capturing what changed, what failed/succeeded, and any infra or config adjustments performed. Ensure docs/LANES.md is updated if behavior/features changed.

- Cycle: Added JVM-aware Dockerfile autogen (Gradle/Maven → eclipse-temurin:<ver>-jre) and extended Lane E autogen to Python (python:<ver>-slim with gunicorn/uvicorn when present) and .NET (sdk:<ver> → aspnet:<ver>). Seeded a Java no-Jib sample repo: ploy-lane-e-java-17-nojib.

- Cycle: Investigated Java 17 no-Jib E2E health failure. Findings and changes:
  - App status endpoint could misreport lane by picking the first existing job; updated to prefer Lane E on Dev to avoid confusion while builds run.
  - fetch-logs.sh SSH path updated to resolve builder alloc ID (running-alloc) before calling logs; removed unsupported "jobs" command.
  - Async build status remained "running"; likely Kaniko build still in progress or waiting on registry. Next: surface builder job/logs via API for better triage and verify PLOY_KANIKO_IMAGE is mirrored on Dev.

- Cycle: TLS verification failure for api.dev.ployman.app (core cause + fix plan).
  - Symptom: `curl` failed with `SSL certificate problem: unable to get local issuer certificate` when hitting `https://api.dev.ployman.app`.
  - Root cause: Traefik served its self-signed default certificate. Logs showed `Unable to parse certificate /data/certificates/dev-ployman-app.crt (no PEM data)`. Dynamic config referenced non-existent/invalid static certs and did not reliably trigger ACME issuance for platform wildcard. In some runs, Traefik also reported `Router uses a nonexistent certificate resolver` for `apps-wildcard`, indicating resolver flags weren’t in effect or config was inconsistent.
  - Changes: Updated `iac/common/templates/traefik-dynamic-config.yml.j2` to rely solely on ACME DNS-01 via resolvers and removed static TLS certificate references. Added two routers to pre-provision wildcards via DNS challenge:
    - `apps-wildcard-cert` → `HostRegexp({app}.{{ ploy.apps_domain }})` with certResolver `apps-wildcard` and domains `[main: {{ ploy.apps_domain }}, sans: *.{{ ploy.apps_domain }}]`.
    - `platform-wildcard-cert` → `HostRegexp({service}.{{ ploy.platform_domain }})` with certResolver `platform-wildcard` and domains `[main: {{ ploy.platform_domain }}, sans: *.{{ ploy.platform_domain }}]`.
  - Deploy path (per AGENTS.md):
    - Ensure NAMECHEAP_* env vars are available on the VPS (hashicorp.yml imports legacy values if present).
    - Commit/push, then run `./bin/ployman api deploy --monitor` and apply IaC via `iac/dev/site.yml` so Traefik picks up the new dynamic config at `/opt/ploy/traefik-data/dynamic-config.yml`.
    - Verify: `openssl s_client -connect api.dev.ployman.app:443 -servername api.dev.ployman.app` should show a valid ACME-issued cert (not "TRAEFIK DEFAULT CERT"). `curl -I https://api.dev.ployman.app/health` should succeed without `-k`.
  - Temporary test workaround: where the harness polls the Dev API, use TLS-insecure only as a stopgap (`PLOY_TLS_INSECURE=1` or `curl -k`) until the above deploy completes.

- Cycle: TLS certs validated on Dev; resume E-only matrix.
  - Result: `openssl s_client -showcerts -connect api.dev.ployman.app:443 -servername api.dev.ployman.app` now presents a valid chain; curl works without `-k`.
  - Action: remove any temporary TLS-insecure flags from local runs; ensure `PLOY_CONTROLLER=https://api.dev.ployman.app/v1` is used for E2E.
  - Next:
    - Re-run Lane E builds (Node 20, Go 1.22, Python 3.12, .NET 8) and verify Kaniko context fetch is stable post-DNS + sync PUT changes.
    - If a build stalls/fails, fetch builder logs via API: `GET /v1/apps/:app/builds/:id/logs` and platform logs: `/platform/api/logs`, `/platform/traefik/logs`.
    - Append results to `tests/e2e/deploy/results.jsonl` and `tests/e2e/deploy/results.md` with image size and build time.

- Cycle: E-only matrix run (post-TLS) — builder allocations failing.
  - Command: `PLOY_CMD=$(pwd)/ploy PLOY_CONTROLLER=https://api.dev.ployman.app/v1 go test ./tests/e2e/deploy -tags e2e -v -timeout 15m`.
  - Node 20 (Lane E): push accepted `id=b-1757907098417613225`; build status → `failed` with `kaniko builder failed for job ploy-lane-e-node-20-e-build-07f329c1caeb: job ... allocation failed (7181e96c-...)`. Health at `https://ploy-lane-e-node-20.dev.ployd.app/healthz` failed.
  - Go 1.22, Python 3.12 (Lane E): similar health failures; builder job listing empty or failed. Traefik platform logs endpoint returned "No running allocations found" for traefik, though API logs were accessible.
  - Builder logs API: returned noisy `alloc_id` due to wrapper stderr mixing with stdout; log fetch attempts used malformed path when alloc ID resolution failed.
  - Hypotheses to validate on VPS:
    - Kaniko image mirror not configured or missing: ensure `PLOY_KANIKO_IMAGE=registry.dev.ployman.app/kaniko-executor:debug` in `/home/ploy/api.env` and image present (roll via `iac/dev/playbooks/roll-api.yml`).
    - DNS inside builder task: builder uses `network_mode=host` and fetches context from `seaweedfs-filer.service.consul:8888`; confirm `dns-consul.yml` applied and host resolves `*.service.consul` via 127.0.0.1:8600.
    - Nomad allocation events: fetch via `/opt/hashicorp/bin/nomad-job-manager.sh alloc-status <alloc-id>` to see exact failure (image pull, permission, or runtime error).
  - Near-term improvements (implemented):
    - Builder logs API now extracts the allocation ID via UUID regex from wrapper output and returns `alloc_status` alongside recent logs.
    - Lane E Kaniko image selection prefers the internal mirror by default when targeting Dev (`registry.dev.ployman.app/kaniko-executor:debug`), unless `PLOY_KANIKO_IMAGE` is explicitly set.
  - Next actions:
    - Verify `PLOY_KANIKO_IMAGE` on Dev and re-roll API env if needed; rerun Node 20 case and capture alloc events/logs; if DNS is the culprit, confirm dnsmasq and resolv.conf on the node and container.

Maintenance Rules
- When a significant change to lane behavior, detection, or builders happens, update docs/LANES.md accordingly.
- This document, docs/LANES.md, and AGENTS.md are the source of truth for detection/build/deploy rules and operator guidance.

Measurement notes
- Image Size: use builder outputs (artifact size for unikernels/jails/VMs; manifest size for OCI) or registry/OS metrics where available.
- Build Time: record total wall-clock for build step (async status can include timestamps; CLI logs may also be parsed).
