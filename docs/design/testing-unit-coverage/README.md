# Testing Unit Coverage

## Why
- Global rules require ≥60% overall coverage and ≥90% on workflow runner packages.
- Unit tests must focus on error paths, retries, and edge cases to stabilize releases.

## What to do
- Expand unit tests across control plane, runtime, and CLI packages targeting critical error handling.
- Track coverage deltas per package and document gaps for follow-up design docs.
- Coordinate with integration efforts in [`../testing-integration-harness/README.md`](../testing-integration-harness/README.md).

## Where to change
- [`internal/controlplane`](../../../internal/controlplane) packages for new table-driven tests.
- [`internal/workflow/runtime`](../../../internal/workflow/runtime) to cover retry logic.
- [`cmd/ploy`](../../../cmd/ploy) CLI packages for error-case tests.
- Coverage reporting in [`Makefile`](../../../Makefile) if needed for per-package metrics.

## COSMIC evaluation
| Functional process           | E | X | R | W | CFP |
|------------------------------|---|---|---|---|-----|
| Expand unit test suites      | 1 | 1 | 1 | 1 | 4   |
| **TOTAL**                    | 1 | 1 | 1 | 1 | 4   |

- Assumption: new tests reuse existing fixtures; no new integration harness required.
- Open question: confirm workflow runner critical packages list matches coverage threshold requirement.

## How to test
- `make test` ensuring `go test -cover ./...` passes with increased coverage.
- Use `go test -coverpkg=...` for critical packages to validate ≥90% targets.
- Document coverage reports in CI artifacts for traceability.
