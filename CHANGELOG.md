# Changelog

## [2026-02-25] Docker Runtime: Guard ContainerCreate Panics

- Added a panic guard in `internal/workflow/step/container_docker.go` so
  `DockerContainerRuntime.Create` converts unexpected Docker SDK panics into
  regular errors instead of crashing the node agent process.
- Extended the Docker runtime fake client to simulate panics during
  `ContainerCreate` (`internal/workflow/step/container_docker_fake_test.go`).
- Added regression coverage for panic-to-error behavior in
  `internal/workflow/step/container_docker_create_test.go`
  (`error_create_panics` case).
- Verification (2026-02-25):
  - `go test ./internal/workflow/step -run 'TestDockerContainerRuntimeCreate|TestDockerContainerRuntimeEnvPassthrough|TestDockerContainerRuntimeNetworkMode'`
  - `go test ./internal/nodeagent -run TestDoesNotExist`

## [2025-10-30] Build Gate: Java Executor Embedded

- Embedded the Java build gate executor in place of the previous CLI shell-out
  (`internal/workflow/buildgate/javaexec`). The executor prefers Gradle/Maven wrappers and
  falls back to a Dockerised Maven image when wrappers are absent.
- Worker and CLI now wire the BuildGate runner with the Java executor
  (step runner under `internal/workflow/runtime/step`, `cmd/ploy/dependencies_runtime_local.go`).
- Added a lifecycle checker that probes the configured Maven image presence
  (`internal/worker/lifecycle/health_buildgate.go`); removed the legacy build gate binary env ref from docs.
- Documentation updated: build gate overview and environment variables
  (`docs/workflow/README.md`, `docs/envs/README.md`).
- Verification (2025-10-30):
  - `make build`
  - `make test`

## [2025-10-26] SSH Transfer Documentation Pass

- Expanded `docs/next/api.md` with transfer slot schemas, artifact upload/download examples, and the
  full OCI registry flow so operators can trace every `/v1/transfers/*`, `/v1/artifacts/*`, and
  `/v1/registry/*` request.
- Added SSH transfer workflow, slot lifecycle, monitoring guidance, and FAQ entries to
  `docs/next/ipfs.md`; recorded the SFTP subsystem requirements in `docs/next/devops.md` and a new
  runbook at `docs/runbooks/control-plane/ssh-transfer.md`.
- Authored `docs/next/ssh-transfer-migration.md`, updated the CLI reference/README, and refreshed
  `docs/envs/README.md` so teams know how to cut over from direct IPFS uploads.
- Linked the new documentation from `docs/next/README.md` and `cmd/ploy/README.md` so future slices
  can discover the transfer playbooks quickly.
- Verification (2025-10-26):
  - `make test`

## [2025-10-21] CLI Surface Refresh Decomposition

- Documented the CLI surface refresh roadmap as five focused slices
  (`docs/tasks/roadmap/05a-cli-command-tree-refresh.md` through
  `docs/tasks/roadmap/05e-cli-operator-enablement.md`) with COSMIC sizing,
  dependencies, and verification guidance.
- Updated the task queue (`docs/tasks/README.md`) to reflect the new slices and
  maintain dependency ordering for roadmap execution.

## [2025-10-21] Mods Step Runtime Artifact Publishing

- Local step executions now persist diff/log artifact CIDs plus retention metadata on stage
  outcomes and record the invocations for CLI summaries (`internal/workflow/runtime/local_client.go`,
  `internal/workflow/runner/stub.go`, `internal/workflow/grid/client.go`).
- Workflow checkpoints embed retention details so the CLI can print a **Stage Artifacts** summary
  with CIDs, digests, and TTLs; `cmd/ploy/mod_run.go` and `cmd/ploy/mod_summaries.go` render the
  new output with updated golden coverage.
- Documentation refreshed to describe immediate artifact availability and CLI inspection flows
  (`docs/next/job.md`, `docs/next/mod.md`, `docs/workflow/README.md`).
- Verification (2025-10-21):
  - `go test ./...`

## [2025-10-21] IPFS Cluster Artifact Store

