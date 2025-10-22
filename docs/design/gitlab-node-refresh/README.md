# GitLab Node Refresh

## Why
- Worker nodes must refresh GitLab tokens before expiry so repository clones and MR automation stay online.
- Bootstrap needs mutual TLS with the signer to avoid storing long-lived secrets on disk.

## What to do
- Extend node bootstrap to request tokens from the signer, refresh them proactively, and flush caches when rotations fire.
- Add health metrics and debug logging so operators can trace token expiry and refresh failures.
- Reference upstream contract in [`../gitlab-token-signer/README.md`](../gitlab-token-signer/README.md).

## Where to change
- [`internal/node/bootstrap`](../../../internal/node/bootstrap) for signer handshake, refresh loop, and cache eviction.
- [`internal/node/git`](../../../internal/node/git) or equivalent helpers to inject refreshed credentials during Git operations.
- [`internal/metrics`](../../../internal/metrics) (if present) to emit refresh success/failure counters.
- Upstream docs: [`../gitlab-token-signer/README.md`](../gitlab-token-signer/README.md) and [`../gitlab-rotation-revocation/README.md`](../gitlab-rotation-revocation/README.md).

## COSMIC evaluation
| Functional process            | E | X | R | W | CFP |
|-------------------------------|---|---|---|---|-----|
| Refresh node credential cache | 1 | 1 | 1 | 1 | 4   |
| **TOTAL**                     | 1 | 1 | 1 | 1 | 4   |

- Assumption: refresh loop stores tokens only in memory; no disk persistence.
- Open question: confirm node bootstrap can retry signer handshake without blocking startup.

## How to test
- `go test ./internal/node/bootstrap -run TestGitLabRefresh` covering handshake, rotation signals, and cache flushes.
- Integration: launch node with fake signer, rotate token, ensure refresh before expiry and log flush.
- Metrics smoke: scrape exporter to confirm refresh success/failure counters increment appropriately.
