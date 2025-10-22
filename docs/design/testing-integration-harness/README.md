# Testing Integration Harness

## Why
- Integration suites must exercise end-to-end Mods flows (scheduler → runtime → artifacts → GC).
- Failures in integration harness should surface before release to avoid regressions.

## What to do
- Extend `tests/integration` to run representative Mods, covering log streaming, SHIFT retries, artifact publishing, and GC interactions.
- Provide fixtures for IPFS, etcd, and node simulators to keep runs deterministic.
- Feed GC outcomes into metrics per [`../gc-audit-metrics/README.md`](../gc-audit-metrics/README.md).

## Where to change
- [`tests/integration`](../../../tests/integration) to add scenarios and fixtures.
- [`internal/controlplane`](../../../internal/controlplane) mocks for scheduler/runtime coordination.
- [`tests/e2e/README.md`](../../../tests/e2e/README.md) to document harness usage and future Grid alignment.

## COSMIC evaluation
| Functional process     | E | X | R | W | CFP |
|------------------------|---|---|---|---|-----|
| Run integration harness| 1 | 1 | 1 | 1 | 4   |
| **TOTAL**              | 1 | 1 | 1 | 1 | 4   |

- Assumption: harness stores artifacts in temporary dirs cleaned post-run.
- Open question: confirm SHIFT failure loops need additional instrumentation hooks.

## How to test
- `make test` ensuring integration suites execute via tags or dedicated target.
- Manual run: `go test ./tests/integration -run TestModsFlow` verifying end-to-end success.
- Capture logs/artifacts during run and validate retention per GC design docs.
