# Build Gate Reboot (Roadmap 21)

## Purpose

Reintroduce the Mods build gate with modern Grid integration, static analysis,
and intelligent log parsing. We recover behaviour from commit `3b11d7e8`—which
emitted builder logs, sandbox builds, and healing retries—while migrating
execution to Grid stages and expanding language-specific static checks (e.g.,
Google Error Prone for Java). The reboot ensures every Mods plan validates code
quality before changes exit the workstation.

## Scope

- Execute build and static analysis stages on Grid via Workflow RPC jobs while
  retaining a deterministic workstation sandbox for RED-phase testing.
- Parse build outputs into structured metadata consumed by Knowledge Base and
  Mods healing flows.
- Keep lane definitions aligned with the Workflow RPC job spec schema (`image`,
  `command`, `env`, `resources`).

## Current Status (2025-10-07)

- Stage scheduling and checkpoint metadata wiring landed via
  `docs/tasks/build-gate/01-stage-planning-and-metadata.md`.
- Sandbox runner landed via `docs/tasks/build-gate/02-sandbox-runner.md`, providing
  structured duration/cache metadata and timeout handling for workstation tests.
- Static check adapter registry shipped under
  `internal/workflow/buildgate/static_checks.go` (see
  `docs/tasks/build-gate/03-static-check-registry.md`), wiring lane defaults,
  manifest overrides, and skip hooks into `StaticCheckRegistry`.
- Go vet adapter exposed through `internal/workflow/buildgate.NewGoVetAdapter`
  (see `docs/tasks/build-gate/05-go-vet-adapter.md`), parsing workstation/Grid
  diagnostics into `StaticCheckFailure` entries with manifest-configurable
  package and tag options.
- Error Prone adapter wired via
  `internal/workflow/buildgate.NewErrorProneAdapter` (see
  `docs/tasks/build-gate/08-error-prone-adapter.md`), enabling Java diagnostics
  with manifest-configurable targets/classpaths and CLI summary coverage
  (shipped 2025-09-29).
- ESLint adapter wired via `internal/workflow/buildgate.NewESLintAdapter` (see
  `docs/tasks/build-gate/09-eslint-adapter.md`), enabling JavaScript diagnostics
  with manifest-configurable targets/config/rule overrides and CLI summary
  coverage (shipped 2025-09-29).
- Log retrieval and Grid artifact ingestion shipped via
  `internal/workflow/buildgate.LogRetriever` and `LogIngestor` (see
  `docs/tasks/build-gate/04-log-retrieval-and-grid-integration.md`), normalising
  Knowledge Base findings and surfacing digests in build gate metadata.
- Build gate runner orchestrates sandbox execution, static checks, and log
  ingestion through `internal/workflow/buildgate.Runner` (see
  `docs/tasks/build-gate/06-build-gate-runner.md`), emitting sanitised metadata for
  checkpoint publication.
- CLI build gate summary surfaces static checks and knowledge base guidance
  after workflow execution (see `docs/tasks/build-gate/07-cli-summary.md`).

- Establish `internal/workflow/buildgate` hosting sandbox compilation, static
  check orchestration, and log parsing utilities.
- Align build execution with Grid workflow stages (`build-gate`,
  `static-checks`) triggered from Mods and general workflow runs.
- Integrate with the Knowledge Base slice for error classification and
  remediation suggestions when builds fail.
- Replace legacy controller HTTP dependencies with Grid RPC
  (`grid.workflow.Submit`) and JetStream artifact retrieval.

## Background & Prior Art

- The legacy implementation in commit `3b11d7e8` executed sandbox builds via
  `build.NewSandboxService()` and fetched logs from controller endpoints. We
  retain the deterministic sandbox step and builder log enrichment, now sourcing
  logs from Grid artifact streams (`ploy.artifact.<ticket>`) and status events.
