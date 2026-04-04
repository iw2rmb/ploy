[.github/](.github) GitHub repository settings and Actions workflows for CI, coverage, release, and smoke checks.
[.gitignore](.gitignore) Ignore rules for build outputs, caches, local tool state, logs, and generated artifacts.
[.golangci.yml](.golangci.yml) Minimal golangci-lint configuration that runs `govet` with strict issue reporting.
[.goreleaser.yml](.goreleaser.yml) GoReleaser configuration for building, packaging, signing, and publishing ploy binaries.
[.pre-commit-config.yaml](.pre-commit-config.yaml) Local pre-commit hook configuration with the repository noop gate.
[Makefile](Makefile) Primary build/test/lint/release task runner with toolchain and version validation targets.
[README.md](README.md) Project overview, installation steps, and CLI-oriented quick-start documentation for Ploy.
[VERSION](VERSION) Canonical semantic version value used for build stamping and releases.
[badges/](badges) Generated SVG coverage badges used in repository documentation surfaces.
[cmd/](cmd) Go command entrypoints and assets for `ploy`, `ployd`, and `ployd-node` binaries.
[design/](design) Design specifications describing intended implementations for planned platform features.
[docs/](docs) Current-state documentation for APIs, lifecycle behavior, schemas, and operational procedures.
[gates/](gates) Gate stack and language/toolchain profile definitions for build-gate execution.
[go.mod](go.mod) Go module manifest declaring the pinned toolchain and direct project dependencies.
[go.sum](go.sum) Checksum lockfile for reproducible Go module dependency resolution.
[images/](images) Container image build contexts and scripts for server, node, mig, and gate runtimes.
[internal/](internal) Internal packages implementing CLI, control plane, node runtime, storage, workflow, and TUI logic.
[roadmap/](roadmap) Implementation roadmaps that sequence planned work items and rollout steps.
[sqlc.yaml](sqlc.yaml) sqlc generation mapping from PostgreSQL schema/queries to strongly typed Go store code.
[staticcheck.conf](staticcheck.conf) Staticcheck ruleset enabling all checks with selected style/noise exceptions.
[tests/](tests) Guard, integration, e2e, and unit test suites plus smoke-test helpers.
[tools/](tools) Maintainer helper programs for completion generation and API token utility tasks.