- Replaced the filesystem artifact publisher with an IPFS Cluster-backed client
  (`internal/workflow/artifacts`) that streams diff/log payloads, records
  SHA-256 digests, and honours replication overrides. The local runtime now
  injects this publisher by default.
- Added CLI artifact commands (`ploy artifact push|pull|status|rm`) backed by
  the same cluster client, including digest reporting and download
  verification (`cmd/ploy/artifact_command.go`, new unit coverage).
- Documented cluster configuration knobs, updated the workflow overview, and
  tracked operational guidance in `docs/next/ipfs.md`.
- Verification (2025-10-21):
  - `go test ./...`

## [2025-10-21] Local Step Runtime Wiring

- Defaulted the CLI to the `local-step` runtime adapter and registered a Docker
  backed `runtime/step` container implementation plus filesystem workspace
  hydration, diff tarball generation, and artifact staging modules used by the
  local step client.
- Updated Mods documentation to describe local execution and staged artifacts
  (`docs/workflow/README.md`, `docs/next/README.md`, `docs/next/job.md`,
  `docs/next/mod.md`) and captured the follow-on artifact
  publisher work in `docs/tasks/roadmap/03a-mod-runtime-artifacts.md`.
- Closed out roadmap task `docs/tasks/roadmap/02-mod-step-runtime.md`, marking it
  completed and logging verification steps; refreshed README/env references for
  the new runtime default.
- Verification (2025-10-21):
  - `go test ./internal/workflow/runtime/...`
  - `go test ./cmd/...`
  - `go test ./...`

## [2025-10-21] Mods Step Runtime Foundations

- Introduced a node-local runtime client that adapts `internal/workflow/runtime/step.Runner` to the
  workflow runner interface and emits sanitized build gate diagnostics.
- Extended step manifest requests with workspace context, surfaced build gate metadata on stage
  checkpoints, and added unit coverage for build gate failure propagation and local runtime outcomes.
- Task `docs/tasks/roadmap/02-mod-step-runtime.md` now tracks this foundation and enumerates the
  follow-up wiring required to retire the Grid adapter.
- Verification (2025-10-21):
  - `go test ./internal/workflow/runtime/...`
  - `go test ./internal/workflow/runtime/step/...`
  - `go test ./...`

## [2025-10-21] Control Plane Scheduler

- Added `internal/controlplane/scheduler` with etcd-backed job submission, claim, heartbeat, and
  completion flows using optimistic transactions and lease watchers.
- Introduced an HTTP surface in `internal/server/http` with lifecycle tests and embedded
  etcd fixtures.
- Created integration suite under `tests/integration/controlplane` exercising multi-worker claims
  and lease expiry requeues; refreshed unit tests for heartbeat renewal and TTL enforcement.
- Documented scheduler behaviour across `docs/next/etcd.md`, `docs/next/queue.md`, `docs/next/job.md`,
  and added the `docs/runbooks/control-plane/job-recovery.md` operational guide alongside updated
  environment variable references.
- Updated `docs/design/control-plane/README.md` and roadmap task metadata to reflect completion of
  `roadmap-control-plane-01` with verification evidence.
- Verification (2025-10-21):
  - `go test ./internal/controlplane/...`
  - `go test -tags integration ./tests/integration/controlplane`
  - `staticcheck ./internal/controlplane/...`

## [2025-10-09] Discovery Env Cleanup

- Retired the legacy discovery override variable so the CLI now reads the beacon
  host exclusively from `GRID_BEACON_URL` and no longer filters inherited shell
  environments (cmd/ploy/dependencies.go, cmd/ploy/dependencies_clients.go,
  tests/e2e/mods_scenarios_test.go).
- Removed helper utilities and tests that referenced the retired toggle to keep
  the harness focused on the supported configuration (cmd/ploy/mod_run_grid_test.go,
  cmd/ploy/dependencies_discovery_test.go, cmd/ploy/test_support_test.go).
