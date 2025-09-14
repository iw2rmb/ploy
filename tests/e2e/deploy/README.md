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
- export GITHUB_PLOY_DEV_USERNAME=<user>
- export GITHUB_PLOY_DEV_PAT=<pat>
- ./tests/e2e/deploy/generate-test-repos.sh

How to run E2E
- export PLOY_CONTROLLER=https://api.dev.ployman.app/v1
- go test ./tests/e2e/deploy -tags e2e -v -timeout 30m

Test matrix (seed)
| Lane | Stack  | Version | Repo                                | Current State |
| ---- | ------ | ------- | ----------------------------------- | ------------- |
| C    | Scala  | 21      | https://github.com/<u>/ploy-lane-c-scala-21 | pending       |
| C    | Java   | 8       | https://github.com/<u>/ploy-lane-c-java-8   | pending       |
| E    | Node   | 20      | https://github.com/<u>/ploy-lane-e-node-20  | pending       |
| E    | Go     | 1.22    | https://github.com/<u>/ploy-lane-e-go-1.22  | pending       |
| E    | Python | 3.12    | https://github.com/<u>/ploy-lane-e-python-3.12 | pending    |
| E    | .NET   | 8.0     | https://github.com/<u>/ploy-lane-e-dotnet-8 | pending      |
| G    | Rust   | 1.79    | https://github.com/<u>/ploy-lane-g-rust-1.79 | pending     |

Notes
- Start with major current versions; expand matrix incrementally.
- Detection and image building must be precise:
  - Java/Scala: detect major version (e.g., 8/11/17/21) from Gradle/Maven and use it in templates/builders.
  - Node: detect engines.node and select appropriate base in autogen when applicable.
  - Go/.NET/Python/Rust: detect project version/SDK and propagate to builders as needed.
- OSv (Lane C) requires compatible base images per Java version; configure mappings on Dev and track status here.
- Keep this table updated after each cycle.

