# Build Gate Error Prone Adapter

- [x] Completed (2025-09-29)

## Why / What For

Extend build gate static analysis coverage to Java by wiring Error Prone
diagnostics into checkpoint metadata and the CLI summary so polyglot repos
receive actionable feedback during workstation runs.

## Required Changes

- Implement an Error Prone-backed static check adapter and register it under the
  `java` language in the static check registry.
- Add manifest/lane configuration hooks for Error Prone options (targets,
  classpath, custom flags, severity overrides).
- Parse Error Prone output into structured `StaticCheckFailure` entries with
  rule identifiers and severity information.
- Ensure CLI summaries and knowledge base ingestion surfaces Error Prone
  findings alongside existing Go coverage.
- Update design docs, roadmap, changelog, and verification logs per repository
  workflow.

## Definition of Done

- Error Prone adapter executes with configurable options and reports failures
  via `StaticCheckReport`.
- Build gate registry includes the adapter with language aliases for
  `java`/`javac` and respects skip/manifest overrides.
- CLI build gate summary prints Error Prone findings and tags them with rule IDs
  and severities.
- Documentation (`docs/design/build-gate/error-prone/README.md`, design index,
  build gate design) reflects the shipped adapter and references verification
  dates.
- `go test ./...` passes with new adapter tests covering execution, parsing, and
  CLI summary output.

## Implementation Notes

- Prefer invoking `javac` with `-Xplugin:ErrorProne` while allowing manifests to
  supply an alternate binary or wrapper script.
- Maintain deterministic output parsing by normalising file paths and handling
  multi-line diagnostics.
- Surface command execution errors distinctly when no diagnostics are produced
  to avoid silent failures.
- Reuse the existing command runner abstraction to support test doubles.

## Tests

- Unit tests for the Error Prone adapter
  (`internal/workflow/buildgate/error_prone_adapter_test.go`).
- Registry coverage ensuring manifest overrides/skip lists interact correctly
  with the new adapter.
- CLI summary test cases rendering Error Prone failures in
  `cmd/ploy/workflow_run_output_test.go`.
- `go test ./...` verification recorded in the changelog and roadmap once GREEN.

## References

- Design plan (`docs/design/build-gate/error-prone/README.md`).
- Build gate architecture (`docs/design/build-gate/README.md`).
- Static check registry design
  (`roadmap/build-gate/03-static-check-registry.md`).
