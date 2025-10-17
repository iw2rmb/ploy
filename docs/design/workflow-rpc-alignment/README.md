# Workflow RPC Alignment (Roadmap 22)

## Purpose

Define how Ploy consumes Grid's Workflow RPC using the official SDK/helper,
standardise job payloads (`image`, `command`, `env`, `resources`) derived from
lane specifications, and align JetStream subject naming so both projects share a
single contract.

## Scope

- Replace the bespoke HTTP client under `internal/workflow/grid` with the
  first-party Grid Workflow RPC SDK (`github.com/iw2rmb/grid/sdk/workflowrpc/go`)
  while keeping workstation fakes for tests.
- Adopt the Grid Workflow RPC helper layer once available so configuration,
  retries, and streaming reuse the upstream abstractions.
- Adapt the workflow runner to build `workflowsdk.JobSpec` payloads from lane
  metadata, manifest constraints, and Aster toggles.
- Synchronise JetStream subjects between Ploy and Grid
  (`webhook.<tenant>.ploy.workflow-ticket`, `jobs.<run>.events`) and surface the
  shared constants through `internal/workflow/contracts`.
- Refresh CLI and documentation to reference the RPC endpoints
  (`/v1/workflows/rpc/runs*`) instead of the legacy stage API.

## Current Status

### 2025-10-05

- Workflow cancellation flows end-to-end via the SDK `Cancel` call, the CLI
  exposes `workflow cancel`, and `StageOutcome` now records run IDs plus
  keep-forever archive metadata for CLI summaries.
- The Workflow RPC SDK state directory defaults to
  `${XDG_CONFIG_HOME:-$HOME/.config}/ploy/grid`, with
  `GRID_WORKFLOW_SDK_STATE_DIR` still honoured for overrides, ensuring manifest
  and CA caches persist across CLI runs.

### 2025-10-01

- SDK + helper adoption complete: `internal/workflow/grid.Client` submits
  `workflowsdk.SubmitRequest` payloads via the official helper and streams run
  state with `helper.StreamStatusWithRetry` while workstation tests inject
  fakes.
- JobSpec composition is wired end-to-end: lane definitions declare job
  defaults, the runner composes `workflowsdk.JobSpec` payloads via the injected
  lane registry, and the Grid client enriches metadata with lane/cache/manifest
  details.
- Stage execution waits for terminal Workflow RPC events, falling back to run
  metadata when streaming reconnects, so Ploy no longer depends on synchronous
  stage responses.
- JetStream subject alignment shipped:
  `internal/workflow/contracts.SubjectsForTenant` now derives
  `webhook.<tenant>.ploy.workflow-ticket` inboxes and `jobs.<run_id>.events`
  streams, with contract tests covering trimmed inputs and empty identifiers.
- Helper adoption completed: the Grid client constructs helper-backed
  submitters that inject bearer auth and retry transient Workflow RPC failures.

## Background

Grid consolidated workflow submission, status streaming, and cancellation behind
the Workflow RPC service (`docs/design/workflow-rpc/README.md` in Grid). Ploy
still posts to `/workflow/stages` and listens on `grid.status.<ticket>`, so live
runs fail once Grid enables the real RPC handler. Aligning the contract
requires:

- Using the SDK’s typed client so Ploy inherits API evolution automatically.
- Populating `JobSpec` with lane-derived image/command/env/resources, so
  scheduler cache hints remain intact.
- Reflecting Grid’s current JetStream subjects
  (`webhook.<tenant>.<source>.<event>`, `jobs.<run_id>.events`) across tickets,
  checkpoints, and log retrieval.

## Client Integration

- `internal/workflow/grid.Client` composes the official SDK client, builds
  submit payloads with helper builders, and streams run status until a terminal
  event (with metadata fallback on reconnect).
- Cancellation flows reuse the SDK helper, and terminal metadata now feeds
  archive export details (ID, class, queued timestamp) back into the CLI.
- In-memory workflow clients remain for tests; helper-backed clients are
  selected when grid credentials (`PLOY_GRID_ID`, `GRID_BEACON_API_KEY`) are set. When
  connected, Ploy ensures the
  Workflow SDK state dir exists (defaulting under `~/.config/ploy/grid`) so
  manifest and CA caches persist between invocations.