- Previous static analysis was ad hoc per lane. This design formalises
  per-language adapters (Java Error Prone, Go `go vet` + `staticcheck`, Node
  ESLint, Python `ruff`, C# Roslyn analyzers) with configurable severity
  thresholds.

## Behaviour

- `build-gate` stage runs inside Grid using the lane-specified container image.
  The stage compiles the repository, executes tests flagged by the plan, and
  archives `builder.log` as a structured artifact envelope.
- Immediately after compilation, the runner invokes language-specific static
  check adapters. Failures produce structured error reports (JSON with rule ID,
  file, line, message) attached to checkpoint metadata.
- Log parsing examines `builder.log` for known error signatures (compiler
  errors, dependency resolution issues) and emits normalized error codes for
  Knowledge Base matching using the default log parser patterns.
- Successful runs propagate build version metadata and artifact digests back to
  the workflow checkpoint. Failures trigger healing retries or escalate to
  `human-in-the-loop` depending on Mods planner guidance.

### Static Check Registry

- `StaticCheckRegistry` indexes adapters by language/tool metadata while
  retaining default severity thresholds so workstation and Grid runs stay
  aligned.
- Lane defaults enable adapters per language via `StaticCheckLaneConfig`,
  defining baseline failure thresholds (`SeverityError`, `SeverityWarning`,
  `SeverityInfo`).
- Repo manifests override defaults using
  `StaticCheckManifest.Languages[<lang>]`, toggling adapters on/off, adjusting
  thresholds, or forwarding adapter options. CLI overrides populate
  `StaticCheckSpec.SkipLanguages` for per-run skips.
- Registry execution emits normalized `StaticCheckReport` entries consumed by
  metadata sanitisation and checkpoint publication.

### Go Vet Adapter

- `buildgate.NewGoVetAdapter` wraps `go vet`, mapping aliases such as `go` and
  `golang` onto the canonical registry key while keeping defaults portable
  across workstation and Grid lanes.
- `StaticCheckRequest.Options["packages"]` defaults to `./...` but supports
  whitespace/comma separated overrides for repos that need to restrict analysis
  to select modules.
- Optional `StaticCheckRequest.Options["tags"]` forwards build tags so `go vet`
  runs match the manifest-defined test matrix without diverging from lane
  defaults.
- Parsed diagnostics populate `StaticCheckFailure` rows tagged with the `govet`
  rule identifier, normalized severity (`error`), and cleaned file/position
  metadata for checkpoint publication.

## Implementation Notes

- `buildgate.SandboxRunner` wraps the deterministic sandbox build logic and
  exposes structured results (duration, cache hit, failure reason/detail) for
  workstation tests. The runner keeps RED-phase tests deterministic while
  production builds continue to execute inside Grid stages.
- Implement a `StaticCheckRegistry` mapping languages to adapters. Each adapter
  runs inside the same Grid job to minimise cold starts and reads configuration
  from repo manifests (e.g., `.errorprone`, `.eslintrc`).
- Lane defaults (`StaticCheckLaneConfig`) provide baseline enablement and
  severity thresholds, while manifest overrides (`StaticCheckManifest`) and CLI
  flags (`--build-gate-skip=<lang>`) merge through `StaticCheckSpec` before
  adapters execute.
- Go vet adapter normalises `go`/`golang` aliases and honours manifest or CLI
  overrides for `packages` (default `./...`) and `tags` so repos can focus the
  analysis scope without editing lane defaults.
- Adapt log enrichment to pull artifacts from Grid: upon failure, inspect
  `jobs.<run_id>.events` for the stage, find the artifact CID, and retrieve logs
  via Grid (`LogSourceGrid`) with an IPFS fallback (`LogSourceIPFS`). Provide
  fallbacks to JetStream attachments in workstation mode via the retriever
  abstraction.
- Log ingestion passes retrieved content to the default parser, mapping
  canonical Knowledge Base codes (e.g., Git auth failures, Go module conflicts,
  linker resolution issues, disk pressure) onto `Metadata.LogFindings` for
  checkpoint publication and CLI summaries.
- Emit checkpoint metadata fields (`build_gate.static_checks`,
  `build_gate.log_digest`) so downstream tooling (Knowledge Base, telemetry) can
  reason about results.
- Provide CLI options to toggle static checks per language and to set failure
  thresholds (`--build-gate-fail-on-warning`, `--build-gate-skip=golang`).
- Ensure compatibility with Mods planner by exposing a
  `buildgate.Run(ctx, spec)` API returning structured outcomes consumed by
  healing logic.
- `buildgate.Runner` wraps the sandbox runner, static check registry, and log
  ingestor, applying metadata sanitisation and preferring ingested log
  digests/findings when available.

## Clarifications (2025-09-27)

- “Sandbox” in this document refers to the deterministic workstation harness
  used for RED-phase unit tests. Live build and static-check execution stays on
  Grid; there is no additional sandbox infrastructure or per-build VM beyond the
  Grid runtime adapters.
- Build jobs must populate the Workflow RPC job spec schema (`image`, `command`,
  `env`, `resources`) before dispatch so Grid accelerators and cache reuse
  function correctly.

## References

- Grid Workflow RPC design for build stage submission
  (`../grid/docs/design/workflow-rpc/README.md`).
- Grid Workflow RPC helper guide for streaming/log retrieval utilities
  (`../grid/sdk/workflowrpc/README.md`).
- Grid log streaming design covering artifact publication
  (`../grid/docs/design/log-streaming/README.md`).
- Ploy Workflow RPC alignment design for job payload requirements
  (`../workflow-rpc-alignment/README.md`).

## Verification (2025-09-27)

- Confirmed `jobs.<run_id>.events` are emitted with artifact metadata in
  `../grid/internal/jobs/publisher_jetstream.go`.
- Verified current Ploy build gate metadata sanitisation in
  `internal/workflow/buildgate/metadata.go` aligns with the documented schema.

## Tests

- Unit tests for sandbox runner covering timeout handling, cache reuse, and
  structured result mapping, using fake Grid adapters in the workstation stub.
- Static check registry unit tests exercising lane/manifest reconciliation, skip
  overrides, and severity thresholds with fake adapters
  (`internal/workflow/buildgate/static_checks_test.go`).
- Adapter-specific tests now cover Go
  (`internal/workflow/buildgate/go_vet_adapter_test.go`), Java Error Prone
  (`internal/workflow/buildgate/error_prone_adapter_test.go`), and JavaScript
  ESLint (`internal/workflow/buildgate/eslint_adapter_test.go`); Ruff and Roslyn
  fixtures remain follow-ups.
- Log ingestion tests covering artifact fallback precedence, truncation limits,
  parser pattern coverage, and metadata sanitisation via
  `internal/workflow/buildgate/log_retriever_test.go`, `log_parser_test.go`,
  `log_ingestion_test.go`, and updated metadata sanitiser coverage.
- Log parser tests feeding historical logs (retrieved from legacy commits) to
  confirm normalization of compiler errors into Knowledge Base-ready codes.
- Workflow runner tests ensuring Grid artifact retrieval works in both stub and
  live JetStream modes.
- Maintain ≥90% coverage across `internal/workflow/buildgate` and associated
  adapters.
- Reinforce RED → GREEN → REFACTOR: land failing sandbox/static-check tests
  first, add minimal adapter wiring, then refactor once coverage targets stay
  green.

## Rollout & Follow-ups

- Add roadmap entries under `docs/tasks/build-gate/` covering sandbox port, static
  check adapters, log parser, and Grid integration.
- Coordinate with Grid maintainers (see `../grid/README.md`) to provision
  dedicated lanes for static analysis binaries and to expose artifact envelopes
  required for log retrieval.
- ESLint adapter slice (`docs/design/build-gate/eslint/README.md`,
  `docs/tasks/build-gate/09-eslint-adapter.md`) is now complete; continue
  prioritising Ruff and Roslyn adapters once workstation coverage expands.