- Updated developer documentation to call out `GRID_BEACON_URL` for overrides
  and refreshed agent guidance to match (README.md, cmd/ploy/README.md,
  docs/envs/README.md, tests/e2e/README.md, AGENTS.md, tests/e2e/config.go).
- Verified via `go test ./...` on 2025-10-09. Refs mods-grid-05.

## [2025-10-11] Grid Client Adoption

- Switched the Ploy CLI to use the shared `sdk/gridclient/go` library, removed
  the legacy endpoint override handling, and enforced `{PLOY_GRID_ID, GRID_BEACON_API_KEY}` credentials
  for live runs while retaining in-memory fallbacks (cmd/ploy/dependencies*.go,
  internal/workflow/grid/client.go).
- Updated unit tests and the Mods/snapshot harnesses to stub the shared client
  and reset legacy environment variables; refreshed the e2e configuration to
  propagate the new inputs (cmd/ploy/*_test.go, tests/e2e/config.go,
  tests/e2e/mods_scenarios_test.go).
- Refreshed CLI/docs/environment references to document the new client flow
  and the endpoint override removal (README.md, cmd/ploy/README.md,
  docs/envs/README.md, docs/design/workflow-rpc-alignment/README.md,
  docs/tasks/grid-client/04-ploy-adoption.md).
- Verified via `go test ./...` on 2025-10-11.

## [2025-10-09] Mods CLI Rename

- Retired the `ploy workflow run` command in favour of `ploy mod run`, updating
  CLI helpers, tests, and usage text to reflect the Mods-specific entrypoint.
- Routed the `workflow` top-level command exclusively to cancellation flows and
  refreshed CLI usage output to match.
- Updated documentation, design records, and environment references to the new
  command name; recorded the rename across tasks and roadmap notes for future
  slices.

## [2025-10-08] Lane Catalog Externalised

- Published [`iw2rmb/ploy-lanes-catalog`](https://github.com/iw2rmb/ploy-lanes-catalog)
  and removed embedded lane definitions from this repository; the CLI now loads
  lanes from `$PLOY_LANES_DIR` (falling back to XDG paths or an adjacent
  checkout).
- Updated CLI docs, environment references, and design tasks to point at the new
  catalogue; added `PLOY_LANES_DIR` to the documented environment variables and
  refreshed the lane validator default.
- Added optional `GRID_BEACON_API_KEY`/`PLOY_GRID_ID` handling alongside the existing
  endpoint override, wiring bearer tokens into the Grid helper and discovery calls
  while updating tests and env listings.
- Extended Mods live Grid smoke to accept `PLOY_E2E_LIVE_SCENARIOS`, allowing
  `TestModsScenariosLiveGrid` to run multiple scenarios via `go test -tags e2e
  ./tests/e2e`; documented the knob in `docs/envs/README.md` and the refactor
  task notes after verifying `ploy lanes describe` against the published lane catalog.

## [2025-10-07] Mods Catalog Alignment

- `docs/design/mods-grid-restoration/README.md`: Reframed follow-up work around a
  dedicated Mods lane repository and Grid catalog registration; re-reviewed
  `go test ./internal/workflow/runner -run TestRunSchedulesHealingPlanAfterBuildGateFailure -count=1`,
  `go test ./cmd/ploy -run TestHandleModRunConfiguresModsFlags -count=1`, and
  `go test -tags e2e ./tests/e2e -count=1` (latest run 2025-10-05) to confirm
  behaviour remains unchanged.
- `docs/design/workstation-roadmap/README.md`: Updated the workstation roadmap
  alignment to track the Mods catalog namespace hand-off to Grid and call out
  coordination requirements (desk review with Mods design + Grid plan on
  2025-10-07).
- `docs/design/README.md`: Refreshed index entries for workstation roadmap/Mods
  designs to highlight the Grid catalog follow-up and new verification timestamp.
- Noted the `/lanes/<namespace>.tar.gz` publication path in the Mods/Grid
  designs so workstation docs match the updated Grid endpoint.

## [2025-10-05] Mods Grid Restoration (Steps 1–4)

- `ploy mod run` now materialises repositories via `WorkflowRun.Repo`,
  cloning into a temporary workspace and honouring optional workspace hints
  before Mods stages execute.
- Mods-specific lanes (`lanes/mods-*.toml`) feed the job composer so
  OpenRewrite, LLM, and human stages dispatch with correct container images and
  resources; build-gate failures trigger healing retries that append `#healN`
  branches using knowledge base signals.
- Documentation refreshed: `cmd/ploy/README.md`,
  `docs/design/mods-grid-restoration/README.md`, `docs/design/mods/README.md`,
  `docs/design/event-contracts/README.md`, and `docs/design/workstation-roadmap/README.md`
  now describe repo materialisation, stage metadata, and healing behaviour.
- Introduced `lanes/` catalog (mirrored to the shared registry) and updated the CLI
  to read from this catalog by default.
- Restored Mods E2E harness under `tests/e2e` so workstation runs validate
  simple, self-healing, and parallel scenarios against the in-memory Grid stub.
- Verification:
  `go test ./internal/workflow/runner -run TestRunSchedulesHealingPlanAfterBuildGateFailure -count=1`,
  `go test ./cmd/ploy -run TestHandleModRunConfiguresModsFlags -count=1`,
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

- Surfaced build gate results in `ploy mod run` by printing static check
  outcomes, failing diagnostics, knowledge base findings, and log digests
  sourced from checkpoint metadata.
- Added log finding support to workflow contracts and runner checkpoint
  conversion so Knowledge Base codes propagate into CLI summaries and downstream
  consumers.
- Documented the milestone in `docs/design/build-gate/README.md`, the updated
  workstation roadmap design index, and roadmap slice
  `docs/tasks/build-gate/07-cli-summary.md`, marking
  `docs/tasks/roadmap/21-build-gate-reboot.md` as complete.

## [2025-09-29] Grid Discovery Alignment

- Updated `cmd/ploy/dependencies.go` to parse the full `/v1/cluster/info`
  payload (API endpoint, JetStream route list, IPFS gateway, feature map,
  version) with strict decoding, caching, and helper accessors.
- Extended discovery and workflow tests
  (`cmd/ploy/dependencies_discovery_test.go`,
  `cmd/ploy/mod_run_grid_test.go`) to cover multi-route handling, feature
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
  existing Go output via `cmd/ploy/mod_run_output_test.go` and normalised
  static check language aliases by mapping `javac` to `java`.
- Deflaked `TestRunExecutesParallelModsStages` by extending the stage start wait
  window to accommodate concurrent scheduling under heavier load.
- Updated documentation and roadmap artifacts:
  `docs/design/build-gate/error-prone/README.md`,
  `docs/design/build-gate/README.md`, `docs/design/README.md`,
  `docs/design/workstation-roadmap/README.md`,
  `docs/tasks/build-gate/08-error-prone-adapter.md`, and
  `docs/tasks/roadmap/21-build-gate-reboot.md` to record the adapter milestone
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
  `cmd/ploy/mod_run_output_test.go` to surface ESLint failures in build
  gate summaries.
- Updated documentation and roadmap artifacts:
  `docs/design/build-gate/eslint/README.md`, `docs/design/build-gate/README.md`,
  `docs/design/README.md`, `docs/design/workstation-roadmap/README.md`, and
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
  `cmd/ploy/snapshot_test.go`, `cmd/ploy/mod_run_grid_test.go`) to exercise
  discovery-driven configuration and sanitized defaults.
- Refreshed documentation (`docs/envs/README.md`, `README.md`,
  `cmd/ploy/README.md`, `docs/SNAPSHOTS.md`,
  `docs/design/{event-contracts,checkpoint-metadata,ipfs-artifacts,snapshot-metadata,stage-artifacts,overview}/README.md`,
  `docs/design/README.md`) and aligned roadmap entries with the discovery-only
  behaviour.
- Verification: `go test ./...` on 2025-09-29.

## Nov 4, 2025

- Node Agent:
  - Always bundle and upload the `/out` directory as an artifact bundle named `mod-out` when it contains files, regardless of `artifact_paths` option.
  - Added unit tests for `uploadOutDirIfPresent` to verify upload behavior and endpoint contract.

## [2025-09-27] Build Gate Log Retrieval & Parsing

- Added `internal/workflow/buildgate.LogRetriever` and `LogIngestor` to download
  Grid build logs with IPFS fallbacks, clamp payload size, compute deterministic
  SHA-256 digests, and parse Knowledge Base findings into checkpoint metadata.
- Introduced a default log parser and metadata sanitisation updates that
  normalise Git authentication, Go module conflict, linker, and disk pressure
  findings for downstream remediation flows.
- Documented the milestone across the build gate design record, roadmap slice
  `docs/tasks/build-gate/04-log-retrieval-and-grid-integration.md`, the
  workstation roadmap tracker, and the design index so roadmap status stays aligned.

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
  design index, `docs/tasks/roadmap/21-build-gate-reboot.md`, and the new roadmap
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

- Updated `internal/workflow/contracts` with shared subject constants so run
  claims flow through `webhook.<tenant>.ploy.workflow-run` and status polling
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
- Migrated shipped manifests (commit-app, smoke), refreshed `docs/MANIFESTS.md`, updated the v2
  design record, and marked roadmap slice
  `docs/tasks/integration-manifests/01-schema-upgrade.md` complete.

## [2025-09-28] Workflow JobSpec Composition

- Extended lane specs with validated `job` blocks
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
  integration when the endpoint override is configured.
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
  `docs/design/build-gate/README.md`, `docs/tasks/roadmap/21-build-gate-reboot.md`).

## [2025-10-05] Build Gate Static Check Registry

- Added `internal/workflow/buildgate.StaticCheckRegistry` with lane default,
  manifest override, and skip handling so build gate stages can coordinate
  language-specific adapters with deterministic severity thresholds.
- Introduced `internal/workflow/buildgate/static_checks_test.go` covering
  severity evaluation, manifest disablement, and skip overrides with fake
  adapters to keep package coverage ≥90%.
- Documented the registry in `docs/design/build-gate/README.md`, marked roadmap
  slice `docs/tasks/build-gate/03-static-check-registry.md` complete, and updated
  `docs/tasks/roadmap/21-build-gate-reboot.md` plus the design index to reflect
  the milestone.

## [2025-09-27] Build Gate Stage Planning & Metadata

- Added `build-gate` and `static-checks` stages to the default workflow planner
  with new runner stage kinds, updating planner/runner tests to reflect the
  build gate sequence.
- Introduced `internal/workflow/buildgate` for metadata sanitisation and
  extended workflow checkpoints/contracts with `build_gate` metadata (schema
  version `2025-09-27.1`).
- Recorded roadmap progress under
  `docs/tasks/build-gate/01-stage-planning-and-metadata.md` and
  `docs/tasks/roadmap/21-build-gate-reboot.md`, refreshing design docs and the root
  README to note the milestone.

## [2025-09-27] Mods Runner Parallel Execution

- Reworked the workflow runner to schedule Mods stages according to dependency
  readiness, allowing planner output stages to run in parallel while
  holding `mods-human` until prerequisites complete.
- Added concurrency-focused unit tests (including retry coverage) plus helper
  stubs so the runner package stays at ≥90% coverage even under
  `go test -cover ./...`.
- Marked `docs/tasks/mods/04-runner-parallel-execution.md` and
  `docs/tasks/roadmap/19-mods-parallel-planner.md` complete, updating
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
  knowledge base docs, and marked roadmap task
  `docs/tasks/knowledge-base/03-cli-evaluate.md` complete.

## [2025-09-27] Knowledge Base CLI Ingest

- Added `ploy knowledge-base ingest` to append incident fixtures into
  the knowledge base catalog, rejecting duplicate IDs and summarising
  ingested incidents for operators.
- Extended `internal/workflow/knowledgebase` with merge/save helpers (atomic
  writes + duplicate safeguards) and comprehensive unit tests, keeping package
  coverage at 90.3%.
- Documented the workstation catalog workflow across `README.md`,
  `docs/DOCS.md`, knowledge base docs, and marked roadmap task
  `docs/tasks/knowledge-base/02-cli-ingest.md` complete.

## [2025-09-27] Knowledge Base Classifier Foundation

- Introduced `internal/workflow/knowledgebase` with trigram TF-IDF and
  Levenshtein scoring, providing a catalog-backed advisor that feeds Mods
  planner recipes, human gates, and recommendations.
- Wired `ploy mod run` to load the knowledge base catalog when
  present, enabling the workstation CLI to surface knowledge base guidance
  without changing behaviour when the catalog is absent.
- Added high-coverage unit tests for the classifier, planner integration, and
  scoring utilities, ensuring `internal/workflow/knowledgebase` stays above the
  90% coverage target while `go test -cover ./...` remains ≥60% overall.
- Updated roadmap entry `docs/tasks/knowledge-base/01-classifier-foundation.md` and
  design docs to reflect the completed Roadmap 20 classifier slice.

## [2025-09-26] Mods Planner CLI Controls

- Added `--mods-plan-timeout` and `--mods-max-parallel` flags to
  `ploy mod run`, forwarding planner tuning options into the runner so
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

- Introduced an experimental Mods planner and skeleton stages for `mods-plan`
  and `mods-human`; this planner has since been removed in favour of the
  current spec‑driven Mods pipeline.

## [2025-09-26] Stage Artifact Streams

- Initially added `contracts.WorkflowArtifact` and JetStream/in-memory events
  client support for per‑stage artifact envelopes; this experimental path has
  since been removed in favour of the current Mods/job pipeline.

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
  schema-versioned snapshot metadata envelopes to `ploy.artifact.<run_id>` when
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
- Updated `ploy mod run` to honour the endpoint override, wiring real Grid
  dispatch when configured and keeping the in-memory stub for offline
  development.
- Expanded CLI and client test suites to cover Grid configuration failures,
  request encoding, and response handling; recorded the roadmap slice as
  complete.

## [2025-09-26] Integration Manifest Schema

- Published `docs/schemas/integration_manifest.schema.json` capturing required
  manifest fields and constraints for topology, fixtures, and lanes.
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
- Updated snapshot documentation and the workstation roadmap to record the
  validation slice ahead of JetStream/Grid wiring.

## [2025-09-26] JetStream Client Wiring

- Introduced `internal/workflow/contracts.JetStreamClient` to consume real runs
  from `grid.webhook.<tenant>` and publish checkpoints to
  `ploy.workflow.<run_id>.checkpoints`.
- Updated `ploy mod run` to honour discovery-provided JetStream routes by
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
  the Kotlin/Gradle recipe.

## [2025-09-26] Recipe Pack Registry

- Added `internal/recipes/packs` with a TOML loader that exposes pluggable
  recipe pack lists and language-aware lookups.
- Published default recipe specs (java-default, kotlin-gradle) to seed Java and Kotlin/Gradle catalog
  coverage.
- Documented the registry in `docs/RECIPES.md`, updated the README, and marked
  the roadmap item complete.

## [2025-09-26] Documentation Cleanup

- Refreshed `README.md` to highlight the CLI-first/Grid model, enumerate all
  completed workstation roadmap slices, and link directly to the design doc.
- Updated the documentation matrix (`docs/DOCS.md`, `docs/LANES.md`,
  `docs/MANIFESTS.md`, `docs/SNAPSHOTS.md`) to emphasise JetStream/Grid
  workflows and point contributors at the relevant guides.
- Added `documentation_cleanup_test.go` to guard the roadmap status and README
  alignment for this slice.

## [2025-09-26] Commit-Scoped Environments

- Added `internal/workflow/environments` service with TDD coverage for dry-run
  planning, execution hydration, and snapshot gap reporting.
- Introduced `ploy environment materialize` CLI command with dry-run/execute
  modes, manifest override support, and human-readable summaries.
- Published new snapshot specs (`commit-db`, `commit-cache`) and GPU lane
  profile to back commit-scoped runs.
- Documented the workflow in `README.md` and `cmd/ploy/README.md`; roadmap slice
  `06-commit-environments` marked complete.

## [2025-09-26] Integration Manifest Compiler

- Introduced `internal/workflow/manifests` with TOML schema validation, JSON
  compilation helpers, and unit tests covering happy/failure paths.
- Extended the workflow runner to require manifest compilation, attach compiled
  payloads to every stage, and let the in-memory Grid stub enforce lane
  allowlists.
- Updated `ploy mod run` to load manifests,
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
- Published a default snapshot spec/fixture to exercise
  the toolkit locally.
- Documented snapshot workflow in `README.md` and `cmd/ploy/README.md`; roadmap
  slice `04-snapshot-toolkit` marked complete with container replay hook
  deferred to the JetStream integration slice.

## [2025-09-26] Lane Engine & Describe CLI

- Added `internal/workflow/lanes` with TOML loader, cache-key composer,
  validation, and unit tests covering required fields and deterministic outputs.
- Introduced node-wasm and go-native lane profiles
  as the first Grid-ready lane profiles.
- Extended `cmd/ploy` with `lanes describe`, golden-style CLI tests, and richer
  top-level usage guidance.
- Propagated lane metadata through the workflow runner and in-memory Grid stub;
  stages now error when lane assignments are missing.
- Documented the lane system in `README.md`, `cmd/ploy/README.md`,
  `docs/LANES.md`, and marked roadmap slice `03-lane-engine` complete.


- Expanded `internal/workflow/runner` with a default DAG planner, stage
  execution loop, retry handling, temporary workspace management, and error
  propagation for Grid interactions.
- Added an in-memory Grid client, stage invocation tracking, and extensive unit
  tests lifting runner package coverage to 94.5%.
## [2025-09-26] Workflow Runner CLI Stub
  testable factories, and emit usage/help output across new error paths.
- Extended CLI tests to cover command dispatch, usage printers, and runner
  wiring; repository-wide `go test -cover ./...` now satisfies ≥60% overall
  coverage.
- Documented discovery-driven configuration and new behaviour
  in `cmd/ploy/README.md`; marked roadmap slice `02-workflow-runner-cli`
  complete.

## [2025-09-25] Event Contract Stub

- Added `internal/workflow/contracts` with schema version `2025-09-25`, subject
  helpers, and validation logic for workflow runs and checkpoints.
- Wired `internal/workflow/runner` to claim runs, validate payloads, and
  publish an initial `claimed` checkpoint through a JetStream stub.
- Updated the CLI to require `--tenant`, bootstrap the in-memory bus, and
  reflect the new behaviour in usage docs.
- Documented the subject map and example payloads in
  `docs/design/event-contracts/README.md`; roadmap slice `01-event-contracts`
  now marked complete.


- Removed legacy API surface and deployment scaffolding tied to the pre-pivot orchestration stack.
## [2025-09-25] Legacy Teardown
- Added guardrail tests that fail if legacy binaries or imports reappear.
- Simplified the build system (`Makefile`) to focus on the workflow CLI.
- Rewrote documentation (`README.md`, `docs/DOCS.md`, `cmd/ploy/README.md`) to
  describe the workflow architecture and roadmap alignment.

## [History]

Prior releases documented the pre-pivot orchestration stack, security engines, and lane
orchestration. Refer to the Git history before `2025-09-25` for archival
details.
## [2025-10-30] Snapshot/JetStream Removal and Grid Pruning

- Removed snapshot tooling and registry from the CLI and environment materialization.
- Dropped JetStream/NATS client and metadata publisher; SSE is the only streaming surface.
- Scrubbed remaining Grid/JetStream mentions from CLI help/completions and top‑level docs.
- Tests updated accordingly; historical entries below may reference Grid/JetStream — they are legacy and no longer applicable to the current codebase.
- Removed client-provided run identifier support in `cmd/ploy` (`--run` flag). Runs are now always server-assigned.
- Replaced the repo with a CLI-only stub (`ploy mod run`, formerly `ploy workflow run`) that validates
  run input and returns `ErrNotImplemented`.
