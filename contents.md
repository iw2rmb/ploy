[.github/](.github) GitHub automation workflows and CI configuration for build, test, coverage, and releases.
[.gitignore](.gitignore) Git ignore patterns for build outputs, caches, local tooling state, and generated artifacts.
[.golangci.yml](.golangci.yml) golangci-lint configuration defining enabled linters and issue handling policy.
[.goreleaser.yml](.goreleaser.yml) GoReleaser pipeline config for multi-platform binaries, packaging, checksums, signing, and distribution.
[.pre-commit-config.yaml](.pre-commit-config.yaml) Pre-commit hook configuration for repository hygiene checks.
[Makefile](Makefile) Canonical make targets for toolchain checks, build, test, lint, and release-oriented tasks.
[README.md](README.md) Project overview and operator/developer usage guidance for the Ploy stack.
[VERSION](VERSION) Single source of truth for the current release version.
[badges/](badges) Generated coverage badge assets consumed by repository documentation.
[cmd/](cmd) Entrypoints and command implementations for `ploy` CLI and `ployd`/`ployd-node` binaries.
[design/](design) Design documents describing proposed implementation approaches for planned work.
[docs/](docs) Current-state documentation for APIs, behavior, operations, schemas, and maintainer workflows.
[gates/](gates) Gate profile and stack definitions used by build/validation workflows.
[go.mod](go.mod) Go module manifest with required dependencies and toolchain version.
[go.sum](go.sum) Dependency checksum lockfile for reproducible Go module resolution.
[images/](images) Docker image build contexts and helper scripts for runtime, mig, ORW, and gate images.
[internal/](internal) Internal application packages implementing domain logic, APIs, storage, runtime, and orchestration flows.
[roadmap/](roadmap) Execution plans and implementation sequencing notes for upcoming tasks.
[sqlc.yaml](sqlc.yaml) sqlc generation config mapping PostgreSQL schema/queries to typed Go store code.
[staticcheck.conf](staticcheck.conf) Staticcheck ruleset configuration used by linting workflows.
[tests/](tests) Guard, integration, and end-to-end test suites with shared test harness scripts.
[tools/](tools) Small maintainer utilities and helper binaries used by development workflows.
