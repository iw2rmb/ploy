# Testing CI Reporting

## Why
- CI pipelines must surface coverage trends, regressions, and unstable tests quickly.
- Developers need visibility into flake frequency and coverage deltas per commit.

## What to do
- Publish coverage reports and delta summaries in CI output and artifacts.
- Track unstable tests, annotating flaky runs with alerts or quarantines.
- Align reporting inputs with local gates from [`../testing-coverage-gates/README.md`](../testing-coverage-gates/README.md) and integration metrics from [`../testing-integration-harness/README.md`](../testing-integration-harness/README.md).

## Where to change
- [`.github/workflows`](../../../.github/workflows) YAML files to add reporting steps.
- [`scripts`](../../../scripts) for coverage diff generation and flake tracking utilities.
- [`docs/v2/testing.md`](../../v2/testing.md) to explain CI dashboards and regression alerts.

## COSMIC evaluation
| Functional process                              | E | X | R | W | CFP |
|-------------------------------------------------|---|---|---|---|-----|
| Publish CI coverage trends and instability data | 1 | 1 | 1 | 0 | 3   |
| **TOTAL**                                       | 1 | 1 | 1 | 0 | 3   |

- Assumption: CI artifacts store coverage reports; no extra persistent datastore needed.
- Open question: confirm flake tracking uses existing retry logs or needs new format.

## How to test
- Trigger CI workflow run, verify coverage artifacts upload and delta summaries display.
- Unit tests for reporting scripts if pure Go or shell.
- Manual review: inspect CI dashboard to ensure flaky test annotations render correctly.
