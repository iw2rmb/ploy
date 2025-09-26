# Changelog

## [2025-09-26] Stage Artifact Streams
- Added `contracts.WorkflowArtifact` and JetStream/in-memory events client support so workflow stage artifacts mirror onto `ploy.artifact.<ticket>` alongside checkpoints.
- Updated the workflow runner to emit artifact envelopes for completed stages, propagate publication failures, and surface envelopes in unit tests for cache hydrator consumers.
- Documented the slice in `docs/design/stage-artifacts/README.md`, marked the checkpoint metadata follow-up complete, and recorded roadmap entry `18-stage-artifact-streams` as shipped.

## [2025-09-26] Workflow Checkpoint Metadata
- Bumped the workflow event schema to `2025-09-26.1` and enriched checkpoints with `stage_metadata` and `artifacts` blocks so Grid consumers can inspect lane assignments, dependencies, and produced manifests directly from JetStream.
- Updated the workflow runner to attach stage metadata for every status transition and to include artifact manifests returned from Grid stage outcomes.
- Extended the Grid Workflow client and contract tests to round-trip artifact payloads, refreshed `docs/design/event-contracts/README.md`, and marked roadmap slice `17-checkpoint-metadata` complete.

## [2025-09-26] Snapshot Metadata Streams
- Added `internal/workflow/snapshots.NewJetStreamMetadataPublisher` to emit schema-versioned snapshot metadata envelopes to `ploy.artifact.<ticket>` when ``JETSTREAM_URL`` is configured, retaining the in-memory stub for offline runs.
- Updated the CLI snapshot registry loader to wire the JetStream metadata publisher automatically and extended `ploy snapshot capture` tests to verify live JetStream behaviour alongside the existing IPFS gateway coverage.
- Refreshed documentation (`docs/SNAPSHOTS.md`, `docs/design/ipfs-artifacts/README.md`, `docs/design/overview/README.md`) and recorded roadmap slice `16-snapshot-metadata-streams` as complete with CHANGELOG entry dated 2025-09-26.

## [2025-09-26] IPFS Artifact Publishing
- Added `internal/workflow/snapshots.NewIPFSGatewayPublisher` to stream snapshot payloads to IPFS gateways via `/api/v0/add`, returning the gateway-provided CID while keeping the in-memory stub fallback for offline runs.
- Updated `ploy snapshot capture` to honour ``IPFS_GATEWAY`` during registry loading, surfacing the returned CID in CLI output and metadata structures.
- Expanded snapshot and CLI test suites with gateway-backed scenarios; refreshed documentation (`docs/design/ipfs-artifacts/README.md`, `docs/SNAPSHOTS.md`, `cmd/ploy/README.md`, `README.md`) and recorded roadmap slice `15-ipfs-artifact-publishing` as complete.

## [2025-09-26] Grid Workflow Client
- Added `internal/workflow/grid` with an HTTP Workflow RPC client that submits stage executions to Grid and records invocation metadata for CLI summaries.
- Updated `ploy workflow run` to honour ``GRID_ENDPOINT``, wiring real Grid dispatch when configured and keeping the in-memory stub for offline development.
- Expanded CLI and client test suites to cover Grid configuration failures, request encoding, and response handling; recorded the roadmap slice as complete.

## [2025-09-26] Integration Manifest Schema
- Published `docs/schemas/integration_manifest.schema.json` capturing required manifest fields and constraints for topology, fixtures, lanes, and Aster toggles.
- Added `ploy manifest schema` to surface the schema for downstream tooling and validation flows.
- Updated documentation (`docs/MANIFESTS.md`, `docs/design/overview/README.md`, `docs/DOCS.md`) and recorded roadmap slice `13-integration-manifest-schema` as complete.

## [2025-09-26] Snapshot Catalog Validation
- Added MySQL (`mysql-orders`) and document-store (`doc-events`) fixtures alongside the existing Postgres snapshots so `ploy snapshot plan|capture` exercises all representative engines locally.
- Implemented the `last4` masking strategy and wired regression coverage that loads the in-repo catalog and executes captures with stub publishers.
- Updated snapshot documentation and the SHIFT roadmap to record the validation slice ahead of JetStream/Grid wiring.

