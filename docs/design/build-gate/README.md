# Build Gate Reboot (Roadmap 21)

## Purpose
Reintroduce the Mods build gate with modern Grid integration, static analysis, and intelligent log parsing. We recover behaviour from commit `3b11d7e8`—which emitted builder logs, sandbox builds, and healing retries—while migrating execution to Grid stages and expanding language-specific static checks (e.g., Google Error Prone for Java). The reboot ensures every Mods plan validates code quality before changes exit the workstation.

## Scope
- Establish `internal/workflow/buildgate` hosting sandbox compilation, static check orchestration, and log parsing utilities.
- Align build execution with Grid workflow stages (`build-gate`, `static-checks`) triggered from Mods and general workflow runs.
- Integrate with the Knowledge Base slice for error classification and remediation suggestions when builds fail.
- Replace legacy controller HTTP dependencies with Grid RPC (`grid.workflow.Submit`) and JetStream artifact retrieval.

## Background & Prior Art
- The legacy implementation in commit `3b11d7e8` executed sandbox builds via `build.NewSandboxService()` and fetched logs from controller endpoints. We retain the deterministic sandbox step and builder log enrichment, now sourcing logs from Grid artifact streams (`ploy.artifact.<ticket>`) and status events.
- Previous static analysis was ad hoc per lane. This design formalises per-language adapters (Java Error Prone, Go `go vet` + `staticcheck`, Node ESLint, Python `ruff`, C# Roslyn analyzers) with configurable severity thresholds.

## Behaviour
- `build-gate` stage runs inside Grid using the lane-specified container image. The stage compiles the repository, executes tests flagged by the plan, and archives `builder.log` as a structured artifact envelope.
- Immediately after compilation, the runner invokes language-specific static check adapters. Failures produce structured error reports (JSON with rule ID, file, line, message) attached to checkpoint metadata.
- Log parsing examines `builder.log` for known error signatures (compiler errors, dependency resolution issues) and emits normalized error codes for Knowledge Base matching.
- Successful runs propagate build version metadata and artifact digests back to the workflow checkpoint. Failures trigger healing retries or escalate to `human-in-the-loop` depending on Mods planner guidance.

## Implementation Notes
- Create `buildgate.SandboxRunner` to wrap the existing sandbox build logic and expose structured results. It should reuse deterministic caching and keep compatibility with prior unit tests.
- Implement a `StaticCheckRegistry` mapping languages to adapters. Each adapter runs inside the same Grid job to minimise cold starts and reads configuration from repo manifests (e.g., `.errorprone`, `.eslintrc`).
- Adapt log enrichment to pull artifacts from Grid: upon failure, query `grid.status.<ticket>` for the stage, find the artifact CID, and retrieve logs via IPFS. Provide fallbacks to JetStream attachments in workstation mode.
- Emit checkpoint metadata fields (`build_gate.static_checks`, `build_gate.log_digest`) so downstream tooling (Knowledge Base, telemetry) can reason about results.
- Provide CLI options to toggle static checks per language and to set failure thresholds (`--build-gate-fail-on-warning`, `--build-gate-skip=golang`).
- Ensure compatibility with Mods planner by exposing a `buildgate.Run(ctx, spec)` API returning structured outcomes consumed by healing logic.

## Tests
- Unit tests for sandbox runner covering timeout handling, cache reuse, and structured result mapping, using fake Grid adapters in the workstation stub.
- Adapter-specific tests verifying each static check tool surfaces expected diagnostics for failing fixtures (Java error-prone sample, Go vet fixture, ESLint error, etc.).
- Log parser tests feeding historical logs (retrieved from legacy commits) to confirm normalization of compiler errors into Knowledge Base-ready codes.
- Workflow runner tests ensuring Grid artifact retrieval works in both stub and live JetStream modes.
- Maintain ≥90% coverage across `internal/workflow/buildgate` and associated adapters.

## Rollout & Follow-ups
- Add roadmap entries under `roadmap/build-gate/` covering sandbox port, static check adapters, log parser, and Grid integration.
- Coordinate with Grid maintainers (see `../grid/README.md`) to provision dedicated lanes for static analysis binaries and to expose artifact envelopes required for log retrieval.
- Future slice: surface build gate results in the CLI summary with actionable remediation links from the Knowledge Base.
