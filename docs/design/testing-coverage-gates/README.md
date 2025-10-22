# Testing Coverage Gates

## Why
- Local `make test` must fail fast when coverage thresholds drop below policy.
- Developers require clear feedback on which packages trigger failures.

## What to do
- Wire coverage gates into `make test`, enforcing ≥60% overall and ≥90% for workflow runner packages.
- Emit actionable error messages referencing coverage gaps and upstream docs.
- Align with CI enforcement described in [`../testing-ci-reporting/README.md`](../testing-ci-reporting/README.md).

## Where to change
- [`Makefile`](../../../Makefile) to add coverage thresholds.
- [`scripts`](../../../scripts) for helper tooling (e.g., coverage diff parser).
- [`docs/v2/testing.md`](../../v2/testing.md) updating developer guidance.
- Reference coverage baselines stored in CI configuration under [`.github/workflows`](../../../.github/workflows).

## COSMIC evaluation
| Functional process                      | E | X | R | W | CFP |
|-----------------------------------------|---|---|---|---|-----|
| Wire local coverage gates into `make test` | 1 | 1 | 1 | 1 | 4 |
| **TOTAL**                               | 1 | 1 | 1 | 1 | 4 |

- Assumption: coverage tool outputs `coverage.out`; gates parse this format.
- Open question: confirm workflow runner package list stays statically defined or generated.

## How to test
- `make test` to ensure gates trigger on intentional coverage drops.
- Unit tests for coverage parsing helpers under `scripts`.
- Manual verification: adjust coverage thresholds temporarily to ensure failure paths fire.