## Runner Data Composition

- Lane definitions (sourced from `configs/lanes`) retain optional `image`,
  `command`, `env`, and resource hints.
- The runner assembles `workflowsdk.JobSpec` using lane defaults plus
  manifest/Aster overrides and records textual resource hints in metadata so
  Grid can score workloads even when numeric limits are unspecified.
- Cache keys continue to publish via checkpoints for downstream cache
  coordination.

## Events & Subject Alignment

- `internal/workflow/contracts` exposes constants for webhook inbox
  (`webhook.<tenant>.ploy.workflow-ticket`) and status stream
  (`jobs.<run_id>.events`).
- Runner consumes status events via the RPC stream (`jobs.<run_id>.events`)
  instead of polling legacy `grid.status.<ticket>`.
- Build-gate log retrieval uses the new event subjects to locate artifacts via
  job CIDs, and archive exports now surface alongside stage summaries.

## CLI Surface

- `mod run` claims tickets, streams Workflow RPC status, and prints stage
  summaries enriched with archive export identifiers when Grid queues
  keep-forever runs.
- `workflow cancel` requires `PLOY_GRID_ID` and `GRID_BEACON_API_KEY`, calls the Workflow RPC cancel
  endpoint, records the run status, and surfaces whether the cancellation was
  new or the run was already terminal.
- CLI help references Workflow RPC endpoints, helper configuration flows, and
  credential expectations; documentation and roadmap entries link back to this
  design so downstream consumers stay in sync.

## Job Spec Schema (2025-10-01)

- `image` — OCI image for the Grid runtime adapter. Required for container
  submissions; must resolve via lane defaults or manifest overrides.
- `command` — executable plus arguments. Optional but Ploy supplies lane or
  manifest defaults when present.
- `env` — merged environment variables from lane configuration, manifest
  toggles, and workflow inputs. Optional; empty maps are omitted.
- `resources` — structured CPU/memory/IO limits. Optional; Ploy records string
  hints in `job.metadata` when numeric values are unavailable.
- `wasm` — mutually exclusive with `image`; required when executing WebAssembly
  stages. Ploy sets `job.runtime=wasmtime` automatically when populated.

Additional fields (`workdir`, `secret_refs`, `accelerators`, `metadata`) remain
optional but Ploy fills metadata with lane, cache, manifest, and resource hints
so Grid scoring and diagnostics stay informative.

## Clarifications (2025-09-27)

- Grid exposes workflow submission under `/v1/workflows/rpc/runs*`. Legacy
  `/v1/workflows/jobs*` routes were removed during the shift; the SDK and helper
  target only the RPC surface. Verification on 2025-09-27 confirmed both the
  HTTP handlers (`../grid/internal/httpapi/workflow_rpc_routes.go`) and the Go
  SDK (`../grid/sdk/workflowrpc/go/client.go`) point at the RPC endpoints.

## Risks & Mitigations

- **Lane metadata gaps**: introduce validation that lanes declare the minimum
  job spec fields; fallback defaults remain for existing specs.
- **Stub parity**: extend the in-memory grid to emulate RPC responses so
  workstation tests remain deterministic.
- **Cross-repo drift**: publish shared subject constants (short-term via
  duplicated values, long-term via generated package or shared module).

## Dependencies

- Grid Workflow RPC SDK (`grid/sdk/workflowrpc/go`).
- Grid Workflow RPC helper (`grid/sdk/workflowrpc/helper`).
- Existing lane and manifest loaders for cache and command metadata.

## Implementation Notes

- **2025-10-01** — Adopted the helper-backed Workflow RPC submitter and streaming
  loop: `internal/workflow/grid/client.go` now builds `workflowsdk.SubmitRequest`
  payloads, streams status via `helper.StreamStatusWithRetry`, falls back to run
  metadata on reconnect, and exposes lane/cache/manifest metadata through job
  payloads.
- **2025-10-01** — Removed the local `internal/workflow/grid/workflowrpc` shim in
  favour of the upstream SDK/helper modules; workstation fakes wrap the helper
  factory for tests.
