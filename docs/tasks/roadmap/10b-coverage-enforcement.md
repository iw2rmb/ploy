# Coverage Enforcement & Quality Gates

## Why

- The repository commitment of ≥60% overall coverage (≥90% for critical workflow runner packages) only protects quality if tooling fails fast when thresholds slip.
- Surfacing structured coverage and lint results in CI artifacts keeps contributors accountable and enables release managers to review trends without rerunning jobs.

## Required Changes

- Instrument `go test -cover ./...` (and any supplementary coverage tools) to emit combined reports, failing the build automatically when documented thresholds are not met.
- Extend the CI pipeline to publish coverage summaries, per-package deltas, and Markdown lint results alongside build artifacts.
- Add a developer-friendly wrapper (e.g., `make coverage`) that mirrors the CI enforcement locally and prints guidance for remediating shortfalls.
- Document the coverage workflow, expected thresholds, and reporting locations within `docs/v2/testing.md` so contributors understand the gating logic.

## Definition of Done

- Local and CI runs fail immediately when overall or critical-package coverage dips below the documented targets.
- Coverage and Markdown lint reports are archived in CI with stable paths, enabling downstream tooling to flag regressions.
- Contributors can run `make coverage` (or the documented equivalent) to reproduce CI failures and obtain remediation steps.

## Tests

- Meta-test or script that exercises the coverage wrapper against sample packages, asserting the command exits non-zero when thresholds are intentionally undercut.
- Unit tests for any new reporting utilities that parse coverage profiles or lint output to ensure edge cases (missing files, malformed reports) surface actionable errors.
- Pipeline-level check that verifies coverage artifacts upload successfully and includes a smoke assertion for the Markdown lint step.
