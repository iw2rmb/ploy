[.github/](.github) GitHub automation configuration for CI, coverage, build, and smoke-test workflows.
[badges/](badges) Generated status badge assets used in project documentation.
[cmd/](cmd) CLI and daemon entrypoints plus command wiring for ploy binaries.
[deploy/](deploy) Local and image build/deploy scripts, Dockerfiles, and environment-specific deployment assets.
[design/](design) Design documents describing implementation approaches for major features.
[docs/](docs) Current-state operational, API, environment, and maintainer documentation.
[gates/](gates) Build-gate profile definitions for supported language/toolchain combinations.
[healing/](healing) Healing action prompts and specs for code, dependency, and infra repair flows.
[internal/](internal) Core application packages for server, node agent, workflow engine, storage, and shared utilities.
[lib/](lib) Reference migration packs and supporting assets used by migration workflows.
[roadmap/](roadmap) Execution roadmaps and decomposed implementation plans.
[scripts/](scripts) Maintainer utility scripts used by local checks and CI guardrails.
[tests/](tests) Integration and end-to-end test suites plus scenario helpers.
[tools/](tools) Small Go helper binaries for generation and developer tooling tasks.
[.gitignore](.gitignore) Git ignore rules for generated artifacts, local state, and tool outputs.
[AGENTS.md](AGENTS.md) Repository-specific agent workflow instructions and validation expectations.
[CHANGELOG.md](CHANGELOG.md) Chronological record of shipped changes and their verification commands.
[Makefile](Makefile) Canonical build, test, lint, coverage, and maintenance targets for the project.
[README.md](README.md) Project overview, architecture, setup, usage, and contributor entrypoint.
[go.mod](go.mod) Go module definition with dependency requirements and toolchain constraints.
[go.sum](go.sum) Cryptographic checksums for Go module dependency integrity.
[sqlc.yaml](sqlc.yaml) sqlc code-generation configuration for typed database query bindings.
[staticcheck.conf](staticcheck.conf) Staticcheck ruleset configuration for repository linting policy.
