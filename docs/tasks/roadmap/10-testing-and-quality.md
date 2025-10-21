# Testing & Quality Gates

## Why
- `docs/v2/testing.md` defines unit, integration, and timeout expectations for the v2 stack, emphasizing workstation execution.
- Ensuring ≥60% overall coverage (≥90% for critical workflow runner packages) and RED → GREEN → REFACTOR cadence avoids regressions while removing Grid dependencies.

## Required Changes
- Establish a consolidated test harness (`make test`) that runs unit tests, SHIFT smoke checks, and CLI acceptance suites locally.
- Configure coverage tooling to fail builds below documented thresholds, surfacing reports in CI artifacts.
- Implement timeout guards and retry logic for long-running Mods to prevent workstation hangs.
- Document test planning templates, including environment variable requirements from `docs/envs/README.md`.

## Definition of Done
- Developers can execute the full test suite locally with deterministic results and clear failure guidance.
- CI pipelines enforce coverage, linting (including `make lint-md`), and long-running test timeouts.
- Contribution docs reference the updated test strategy and workflows for new slices.

## Tests
- Meta-tests verifying coverage thresholds, lint commands, and workspace setup scripts.
- Integration tests that run representative Mods end-to-end under controlled timeouts.
- Regression suites simulating SHIFT failures and ensuring descriptive CLI output.