- **2025-09-30** — Updated `internal/workflow/contracts` subject helpers to
  publish and subscribe via `webhook.<tenant>.ploy.workflow-ticket` and
  `jobs.<run_id>.events`, added trimming safeguards, and refreshed JetStream
  tests to cover the new wildcard patterns.
- **2025-09-28** — `internal/workflow/grid.Client` gained an injectable factory
  and invocation tracking so downstream tests can substitute fake clients while
  production paths use the SDK.
- **2025-09-28** — Added `runner.LaneJobComposer` with CLI wiring so stage
  execution composes `JobSpec` payloads from lane metadata, merges
  env/resources, and stamps lane/cache/manifest metadata before submission.

## Deliverables

- SDK-backed workflow client with unit tests.
- Runner tests covering `JobSpec` assembly and RPC/helper invocation failure
  modes.
- Updated documentation (design index, CLI README, build gate design)
  referencing the new contract and helper guidance.
- Subject helper alignment across `internal/workflow/contracts` and JetStream
  tests ensuring `webhook.<tenant>.ploy.workflow-ticket` and
  `jobs.<run_id>.events` are treated as first-class constants.
- Roadmap tasks completed with changelog entries dated on completion.

## References

- Grid Workflow RPC design (`../grid/docs/design/workflow-rpc/README.md`).
- Grid Workflow RPC SDK implementation (`../grid/sdk/workflowrpc/go/client.go`).
- Grid Workflow RPC helper roadmap
  (`../grid/docs/tasks/workflow-rpc/04-sdk-helper-layer.md`).
- Grid Workflow RPC helper usage guide (`../grid/sdk/workflowrpc/README.md`).

## Verification

- **2025-10-01** — Validated helper-backed submissions and streaming: `go test
  ./internal/workflow/grid` exercises submit payload construction plus terminal
  status streaming, while helper unit tests cover bearer token propagation,
  retry behaviour on 5xx responses, and context cancellation.
- **2025-09-30** — Exercised JetStream helper tests to confirm
  `webhook.<tenant>.ploy.workflow-ticket` bindings resolve stream names and
  honour consumer reuse; verified subject derivation trims whitespace before
  formatting.
- **2025-09-27** — Confirmed RPC routes are served at `/v1/workflows/rpc/runs*`
  in `../grid/internal/httpapi/workflow_rpc_routes.go`.
- **2025-09-27** — Confirmed SDK client targets the same endpoints and payload
  schema in `../grid/sdk/workflowrpc/go/client.go`.
- **2025-09-27** — Reviewed helper builder coverage in
  `../grid/sdk/workflowrpc/helper` tests to ensure retry semantics align with
  Ploy needs.

## Tests

- Unit tests in `internal/workflow/grid/client_test.go` covering submit payload
  construction, streaming terminal events, and error propagation.
- Runner integration test ensuring job spec carries lane metadata and cache key.
- Contract tests covering subject derivation (including whitespace trimming) and
  build-gate log retrieval once statuses move to `jobs.<run_id>.events`.
- Helper adoption tests validating Ploy wiring against the new helper layer
  (auth header, retry semantics, CLI factory injection).
- Continue RED → GREEN → REFACTOR: fail workflow RPC tests first, introduce
  minimal helper wiring, then refactor after coverage remains steady.

## Roadmap Tasks

- [x] `docs/tasks/workflow-rpc-alignment/01-grid-sdk-client.md`
- [x] `docs/tasks/workflow-rpc-alignment/02-runner-job-spec.md`
- [x] `docs/tasks/workflow-rpc-alignment/03-subject-alignment.md`
- [x] `docs/tasks/workflow-rpc-alignment/04-helper-adoption.md`

## Completion Criteria

- Ploy submits workflow runs via the SDK, producing valid `JobSpec` payloads and
  receiving status streams from Grid.
- JetStream subjects in contracts and documentation match the Grid
  implementation.
- Build gate log retrieval and knowledge-base integration operate on the updated
  subject scheme.
- Documentation (design index, CLI, build gate) reflects the final behaviour and
  dates.
