# Local Test Harness Unification

## Why

- `docs/v2/testing.md` requires developers to execute unit, integration, and timeout-guarded suites locally; the current patchwork of scripts makes the guidance hard to follow.
- SHIFT smoke checks and CLI acceptance suites must run together ahead of Grid usage so regressions are caught during workstation development.

## Required Changes

- Expand `make test` (and any supporting scripts under `scripts/`) into a consolidated harness that sequences Go unit tests, SHIFT smoke checks, and CLI acceptance suites without manual orchestration.
- Add feature flags or env toggles so contributors can focus on subsets (e.g., `SHIFT_ONLY`, `ACCEPTANCE_ONLY`) while keeping the default run exhaustive.
- Ensure prerequisite setup (mod fixtures, mocked services, local cache directories) is automated or validated up front with actionable error messages.
- Emit structured PASS/FAIL summaries and surface pointers to troubleshooting docs when individual phases fail.

## Definition of Done

- Running `make test` on a clean workstation executes the entire suite deterministically, providing clear pass/fail outcomes and links to remediation guidance.
- SHIFT smoke checks and CLI acceptance suites run as part of the default harness invocation without additional manual steps.
- Harness logs include timestamps, phase boundaries, and exit codes, enabling quick triage in both local and CI runs.

## Tests

- End-to-end smoke test that invokes the harness in CI and asserts all phases run, collecting artifacts from each stage.
- Unit tests for any new harness orchestration helpers (e.g., env validation, phase sequencing) to confirm they fail fast on missing prerequisites.
- Regression test that simulates a failing SHIFT suite and verifies the harness reports the failure with actionable messaging.
