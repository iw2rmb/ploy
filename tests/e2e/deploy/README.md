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
- Run: `go test ./tests/e2e/deploy -tags e2e -v -timeout 10m` (assumes `PLOY_CONTROLLER` is set)
- Results are appended to:
  - JSONL: tests/e2e/deploy/results.jsonl (one JSON per run)
  - Markdown rows: tests/e2e/deploy/results.md (includes image size and build time)

Quick filters
- Single subtest: `go test ./tests/e2e/deploy -tags e2e -v -run 'TestDeployMatrix/E-node-20' -timeout 6m`
- Explicit case (Java 17, no Jib): `go test ./tests/e2e/deploy -tags e2e -v -run TestDeploy_Java17_NoJib -timeout 10m`

After each run: Inspect logs first
- Preferred: `APP_NAME=<name> [LANE=<A|C|E>] [SHA=<sha12>] [BUILD_ID=<id>] [LINES=200] [TARGET_HOST=<ip>] [OUT_DIR=./e2e-logs] ./tests/e2e/deploy/fetch-logs.sh`
  - Pulls app status/logs, Platform API and Traefik logs.
  - If `BUILD_ID` is set (from async response), includes builder logs via API.
  - If `TARGET_HOST` is set, fetches builder logs and app alloc status/logs via Nomad job-manager wrapper.
- Direct endpoints (fallback):
  - Controller logs: `curl -sS "$PLOY_CONTROLLER/platform/api/logs?lines=200"`
  - Traefik logs: `curl -sS "$PLOY_CONTROLLER/platform/traefik/logs?lines=200"`

Test matrix (seed)
| Lane | Stack   | Version | Repo                                         | Image Size (compressed) | Uncompressed Size | Build Time | Builder CPU | Builder Memory | Current State |
| ---- | ------- | ------- | -------------------------------------------- | ------------------------ | ----------------- | ---------- | ----------- | -------------- | ------------- |
| A    | Go      | 1.22    | https://github.com/<u>/ploy-lane-a-go-1.22   | —                        | —                 | —          | —           | —              | pending       |
| B    | Node    | 20      | https://github.com/<u>/ploy-lane-b-node-20   | —                        | —                 | —          | —           | —              | pending       |
| B    | Python  | 3.12    | https://github.com/<u>/ploy-lane-b-python-3.12 | —                      | —                 | —          | —           | —              | pending       |
| C    | Scala   | 21      | https://github.com/<u>/ploy-lane-c-scala-21  | —                        | —                 | —          | —           | —              | pending       |
| C    | Java    | 8       | https://github.com/<u>/ploy-lane-c-java-8    | —                        | —                 | —          | —           | —              | pending       |
| D    | Python  | 3.12    | https://github.com/<u>/ploy-lane-d-python-3.12 | —                      | —                 | —          | —           | —              | pending       |
| D    | Node    | 20      | https://github.com/<u>/ploy-lane-d-node-20   | —                        | —                 | —          | —           | —              | pending       |
| E    | Node    | 20      | https://github.com/<u>/ploy-lane-e-node-20   | 52.3MB                   | 128.2MB           | 23.5s      | 500         | 512MB          | passed        |
| E    | Go      | 1.22    | https://github.com/<u>/ploy-lane-e-go-1.22   | 4.9MB                    | 8.7MB             | —          | 500         | 512MB          | passed        |
| E    | Python  | 3.12    | https://github.com/<u>/ploy-lane-e-python-3.12 | 47.3MB                  | 113.7MB           | 22.2s      | 500         | 512MB          | passed        |
| E    | .NET    | 8.0     | https://github.com/<u>/ploy-lane-e-dotnet-8  | 97.9MB                   | 207.6MB           | 125.0s     | 500         | 2048MB         | passed        |
| G    | Rust    | 1.79    | https://github.com/<u>/ploy-lane-g-rust-1.79 | —                        | —                 | —          | —           | —              | pending       |

Notes
- Start with major current versions; expand matrix incrementally.
- Detection and image building must be precise:
  - Java/Scala: detect major version (e.g., 8/11/17/21) from Gradle/Maven and use it in templates/builders.
  - Node: detect engines.node and select appropriate base in autogen when applicable.
  - Go/.NET/Python/Rust: detect project version/SDK and propagate to builders as needed.
- OSv (Lane C) requires compatible base images per Java version; configure mappings on Dev and track status here.
- Keep this table updated after each cycle.

Cycle State
- Current key takeaways only; update each cycle and keep concise. For historical notes, use CHANGELOG.md.

 - App-name normalization: tests sanitize names to `[a-z0-9-]`, replacing dots in versions (e.g., `1.22` → `1-22`).
 - Lane E status: Node 20, Go 1.22, and Python 3.12 pass with event-driven health; .NET 8 pending due to Kaniko OOM during snapshot (exit code 137).
 - Targeted fix: bump Kaniko builder memory only for .NET builds (default 2048MB; others remain at 512MB). Env override: `PLOY_KANIKO_MEMORY_DOTNET_MB`.
 - Image size: captured from registry manifest (compressed) and Docker inspect (uncompressed) and recorded in results files.
 - Logs: use `fetch-logs.sh` with `BUILD_ID` and `TARGET_HOST` to retrieve builder and app alloc status/logs before deeper triage.
 - Cleanup: apps are destroyed after each cycle; if automated destroy fails, destroy manually: `ploy apps destroy --name <app> --force`.

Maintenance Rules
- When a significant change to lane behavior, detection, or builders happens, update docs/LANES.md accordingly.
- This document, docs/LANES.md, and AGENTS.md are the source of truth for detection/build/deploy rules and operator guidance.

Measurement notes
- Image Size: use builder outputs (artifact size for unikernels/jails/VMs; manifest size for OCI) or registry/OS metrics where available.
- Build Time: record total wall-clock for build step (async status can include timestamps; CLI logs may also be parsed).
