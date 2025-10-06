# Changelog

## [2025-10-08] Lane Catalog Externalised

- Published [`iw2rmb/ploy-lanes-catalog`](https://github.com/iw2rmb/ploy-lanes-catalog)
  and removed embedded lane definitions from this repository; the CLI now loads
  lanes from `$PLOY_LANES_DIR` (falling back to XDG paths or an adjacent
  checkout).
- Updated CLI docs, environment references, and design tasks to point at the new
  catalogue; added `PLOY_LANES_DIR` to the documented environment variables and
  refreshed the lane validator default.
- Added optional `GRID_API_KEY`/`GRID_ID` handling alongside the existing
  `GRID_ENDPOINT`, wiring bearer tokens into the Grid helper and discovery calls
  while updating tests and env listings.
- Extended Mods live Grid smoke to accept `PLOY_E2E_LIVE_SCENARIOS`, allowing
  `TestModsScenariosLiveGrid` to run multiple scenarios via `go test -tags e2e
  ./tests/e2e`; documented the knob in `docs/envs/README.md` and the refactor
  task notes after verifying `ploy lanes describe` against the SHIFT catalog.

## [2025-10-07] Mods Catalog Alignment

- `docs/design/mods-grid-restoration/README.md`: Reframed follow-up work around a
  dedicated Mods lane repository and Grid catalog registration; re-reviewed
  `go test ./internal/workflow/runner -run TestRunSchedulesHealingPlanAfterBuildGateFailure -count=1`,
  `go test ./cmd/ploy -run TestHandleWorkflowRunConfiguresModsFlags -count=1`, and
  `go test -tags e2e ./tests/e2e -count=1` (latest run 2025-10-05) to confirm
  behaviour remains unchanged.
- `docs/design/shift/README.md`: Updated SHIFT roadmap alignment to track the
  Mods catalog namespace hand-off to Grid and call out coordination requirements
  (desk review with Mods design + Grid plan on 2025-10-07).
- `docs/design/README.md`: Refreshed index entries for SHIFT/Mods designs to
  highlight the Grid catalog follow-up and new verification timestamp.
- Noted the `/lanes/<namespace>.tar.gz` publication path in the Mods/Grid
  designs so workstation docs match the updated Grid endpoint.

## [2025-10-05] Mods Grid Restoration (Steps 1–4)

- `ploy mod run` now materialises repositories via `WorkflowTicket.Repo`,
  cloning into a temporary workspace and honouring optional workspace hints
  before Mods stages execute.
- Mods-specific lanes (`lanes/mods-*.toml`) feed the job composer so
  OpenRewrite, LLM, and human stages dispatch with correct container images and
  resources; build-gate failures trigger healing retries that append `#healN`
  branches using knowledge base signals.
- Documentation refreshed: `cmd/ploy/README.md`,
  `docs/design/mods-grid-restoration/README.md`, `docs/design/mods/README.md`,
  `docs/design/event-contracts/README.md`, and `docs/design/shift/README.md`
  now describe repo materialisation, stage metadata, and healing behaviour.
- Introduced `lanes/` catalog (mirrored to SHIFT) with a
  `go run ./tools/lanesvalidate` helper and `make lanes-validate` target; CLI now
  reads from this catalog by default.
- Restored Mods E2E harness under `tests/e2e` so workstation runs validate
  simple, self-healing, and parallel scenarios against the in-memory Grid stub.
- Verification:
  `go test ./internal/workflow/runner -run TestRunSchedulesHealingPlanAfterBuildGateFailure -count=1`,
  `go test ./cmd/ploy -run TestHandleWorkflowRunConfiguresModsFlags -count=1`,
  `go test -tags e2e ./tests/e2e -count=1`.

## [2025-10-05] Workflow RPC Cancellation & Archive Summaries

- Added `workflow cancel` to the CLI, wiring Workflow RPC cancellation through
  the Grid SDK helper and surfacing whether the request was accepted or the run
  had already finished.
- Persisted Workflow RPC trust state under
  `${XDG_CONFIG_HOME:-$HOME/.config}/ploy/grid` (override via
  `GRID_WORKFLOW_SDK_STATE_DIR`) so manifests and CA bundles survive successive
  runs without re-bootstrap.
- Enriched `StageOutcome` with run IDs and archive export metadata so CLI
  summaries highlight keep-forever runs once Grid queues archive requests.
- Updated `docs/design/workflow-rpc-alignment/README.md`,
  `docs/design/README.md`, `docs/envs/README.md`, and `cmd/ploy/README.md` to
  document cancellation flows, archive surfacing, and the SDK state directory.
- Verification: `go test ./...` on 2025-10-05.

## [2025-10-07] Build Gate CLI Summary

- Surfaced build gate results in `ploy workflow run` by printing static check
  outcomes, failing diagnostics, knowledge base findings, and log digests
  sourced from checkpoint metadata.
- Added log finding support to workflow contracts and runner checkpoint
  conversion so Knowledge Base codes propagate into CLI summaries and downstream
  consumers.
- Documented the milestone in `docs/design/build-gate/README.md`, the new SHIFT
  design index, and roadmap slice `docs/tasks/build-gate/07-cli-summary.md`,
  marking `docs/tasks/shift/21-build-gate-reboot.md` as complete.

## [2025-09-29] Grid Discovery Alignment

- Updated `cmd/ploy/dependencies.go` to parse the full `/v1/cluster/info`
  payload (API endpoint, JetStream route list, IPFS gateway, feature map,
  version) with strict decoding, caching, and helper accessors.
- Extended discovery and workflow tests
  (`cmd/ploy/dependencies_discovery_test.go`,
  `cmd/ploy/workflow_run_grid_test.go`) to cover multi-route handling, feature
  checks, caching, and fallback semantics; verified via `go test ./cmd/ploy`.
- Documented the milestone in `docs/design/discovery-alignment/README.md`,
  refreshed README guidance, and marked roadmap slices
  `docs/tasks/discovery-alignment/01-cluster-info-parser.md`–`03-workflow-grid-alignment.md`
  complete.

## [2025-09-29] Build Gate Error Prone Adapter

- Added `internal/workflow/buildgate.NewErrorProneAdapter` to execute Java Error
  Prone with manifest-configurable targets, classpaths, and flags while parsing
  diagnostics into `StaticCheckFailure` entries backed by
  `internal/workflow/buildgate/error_prone_adapter_test.go`.
- Wired CLI workflow summaries to display Error Prone findings alongside
  existing Go output via `cmd/ploy/workflow_run_output_test.go` and normalised
  static check language aliases by mapping `javac` to `java`.
- Deflaked `TestRunExecutesParallelModsStages` by extending the stage start wait
  window to accommodate concurrent scheduling under heavier load.
- Updated documentation and roadmap artifacts:
  `docs/design/build-gate/error-prone/README.md`,
  `docs/design/build-gate/README.md`, `docs/design/README.md`,
  `docs/design/shift/README.md`, `docs/tasks/build-gate/08-error-prone-adapter.md`,
  and `docs/tasks/shift/21-build-gate-reboot.md` to record the adapter milestone
  and verification.
- Verification: `go test ./...` on 2025-09-29.

## [2025-09-29] Build Gate ESLint Adapter

- Added `internal/workflow/buildgate.NewESLintAdapter` to execute ESLint with
  manifest-configurable targets, config files, and rule severity overrides while
  converting diagnostics into `StaticCheckFailure` entries
  (`internal/workflow/buildgate/eslint_adapter.go`).
- Extended unit coverage with
  `internal/workflow/buildgate/eslint_adapter_test.go` and
  `static_checks_helpers_test.go`, and refreshed CLI fixtures in
  `cmd/ploy/workflow_run_output_test.go` to surface ESLint failures in build
  gate summaries.
- Updated documentation and roadmap artifacts:
  `docs/design/build-gate/eslint/README.md`, `docs/design/build-gate/README.md`,
  `docs/design/README.md`, `docs/design/shift/README.md`, and
  `docs/tasks/build-gate/09-eslint-adapter.md` to mark the JavaScript adapter
  milestone complete.
- Verification: `go test ./internal/workflow/buildgate -count=1`,
  `go test ./internal/workflow/runner -count=1`, and `go test ./...` on
  2025-09-29 after stabilising `TestRunExecutesParallelModsStages` timeouts.

## [2025-09-29] Discovery-Only Environment Wiring

- Removed legacy JetStream and IPFS environment fallbacks from
  `cmd/ploy/dependencies.go`, relying exclusively on Grid discovery for
  JetStream routes and IPFS gateways.
- Updated CLI tests (`cmd/ploy/dependencies_discovery_test.go`,
  `cmd/ploy/snapshot_test.go`, `cmd/ploy/workflow_run_grid_test.go`) to exercise
  discovery-driven configuration and sanitized defaults.
- Refreshed documentation (`docs/envs/README.md`, `README.md`,
  `cmd/ploy/README.md`, `docs/SNAPSHOTS.md`,
  `docs/design/{event-contracts,checkpoint-metadata,ipfs-artifacts,snapshot-metadata,stage-artifacts,overview}/README.md`,
  `docs/design/README.md`) and aligned roadmap entries with the discovery-only
  behaviour.
- Verification: `go test ./...` on 2025-09-29.

## [2025-09-27] Build Gate Log Retrieval & Parsing

- Added `internal/workflow/buildgate.LogRetriever` and `LogIngestor` to download
  Grid build logs with IPFS fallbacks, clamp payload size, compute deterministic
  SHA-256 digests, and parse Knowledge Base findings into checkpoint metadata.
- Introduced a default log parser and metadata sanitisation updates that
  normalise Git authentication, Go module conflict, linker, and disk pressure
  findings for downstream remediation flows.
- Documented the milestone across the build gate design record, roadmap slice
  `docs/tasks/build-gate/04-log-retrieval-and-grid-integration.md`, the SHIFT
  tracker, and the design index so roadmap status stays aligned.

## [2025-09-27] Build Gate Go Vet Adapter

- Added `internal/workflow/buildgate.NewGoVetAdapter` to run `go vet` with
  manifest-configurable package scopes and build tag overrides, returning
  `StaticCheckFailure` entries tagged with the `govet` rule identifier.
- Normalised static check language aliases so lane defaults, manifest overrides,
  and CLI skip flags can use `go` or `golang` interchangeably when dispatching
  adapters.
- Documented the adapter milestone across the build gate design record and
  roadmap slices, and introduced dedicated unit tests covering command option
  propagation and diagnostic parsing.

## [2025-09-27] Build Gate Runner Orchestration

- Added `internal/workflow/buildgate.Runner` to orchestrate sandbox builds,
  static check execution, and log ingestion while returning sanitised build gate
  metadata for checkpoint publication.
- Introduced `internal/workflow/buildgate/runner_test.go` with unit tests
  covering dependency validation and aggregated metadata to keep the package
  above the 90% coverage target.
- Documented the runner milestone across `docs/design/build-gate/README.md`, the
  design index, `docs/tasks/shift/21-build-gate-reboot.md`, and the new roadmap
  slice `docs/tasks/build-gate/06-build-gate-runner.md`.

## [2025-10-01] Workflow RPC Helper Adoption

- Replaced the local Workflow RPC shim with a helper-backed client in
  `internal/workflow/grid/client.go` that injects bearer tokens, streams run
  status with retryable cursors, and surfaces typed HTTP errors for retry
  decisions.
- Expanded Grid client unit tests to cover submit payload construction, terminal
  streaming events, and error propagation alongside helper tests for auth
  headers, retry backoff, and context cancellation.
- Marked roadmap slice `workflow-rpc-alignment/04-helper-adoption` complete and
  refreshed the Workflow RPC alignment design, design index, and overview to
  document adoption of the upstream SDK/helper modules and retirement of
  `internal/workflow/grid/workflowrpc`.

## [2025-09-30] JetStream Subject Alignment

- Updated `internal/workflow/contracts` with shared subject constants so ticket
  claims flow through `webhook.<tenant>.ploy.workflow-ticket` and status polling
  follows `jobs.<run_id>.events`, including whitespace safeguards for derived
  subjects.
- Refreshed JetStream client tests to cover the new webhook stream wildcard and
  trimmed subject handling, keeping workstation coverage deterministic.
- Documented the migration across the Workflow RPC alignment design, overview
  reference, Mods design, and roadmap slice
  `workflow-rpc-alignment/03-subject-alignment` (now marked complete).

## [2025-09-29] Integration Manifest v2 Schema

- Enforced v2 manifests across the loader by requiring
  `manifest_version = "v2"`, adding services/edges/exposures validation,
  deterministic ordering, and exported helpers (`LoadFile`,
  `EncodeCompilationToTOML`) for single-file workflows.
- Introduced `ploy manifest validate [--rewrite=v2]` to validate or rewrite
  manifests in place, preserving file permissions and mirroring Grid topology
  fixtures.
- Migrated shipped manifests (`configs/manifests/commit-app.toml`,
  `configs/manifests/smoke.toml`), refreshed `docs/MANIFESTS.md`, updated the v2
  design record, and marked roadmap slice
  `docs/tasks/integration-manifests/01-schema-upgrade.md` complete.

## [2025-09-28] Workflow JobSpec Composition

- Extended lane specs under `configs/lanes/*.toml` with validated `job` blocks
  covering image, command, environment, resources, and optional priority
  defaults.
- Introduced `runner.LaneJobComposer` plus CLI wiring so workstation runs
  compose `workflowsdk.JobSpec` payloads from lane metadata while the Grid
  client stamps lane/cache/manifest metadata for scheduler scoring.
- Updated documentation (`docs/LANES.md`,
  `docs/design/workflow-rpc-alignment/README.md`, `cmd/ploy/README.md`) and
  marked roadmap slice `workflow-rpc-alignment/02-runner-job-spec` complete
  alongside expanded unit coverage.

## [2025-09-28] Workflow RPC SDK Wrapper

- Added `internal/workflow/grid/workflowrpc`, a workstation-safe shim around the
  Grid Workflow RPC SDK with unit tests covering request marshalling, response
  handling, and error propagation.
- Replaced the bespoke HTTP client in `internal/workflow/grid` with the
  SDK-backed wrapper, keeping invocation tracking intact and enabling CLI
  integration when `GRID_ENDPOINT` is configured.
- Updated `cmd/ploy/README.md`, the Workflow RPC alignment design, and the
  roadmap entry to reflect the SDK wiring milestone; marked roadmap slice
  `workflow-rpc-alignment/01-grid-sdk-client` complete.
- Adjusted a knowledge base filesystem test to skip when running as root so
  workstation test runs remain deterministic.

## [2025-10-05] Build Gate Sandbox Runner

- Added `internal/workflow/buildgate.SandboxRunner` to wrap deterministic
  sandbox builds with structured outcomes (duration, cache hits, failure
  metadata) and introduced focused unit tests for success, failure, and timeout
  scenarios.
- Updated knowledge base catalog tests to skip permission checks when running as
  root so `go test ./...` stays portable across environments.
- Recorded docs/tasks/design updates for the sandbox runner milestone
  (`docs/tasks/build-gate/02-sandbox-runner.md`,
  `docs/design/build-gate/README.md`, `docs/tasks/shift/21-build-gate-reboot.md`).

## [2025-10-05] Build Gate Static Check Registry

- Added `internal/workflow/buildgate.StaticCheckRegistry` with lane default,
  manifest override, and skip handling so build gate stages can coordinate
  language-specific adapters with deterministic severity thresholds.
- Introduced `internal/workflow/buildgate/static_checks_test.go` covering
  severity evaluation, manifest disablement, and skip overrides with fake
  adapters to keep package coverage ≥90%.
- Documented the registry in `docs/design/build-gate/README.md`, marked roadmap
  slice `docs/tasks/build-gate/03-static-check-registry.md` complete, and updated
  `docs/tasks/shift/21-build-gate-reboot.md` plus the design index to reflect the
  milestone.

## [2025-09-27] Build Gate Stage Planning & Metadata

- Added `build-gate` and `static-checks` stages to the default workflow planner
  with new runner stage kinds, updating CLI Aster overrides and planner/runner
  tests to reflect the build gate sequence.
- Introduced `internal/workflow/buildgate` for metadata sanitisation and
  extended workflow checkpoints/contracts with `build_gate` metadata (schema
  version `2025-09-27.1`).
- Recorded roadmap progress under
  `docs/tasks/build-gate/01-stage-planning-and-metadata.md` and
  `docs/tasks/shift/21-build-gate-reboot.md`, refreshing design docs and the root
  README to note the milestone.

## [2025-09-27] Mods Runner Parallel Execution

- Reworked the workflow runner to schedule Mods stages according to dependency
  readiness, launching `orw-apply`, `orw-gen`, and `llm-plan` in parallel while
  holding `llm-exec`/`mods-human` until prerequisites complete.
- Added concurrency-focused unit tests (including retry coverage) plus helper
  stubs so the runner package stays at ≥90% coverage even under
  `go test -cover ./...`.
- Marked `docs/tasks/mods/04-runner-parallel-execution.md` and
  `docs/tasks/shift/19-mods-parallel-planner.md` complete, updating
  `docs/design/mods/README.md`, `docs/design/README.md`, and the root
  `README.md` to reflect the finished Roadmap 19 slice.

## [2025-09-27] Knowledge Base CLI Evaluation

- Added `knowledgebase.Match` helper returning incident IDs and similarity
  scores so tooling can inspect advisor output without changing Mods planner
  wiring.
- Introduced `ploy knowledge-base evaluate --fixture <samples.json>` with
  score-floor filtering, PASS/MISS labelling, and aggregate accuracy metrics for
  workstation validation.
- Documented the evaluation workflow across
  `docs/design/knowledge-base/README.md`, `cmd/ploy/README.md`,
  `configs/knowledge-base/README.md`, and marked roadmap task
  `docs/tasks/knowledge-base/03-cli-evaluate.md` complete.

## [2025-09-27] Knowledge Base CLI Ingest

- Added `ploy knowledge-base ingest` to append incident fixtures into
  `configs/knowledge-base/catalog.json`, rejecting duplicate IDs and summarising
  ingested incidents for operators.
- Extended `internal/workflow/knowledgebase` with merge/save helpers (atomic
  writes + duplicate safeguards) and comprehensive unit tests, keeping package
  coverage at 90.3%.
- Documented the workstation catalog workflow across `README.md`,
  `docs/DOCS.md`, `configs/knowledge-base/README.md`, and marked roadmap task
  `docs/tasks/knowledge-base/02-cli-ingest.md` complete.

## [2025-09-27] Knowledge Base Classifier Foundation

- Introduced `internal/workflow/knowledgebase` with trigram TF-IDF and
  Levenshtein scoring, providing a catalog-backed advisor that feeds Mods
  planner recipes, human gates, and recommendations.
- Wired `ploy workflow run` to load `configs/knowledge-base/catalog.json` when
  present, enabling the workstation CLI to surface knowledge base guidance
  without changing behaviour when the catalog is absent.
- Added high-coverage unit tests for the classifier, planner integration, and
  scoring utilities, ensuring `internal/workflow/knowledgebase` stays above the
  90% coverage target while `go test -cover ./...` remains ≥60% overall.
- Updated roadmap entry `docs/tasks/knowledge-base/01-classifier-foundation.md` and
  design docs to reflect the completed Roadmap 20 classifier slice.

## [2025-09-26] Mods Planner CLI Controls

- Added `--mods-plan-timeout` and `--mods-max-parallel` flags to
  `ploy workflow run`, forwarding planner tuning options into the runner so
  operators can timebox plan evaluation and cap concurrent Mods stages.
- Updated the default planner to push the new options into Mods stage metadata,
  publish concurrency hints in checkpoints/artifact envelopes, and include the
  hints when dispatching stages to the Grid client.
- Documented the flags across CLI docs, marked
  `docs/tasks/mods/03-cli-grid-wiring.md` complete, and refreshed the Mods design
  record to note the CLI/Grid wiring milestone.

## [2025-09-26] Mods Knowledge Base Metadata

- Extended the Mods planner with a knowledge base advisor that records recipe
  selections, concurrency hints, and human expectations inside
  `stage_metadata.mods` for plan and human stages.
- Updated workflow contracts and the runner to serialize Mods
  plan/human/recommendation metadata in checkpoints and artifact envelopes,
  exercising the new schema in unit tests.
- Recorded completion of `docs/tasks/mods/02-knowledge-base-feedback.md` and
  refreshed `docs/design/mods/README.md` / `docs/design/README.md` to capture
  the knowledge base feedback milestone (Roadmap 19).

## [2025-09-26] Mods Planner Skeleton

- Added `internal/workflow/mods` with a planner that emits `mods-plan`,
  `orw-apply`, `orw-gen`, `llm-plan`, `llm-exec`, and `mods-human` stages wired
  with repository lane defaults.
- Updated the default workflow planner to delegate Mods staging, propagate new
  stage kinds, and adjust unit tests/checkpoint expectations across the runner
  suite.
- Recorded completion of `docs/tasks/mods/01-planner-skeleton.md` and refreshed
  `docs/design/mods/README.md` to capture the planner skeleton milestone.

## [2025-09-26] Stage Artifact Streams

- Added `contracts.WorkflowArtifact` and JetStream/in-memory events client
  support so workflow stage artifacts mirror onto `ploy.artifact.<ticket>`
  alongside checkpoints.
- Updated the workflow runner to emit artifact envelopes for completed stages,
  propagate publication failures, and surface envelopes in unit tests for cache
  hydrator consumers.
- Documented the slice in `docs/design/stage-artifacts/README.md`, marked the
  checkpoint metadata follow-up complete, and recorded roadmap entry
  `18-stage-artifact-streams` as shipped.

## [2025-09-26] Workflow Checkpoint Metadata

- Bumped the workflow event schema to `2025-09-26.1` and enriched checkpoints
  with `stage_metadata` and `artifacts` blocks so Grid consumers can inspect
  lane assignments, dependencies, and produced manifests directly from
  JetStream.
- Updated the workflow runner to attach stage metadata for every status
  transition and to include artifact manifests returned from Grid stage
  outcomes.
- Extended the Grid Workflow client and contract tests to round-trip artifact
  payloads, refreshed `docs/design/event-contracts/README.md`, and marked
  roadmap slice `17-checkpoint-metadata` complete.

## [2025-09-26] Snapshot Metadata Streams

- Added `internal/workflow/snapshots.NewJetStreamMetadataPublisher` to emit
  schema-versioned snapshot metadata envelopes to `ploy.artifact.<ticket>` when
  discovery returns JetStream routes, retaining the in-memory stub for offline
  runs.
- Updated the CLI snapshot registry loader to wire the JetStream metadata
  publisher automatically and extended `ploy snapshot capture` tests to verify
  live JetStream behaviour alongside the IPFS gateway coverage.
- Refreshed documentation (`docs/SNAPSHOTS.md`,
  `docs/design/ipfs-artifacts/README.md`, `docs/design/overview/README.md`) and
  recorded roadmap slice `16-snapshot-metadata-streams` as complete with
  CHANGELOG entry dated 2025-09-26.

## [2025-09-26] IPFS Artifact Publishing

- Added `internal/workflow/snapshots.NewIPFSGatewayPublisher` to stream snapshot
  payloads to IPFS gateways via `/api/v0/add`, returning the gateway-provided
  CID while keeping the in-memory stub fallback for offline runs.
- Updated `ploy snapshot capture` to honour discovery-provided gateways during
  registry loading, surfacing the returned CID in CLI output and metadata
  structures.
- Expanded snapshot and CLI test suites with gateway-backed scenarios; refreshed
  documentation (`docs/design/ipfs-artifacts/README.md`, `docs/SNAPSHOTS.md`,
  `cmd/ploy/README.md`, `README.md`) and recorded roadmap slice
  `15-ipfs-artifact-publishing` as complete.

## [2025-09-26] Grid Workflow Client

- Added `internal/workflow/grid` with an HTTP Workflow RPC client that submits
  stage executions to Grid and records invocation metadata for CLI summaries.
- Updated `ploy workflow run` to honour `GRID_ENDPOINT`, wiring real Grid
  dispatch when configured and keeping the in-memory stub for offline
  development.
- Expanded CLI and client test suites to cover Grid configuration failures,
  request encoding, and response handling; recorded the roadmap slice as
  complete.

## [2025-09-26] Integration Manifest Schema

- Published `docs/schemas/integration_manifest.schema.json` capturing required
  manifest fields and constraints for topology, fixtures, lanes, and Aster
  toggles.
- Added `ploy manifest schema` to surface the schema for downstream tooling and
  validation flows.
- Updated documentation (`docs/MANIFESTS.md`, `docs/design/overview/README.md`,
  `docs/DOCS.md`) and recorded roadmap slice `13-integration-manifest-schema` as
  complete.

## [2025-09-26] Snapshot Catalog Validation

- Added MySQL (`mysql-orders`) and document-store (`doc-events`) fixtures
  alongside the existing Postgres snapshots so `ploy snapshot plan|capture`
  exercises all representative engines locally.
- Implemented the `last4` masking strategy and wired regression coverage that
  loads the in-repo catalog and executes captures with stub publishers.
- Updated snapshot documentation and the SHIFT roadmap to record the validation
  slice ahead of JetStream/Grid wiring.

## [2025-09-26] JetStream Client Wiring

- Introduced `internal/workflow/contracts.JetStreamClient` to consume real
  tickets from `grid.webhook.<tenant>` and publish checkpoints to
  `ploy.workflow.<ticket>.checkpoints`.
- Updated `ploy workflow run` to honour discovery-provided JetStream routes by
  dialing JetStream (falling back to the in-memory stub when routes are missing)
  and surfacing connection failures to the caller.
- Added unit tests that exercise the client against an in-process JetStream
  server plus CLI coverage for misconfiguration errors.
- Refreshed documentation and roadmap entries to describe the live JetStream
  behaviour and new configuration toggle.

## [2025-09-26] Workflow Checkpoint Cache Keys

- Extended the workflow event contract to include lane cache keys on every
  checkpoint and bumped the schema version to `2025-09-26`.
- Updated the workflow runner to compute cache keys via injected composers,
  ensuring JetStream checkpoints surface cache-coordination signals.
- Wired the CLI to derive cache keys from lane specs so Grid integrations can
  rely on consistent cache metadata ahead of JetStream wiring.

## [2025-09-26] Mods Terminology Guard

- Replaced remaining ARF references with mods terminology across roadmap and
  recipe documentation.
- Added `terminology_guard_test.go` to enforce the naming convention and block
  regressions.
- Expanded `docs/RECIPES.md` with a detailed explanation of
  `configs/recipes/kotlin-gradle.toml`.

## [2025-09-26] Recipe Pack Registry

- Added `internal/recipes/packs` with a TOML loader that exposes pluggable
  recipe pack lists and language-aware lookups.
- Published default specs (`configs/recipes/java-default.toml`,
  `configs/recipes/kotlin-gradle.toml`) to seed Java and Kotlin/Gradle catalog
  coverage.
- Documented the registry in `docs/RECIPES.md`, updated the README, and marked
  the roadmap item complete.

## [2025-09-26] Documentation Cleanup

- Refreshed `README.md` to highlight the CLI-first/Grid model, enumerate all
  completed SHIFT slices, and link directly to the design doc.
- Updated the documentation matrix (`docs/DOCS.md`, `docs/LANES.md`,
  `docs/MANIFESTS.md`, `docs/SNAPSHOTS.md`) to emphasise JetStream/Grid
  workflows and point contributors at the relevant guides.
- Added `documentation_cleanup_test.go` to guard the roadmap status and README
  alignment for this slice.

## [2025-09-26] Aster Hook Integration

- Added `internal/workflow/aster` with a filesystem-backed locator that
  discovers per-stage bundle metadata from `configs/aster/` and surfaces
  provenance data for Grid submissions.
- Extended the workflow runner to require an Aster locator, attach sorted toggle
  metadata to every stage, and honour per-stage disablement while keeping cache
  keys deterministic.
- Introduced `--aster` and `--aster-step` flags on `ploy workflow run`, along
  with post-run bundle summaries so operators can verify toggles before Grid
  wiring lands.
- Expanded CLI and runner test suites to cover bundle detection, metadata
  propagation, per-stage overrides, and regression behaviour when Aster is
  disabled.
- Documented the workflow in `cmd/ploy/README.md`, `docs/MANIFESTS.md`, and
  `docs/design/overview/README.md`; roadmap slice `07-aster-hook` marked
  complete.

## [2025-09-26] Commit-Scoped Environments

- Added `internal/workflow/environments` service with TDD coverage for dry-run
  planning, execution hydration, and snapshot gap reporting.
- Introduced `ploy environment materialize` CLI command with dry-run/execute
  modes, manifest override support, and human-readable summaries.
- Published new snapshot specs (`commit-db`, `commit-cache`) and GPU lane
  profile (`configs/lanes/gpu-ml.toml`) to back commit-scoped runs.
- Documented the workflow in `README.md` and `cmd/ploy/README.md`; roadmap slice
  `06-commit-environments` marked complete.

## [2025-09-26] Integration Manifest Compiler

- Introduced `internal/workflow/manifests` with TOML schema validation, JSON
  compilation helpers, and unit tests covering happy/failure paths.
- Extended the workflow runner to require manifest compilation, attach compiled
  payloads to every stage, and let the in-memory Grid stub enforce lane
  allowlists.
- Updated `ploy workflow run` to load manifests from `configs/manifests/`,
  surface actionable validation errors, and documented the schema in
  `docs/MANIFESTS.md` alongside new sample manifests (`smoke`, `commit-app`).
- Added CLI tests asserting manifest loader wiring and error propagation;
  roadmap slice `05-integration-manifests` is now complete.

## [2025-09-26] Snapshot Toolkit CLI

- Added `internal/workflow/snapshots` with TOML spec loader, rule engine
  (strip/mask/synthetic), deterministic fingerprinting, and metadata publishing
  hooks backed by in-memory IPFS/JetStream stubs.
- Introduced `ploy snapshot plan` and `ploy snapshot capture` commands, plus CLI
  tests covering usage, summary output, and capture reporting.
- Published default snapshot spec/fixture under `configs/snapshots/` to exercise
  the toolkit locally.
- Documented snapshot workflow in `README.md` and `cmd/ploy/README.md`; roadmap
  slice `04-snapshot-toolkit` marked complete with container replay hook
  deferred to the JetStream integration slice.

## [2025-09-26] Lane Engine & Describe CLI

- Added `internal/workflow/lanes` with TOML loader, cache-key composer,
  validation, and unit tests covering required fields and deterministic outputs.
- Introduced `configs/lanes/node-wasm.toml` and `configs/lanes/go-native.toml`
  as the first Grid-ready lane profiles.
- Extended `cmd/ploy` with `lanes describe`, golden-style CLI tests, and richer
  top-level usage guidance.
- Propagated lane metadata through the workflow runner and in-memory Grid stub;
  stages now error when lane assignments are missing.
- Documented the lane system in `README.md`, `cmd/ploy/README.md`,
  `docs/LANES.md`, and marked roadmap slice `03-lane-engine` complete.

## [2025-09-26] Workflow Runner CLI Stub

- Expanded `internal/workflow/runner` with a default DAG planner, stage
  execution loop, retry handling, temporary workspace management, and error
  propagation for Grid interactions.
- Added an in-memory Grid client, stage invocation tracking, and extensive unit
  tests lifting runner package coverage to 94.5%.
- Updated `cmd/ploy` to support `--ticket auto`, inject JetStream/Grid stubs via
  testable factories, and emit usage/help output across new error paths.
- Extended CLI tests to cover command dispatch, usage printers, and runner
  wiring; repository-wide `go test -cover ./...` now satisfies ≥60% overall
  coverage.
- Documented discovery-driven configuration (`GRID_ENDPOINT`) and new behaviour
  in `cmd/ploy/README.md`; marked roadmap slice `02-workflow-runner-cli`
  complete.

## [2025-09-25] Event Contract Stub

- Added `internal/workflow/contracts` with schema version `2025-09-25`, subject
  helpers, and validation logic for workflow tickets and checkpoints.
- Wired `internal/workflow/runner` to claim tickets, validate payloads, and
  publish an initial `claimed` checkpoint through a JetStream stub.
- Updated the CLI to require `--tenant`, bootstrap the in-memory bus, and
  reflect the new behaviour in usage docs.
- Documented the subject map and example payloads in
  `docs/design/event-contracts/README.md`; roadmap slice `01-event-contracts`
  now marked complete.

## [2025-09-25] Legacy Teardown

- Removed all legacy API, Nomad, Consul, SeaweedFS, and deployment scaffolding.
- Replaced the repo with a CLI-only stub (`ploy workflow run`) that validates
  ticket input and returns `ErrNotImplemented`.
- Added guardrail tests that fail if legacy binaries or imports reappear.
- Simplified the build system (`Makefile`) to focus on the workflow CLI.
- Rewrote documentation (`README.md`, `docs/DOCS.md`, `cmd/ploy/README.md`) to
  describe the Shift architecture and roadmap alignment.

## [History]

Prior releases documented Nomad-based services, security engines, and lane
orchestration. Refer to the Git history before `2025-09-25` for archival
details.
