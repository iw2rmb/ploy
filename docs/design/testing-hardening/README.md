# Testing Hardening

## Why
- Ploy commits to ≥60% overall test coverage and ≥90% on workflow runner packages per global workflow rules.
- Integration and smoke tests must exercise end-to-end Mods flows (scheduler → runtime → artifacts → GC) before v2 ships.
- CI should detect regressions quickly, highlighting coverage deltas and unstable tests.

## What to do
- Expand unit tests across control plane, runtime, and CLI packages focusing on error paths, retries, and edge cases.
- Build integration suites that run representative Mods (OpenRewrite sample) covering log streaming, SHIFT failure loops, artifact publishing, and GC interactions.
- Wire coverage gates into `make test` and CI pipelines, enforcing thresholds and reporting coverage trends.

## Where to change
- [`tests/integration`](../../../tests/integration) to extend harnesses for control plane + node runs with SHIFT stubs and IPFS fixtures.
- [`cmd/ploy`](../../../cmd/ploy), [`internal/controlplane`](../../../internal/controlplane), [`internal/workflow/runtime`](../../../internal/workflow/runtime), and related packages to add table-driven unit tests.
- CI configuration ([`.github/workflows`](../../../.github/workflows)/*.yml or equivalent) and the [`Makefile`](../../../Makefile) targets to enforce coverage gates and publish reports.

## COSMIC evaluation
| Functional process          | E | X | R | W | CFP |
|-----------------------------|---|---|---|---|-----|
| Enforce coverage thresholds | 1 | 1 | 2 | 1 | 5   |
| Run integration harness     | 1 | 1 | 1 | 1 | 4   |
| Expand unit test suites     | 1 | 1 | 1 | 1 | 4   |
| **TOTAL**                   | 3 | 3 | 4 | 3 | 13  |

- Assumptions: counts exclude flaky-test triage flows and treat coverage reporting as a single write to CI status.
- Open questions: confirm whether artifact uploads during integration runs add additional writes beyond the harness report.

## How to test
- `make test` to ensure `go test -cover ./...` meets thresholds and new suites pass.
- Integration CI job executing end-to-end Mods runs; validate logs and artifacts persist as expected.
- Manual audit: run `go tool cover -html=coverage.out` or equivalent to confirm critical packages meet 90% coverage.
