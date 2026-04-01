[.github/](.github) GitHub automation workflows and CI configuration for building, testing, and publishing.
[.gitignore](.gitignore) Git ignore rules for build outputs, local state, and generated artifacts.
[.golangci.yml](.golangci.yml) Repository golangci-lint policy defining enabled linters, exclusions, and import-boundary checks.
[.goreleaser.yml](.goreleaser.yml) GoReleaser release configuration for multi-platform binaries, checksums, SBOMs, signing, and Homebrew tap publishing.
[.pre-commit-config.yaml](.pre-commit-config.yaml) Pre-commit hook set for file hygiene, Go formatting, markdown linting, and manual static analysis.
[Makefile](Makefile) Canonical project tasks for build, test, lint, and release-oriented workflows.
[README.md](README.md) Primary project overview with architecture, setup, and usage guidance.
[VERSION](VERSION) Current project version identifier used by build and release workflows.
[badges/](badges) Generated badge assets used in repository documentation.
[cmd/](cmd) CLI entrypoints and command bootstrapping for ploy binaries.
[deploy/](deploy) Deployment scripts, image build contexts, and environment packaging assets.
[design/](design) Design documents describing planned implementations and technical approaches.
[docs/](docs) Current-state documentation for behavior, operations, and interfaces.
[gates/](gates) Guardrail profile definitions for language and toolchain policy checks.
[go.mod](go.mod) Go module definition with dependency and toolchain requirements.
[go.sum](go.sum) Dependency checksum lockfile for Go modules.
[internal/](internal) Core application packages implementing workflows, APIs, and runtime logic.
[roadmap/](roadmap) Decomposed implementation plans and execution sequencing notes.
[sqlc.yaml](sqlc.yaml) sqlc code generation configuration for typed database access layers.
[staticcheck.conf](staticcheck.conf) Staticcheck ruleset configuration enforced in repository linting.
[tests/](tests) Integration and scenario-level test suites and supporting fixtures.
[tools/](tools) Small helper binaries and utilities used by maintainers and automation.
