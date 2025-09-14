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

Maintenance Rules
- When a significant change to lane behavior, detection, or builders happens, update docs/LANES.md accordingly.
- This document, docs/LANES.md, and AGENTS.md are the source of truth for detection/build/deploy rules and operator guidance.

Measurement notes
- Image Size: use builder outputs (artifact size for unikernels/jails/VMs; manifest size for OCI) or registry/OS metrics where available.
- Build Time: record total wall-clock for build step (async status can include timestamps; CLI logs may also be parsed).
