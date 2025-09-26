# Changelog

## [2025-09-26] Documentation Cleanup
- Refreshed `README.md` to highlight the CLI-first/Grid model, enumerate all completed SHIFT slices, and link directly to the design doc.
- Updated the documentation matrix (`docs/DOCS.md`, `docs/LANES.md`, `docs/MANIFESTS.md`, `docs/SNAPSHOTS.md`) to emphasise JetStream/Grid workflows and point contributors at the relevant guides.
- Added `documentation_cleanup_test.go` to guard the roadmap status and README alignment for this slice.


## [2025-09-26] Aster Hook Integration
- Added `internal/workflow/aster` with a filesystem-backed locator that discovers per-stage bundle metadata from `configs/aster/` and surfaces provenance data for Grid submissions.
- Extended the workflow runner to require an Aster locator, attach sorted toggle metadata to every stage, and honour per-stage disablement while keeping cache keys deterministic.
- Introduced `--aster` and `--aster-step` flags on `ploy workflow run`, along with post-run bundle summaries so operators can verify toggles before Grid wiring lands.
- Expanded CLI and runner test suites to cover bundle detection, metadata propagation, per-stage overrides, and regression behaviour when Aster is disabled.
- Documented the workflow in `cmd/ploy/README.md`, `docs/MANIFESTS.md`, and `docs/design/shift/README.md`; roadmap slice `07-aster-hook` marked complete.

## [2025-09-26] Commit-Scoped Environments
- Added `internal/workflow/environments` service with TDD coverage for dry-run planning, execution hydration, and snapshot gap reporting.
- Introduced `ploy environment materialize` CLI command with dry-run/execute modes, manifest override support, and human-readable summaries.
- Published new snapshot specs (`commit-db`, `commit-cache`) and GPU lane profile (`configs/lanes/gpu-ml.toml`) to back commit-scoped runs.
- Documented the workflow in `README.md` and `cmd/ploy/README.md`; roadmap slice `06-commit-environments` marked complete.

## [2025-09-26] Integration Manifest Compiler
- Introduced `internal/workflow/manifests` with TOML schema validation, JSON compilation helpers, and unit tests covering happy/failure paths.
- Extended the workflow runner to require manifest compilation, attach compiled payloads to every stage, and let the in-memory Grid stub enforce lane allowlists.
- Updated `ploy workflow run` to load manifests from `configs/manifests/`, surface actionable validation errors, and documented the schema in `docs/MANIFESTS.md` alongside new sample manifests (`smoke`, `commit-app`).
- Added CLI tests asserting manifest loader wiring and error propagation; roadmap slice `05-integration-manifests` is now complete.

## [2025-09-26] Snapshot Toolkit CLI
- Added `internal/workflow/snapshots` with TOML spec loader, rule engine (strip/mask/synthetic), deterministic fingerprinting, and metadata publishing hooks backed by in-memory IPFS/JetStream stubs.
- Introduced `ploy snapshot plan` and `ploy snapshot capture` commands, plus CLI tests covering usage, summary output, and capture reporting.
- Published default snapshot spec/fixture under `configs/snapshots/` to exercise the toolkit locally.
- Documented snapshot workflow in `README.md` and `cmd/ploy/README.md`; roadmap slice `04-snapshot-toolkit` marked complete with container replay hook deferred to the JetStream integration slice.

## [2025-09-26] Lane Engine & Describe CLI
- Added `internal/workflow/lanes` with TOML loader, cache-key composer, validation, and unit tests covering required fields and deterministic outputs.
- Introduced `configs/lanes/node-wasm.toml` and `configs/lanes/go-native.toml` as the first Grid-ready lane profiles.
- Extended `cmd/ploy` with `lanes describe`, golden-style CLI tests, and richer top-level usage guidance.
- Propagated lane metadata through the workflow runner and in-memory Grid stub; stages now error when lane assignments are missing.
- Documented the lane system in `README.md`, `cmd/ploy/README.md`, `docs/LANES.md`, and marked roadmap slice `03-lane-engine` complete.

## [2025-09-26] Workflow Runner CLI Stub
- Expanded `internal/workflow/runner` with a default DAG planner, stage execution loop, retry handling, temporary workspace management, and error propagation for Grid interactions.
- Added an in-memory Grid client, stage invocation tracking, and extensive unit tests lifting runner package coverage to 94.5%.
- Updated `cmd/ploy` to support `--ticket auto`, inject JetStream/Grid stubs via testable factories, and emit usage/help output across new error paths.
- Extended CLI tests to cover command dispatch, usage printers, and runner wiring; repository-wide `go test -cover ./...` now satisfies ≥60% overall coverage.
- Documented environment placeholders (`JETSTREAM_URL`, `GRID_ENDPOINT`, `IPFS_GATEWAY`) and new behaviour in `cmd/ploy/README.md`; marked roadmap slice `02-workflow-runner-cli` complete.

## [2025-09-25] Event Contract Stub
- Added `internal/workflow/contracts` with schema version `2025-09-25`, subject helpers, and validation logic for workflow tickets and checkpoints.
- Wired `internal/workflow/runner` to claim tickets, validate payloads, and publish an initial `claimed` checkpoint through a JetStream stub.
- Updated the CLI to require `--tenant`, bootstrap the in-memory bus, and reflect the new behaviour in usage docs.
- Documented the subject map and example payloads in `docs/design/shift/event-contracts.md`; roadmap slice `01-event-contracts` now marked complete.

## [2025-09-25] Legacy Teardown
- Removed all legacy API, Nomad, Consul, SeaweedFS, and deployment scaffolding.
- Replaced the repo with a CLI-only stub (`ploy workflow run`) that validates ticket input and returns `ErrNotImplemented`.
- Added guardrail tests that fail if legacy binaries or imports reappear.
- Simplified the build system (`Makefile`) to focus on the workflow CLI.
- Rewrote documentation (`README.md`, `docs/DOCS.md`, `cmd/ploy/README.md`) to describe the Shift architecture and roadmap alignment.

## [History]
Prior releases documented Nomad-based services, security engines, and lane orchestration. Refer to the Git history before `2025-09-25` for archival details.
