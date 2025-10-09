# Build Gate – Java Error Prone Adapter (Roadmap 21)

## Status

- [x] Completed 2025-09-29 — Java Error Prone static check adapter implemented
      with registry wiring, CLI summary coverage, and tests (`go test ./...`).

## Purpose

Deliver a Java-focused static analysis adapter so the build gate stage surfaces
Error Prone diagnostics alongside existing Go coverage. This keeps the CLI
summaries, checkpoint metadata, and knowledge base ingest aligned for polyglot
repositories.

## Background

- `docs/design/build-gate/README.md` established the multi-language adapter
  strategy but only the Go adapter has shipped so far.
- `docs/tasks/build-gate/03-static-check-registry.md` calls out Java Error Prone as
  a follow-up adapter to populate the registry.
- `cmd/ploy/mod_run_output_test.go` and corresponding knowledge base wiring
  already expect multi-tool output.
- `internal/workflow/buildgate/static_checks_registry.go` normalises adapter
  results, so a dedicated Error Prone adapter can plug in without further
  orchestration changes.

## Goals

1. Implement an Error Prone adapter that wraps `javac -Xplugin:ErrorProne` (or
   `error_prone_runner`) while normalising diagnostics into `StaticCheckFailure`
   entries.
2. Register the adapter under the canonical `java` language alias with severity
   defaults aligned to the registry contract.
3. Extend the CLI summary and knowledge base flows to display Error Prone
   findings via the shared metadata pipeline.
4. Provide manifest/lane options so repositories can scope packages (targets)
   and pass additional Error Prone flags.

## Non-Goals

- Expand beyond workstation execution (Grid remote execution remains out of
  scope for this slice).
- Add adapters for ESLint, Ruff, or Roslyn (tracked as separate follow-ups).
- Implement Error Prone custom rule configuration; only pass-through flag
  support is planned.

## Proposed Changes

1. Create `internal/workflow/buildgate/error_prone_adapter.go` implementing
   `StaticCheckAdapter` with command runner injection for tests.
2. Parse Error Prone output (compiler-style) into structured failures capturing
   file, line, column, rule, severity (`ERROR`, `WARNING`), and message.
3. Add alias handling for `java`/`javac` in `normalizeLanguage` and update
   registry defaults.
4. Update registry construction helpers/tests to register the adapter and verify
   manifest overrides.
5. Extend CLI/workflow tests to cover Error Prone failures appearing in the
   build gate summary.

## Implementation Plan

- **Adapter**: Shell out to the configured Java binary with
  `-Xplugin:ErrorProne` and optional `-Xep`, `-XepDisableAllChecks`,
  `-Xep:Check=LEVEL`, `-classpath`, and `--patch-module` options surfaced
  through manifest overrides.
- **Options Handling**: Support manifest options `targets`, `classpath`,
  `flags`, and `severity` where `severity` defaults to `error`.
- **Parsing**: Error Prone emits diagnostics matching
  `path:line:column: [severity] message [RuleName]`. Implement a parser
  resilient to multi-line messages by buffering until the next diagnostic
  prefix.
- **Registry Wiring**: Register the adapter with default severity `error` and
  ensure skip/override behaviour matches existing registry semantics.
- **Testing**: Add focused unit tests for the adapter (command execution,
  parsing variants, severity mapping, options) and extend CLI summary tests with
  representative Error Prone output.

## Dependencies

- `docs/design/build-gate/README.md` — overarching build gate architecture.
- `internal/workflow/buildgate/static_checks_registry.go` — adapter registry
  integration point.
- `internal/workflow/buildgate/static_checks_types.go` — shared adapter
  interfaces and severity constants.
- `cmd/ploy/mod_run_output_test.go` — CLI summary expectations that must
  now include Java diagnostics.
- Upstream Error Prone tooling documentation
  (`https://errorprone.info/docs/installation`) referenced for flag behaviour.

## Risks & Mitigations

- **Compiler Invocation Variance**: Error Prone requires specific command
  layouts; mitigate via configurable `java` binary and explicit flag ordering
  tests.
- **Diagnostic Format Drift**: Error Prone output can change; include fixtures
  covering common patterns and guard with tests.
- **Performance**: Running Error Prone on large targets may be slow; expose
  manifest `targets` option to scope analysis.
- **Environment Requirements**: Require that `JAVA_HOME`/`error_prone_core`
  exist; document fallback behaviour (surface explicit error when binaries are
  unavailable).

## Test Strategy

- Unit tests for the adapter covering successful execution, option propagation,
  stderr parsing, warning vs error severity, and handling of command failures.
- Registry tests asserting the adapter registers under `java`, honours skip
  lists, and passes manifest options.
- CLI summary tests ensuring Error Prone failures render correctly in the build
  gate summary output.
- Repository-wide `go test ./...` to confirm no regressions.
- Preserve RED → GREEN → REFACTOR cadence: start with failing Error Prone tests,
  add minimal adapter wiring, then refactor once coverage is stable.

## Deliverables

- `internal/workflow/buildgate/error_prone_adapter.go` plus tests.
- Updated registry helpers/aliases.
- Documentation updates: this design doc (marked complete), build gate design
  status, design index, roadmap task, README feature highlight verification
  note, and changelog entry with date and verification details.
- Roadmap task `docs/tasks/build-gate/08-error-prone-adapter.md` recording scope,
  DOD, and verification commands.