## [2025-09-26] JetStream Client Wiring
- Introduced `internal/workflow/contracts.JetStreamClient` to consume real tickets from `grid.webhook.<tenant>` and publish checkpoints to `ploy.workflow.<ticket>.checkpoints`.
- Updated `ploy workflow run` to honour ``JETSTREAM_URL`` by dialing JetStream (falling back to the in-memory stub when unset) and surfacing connection failures to the caller.
- Added unit tests that exercise the client against an in-process JetStream server plus CLI coverage for misconfiguration errors.
- Refreshed documentation and roadmap entries to describe the live JetStream behaviour and new configuration toggle.

## [2025-09-26] Workflow Checkpoint Cache Keys
- Extended the workflow event contract to include lane cache keys on every checkpoint and bumped the schema version to `2025-09-26`.
- Updated the workflow runner to compute cache keys via injected composers, ensuring JetStream checkpoints surface cache-coordination signals.
- Wired the CLI to derive cache keys from lane specs so Grid integrations can rely on consistent cache metadata ahead of JetStream wiring.

## [2025-09-26] Mods Terminology Guard
- Replaced remaining ARF references with mods terminology across roadmap and recipe documentation.
- Added `terminology_guard_test.go` to enforce the naming convention and block regressions.
- Expanded `docs/RECIPES.md` with a detailed explanation of `configs/recipes/kotlin-gradle.toml`.

## [2025-09-26] Recipe Pack Registry
- Added `internal/recipes/packs` with a TOML loader that exposes pluggable recipe pack lists and language-aware lookups.
- Published default specs (`configs/recipes/java-default.toml`, `configs/recipes/kotlin-gradle.toml`) to seed Java and Kotlin/Gradle catalog coverage.
- Documented the registry in `docs/RECIPES.md`, updated the README, and marked the roadmap item complete.

## [2025-09-26] Documentation Cleanup
- Refreshed `README.md` to highlight the CLI-first/Grid model, enumerate all completed SHIFT slices, and link directly to the design doc.
- Updated the documentation matrix (`docs/DOCS.md`, `docs/LANES.md`, `docs/MANIFESTS.md`, `docs/SNAPSHOTS.md`) to emphasise JetStream/Grid workflows and point contributors at the relevant guides.
- Added `documentation_cleanup_test.go` to guard the roadmap status and README alignment for this slice.


## [2025-09-26] Aster Hook Integration
- Added `internal/workflow/aster` with a filesystem-backed locator that discovers per-stage bundle metadata from `configs/aster/` and surfaces provenance data for Grid submissions.
- Extended the workflow runner to require an Aster locator, attach sorted toggle metadata to every stage, and honour per-stage disablement while keeping cache keys deterministic.
- Introduced `--aster` and `--aster-step` flags on `ploy workflow run`, along with post-run bundle summaries so operators can verify toggles before Grid wiring lands.
- Expanded CLI and runner test suites to cover bundle detection, metadata propagation, per-stage overrides, and regression behaviour when Aster is disabled.
- Documented the workflow in `cmd/ploy/README.md`, `docs/MANIFESTS.md`, and `docs/design/overview/README.md`; roadmap slice `07-aster-hook` marked complete.

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
- Documented the subject map and example payloads in `docs/design/event-contracts/README.md`; roadmap slice `01-event-contracts` now marked complete.

## [2025-09-25] Legacy Teardown
- Removed all legacy API, Nomad, Consul, SeaweedFS, and deployment scaffolding.
- Replaced the repo with a CLI-only stub (`ploy workflow run`) that validates ticket input and returns `ErrNotImplemented`.
- Added guardrail tests that fail if legacy binaries or imports reappear.
- Simplified the build system (`Makefile`) to focus on the workflow CLI.
- Rewrote documentation (`README.md`, `docs/DOCS.md`, `cmd/ploy/README.md`) to describe the Shift architecture and roadmap alignment.

## [History]
Prior releases documented Nomad-based services, security engines, and lane orchestration. Refer to the Git history before `2025-09-25` for archival details.
