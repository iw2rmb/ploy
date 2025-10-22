# GitLab Rotation Revocation

## Why
- Credential rotations must revoke stale tokens across nodes to prevent unauthorized GitLab access.
- Operators need audit markers showing which nodes consumed or failed to revoke credentials.

## What to do
- Broadcast rotation events over etcd watchers and control plane RPCs so nodes drop cached tokens immediately.
- Revoke stale tokens via GitLab API, logging failures with node identifiers.
- Feed audit events into observability pipelines referenced in [`../gitlab-cli-ops/README.md`](../gitlab-cli-ops/README.md).

## Where to change
- [`internal/config/gitlab`](../../../internal/config/gitlab) to emit rotation events and track token IDs.
- [`internal/controlplane/events`](../../../internal/controlplane/events) (or equivalent) for fan-out to nodes.
- [`internal/node/bootstrap`](../../../internal/node/bootstrap) for rotation signal handling.
- Upstream docs: [`../gitlab-token-signer/README.md`](../gitlab-token-signer/README.md) and [`../gitlab-cli-ops/README.md`](../gitlab-cli-ops/README.md). Node refresh flows now live in [`../.archive/gitlab-node-refresh/README.md`](../.archive/gitlab-node-refresh/README.md).

## COSMIC evaluation
| Functional process                        | E | X | R | W | CFP |
|-------------------------------------------|---|---|---|---|-----|
| Broadcast rotation events and revoke tokens | 1 | 1 | 1 | 1 | 4 |
| **TOTAL**                                 | 1 | 1 | 1 | 1 | 4 |

- Assumption: GitLab API supports bulk revocation by token ID; fallback loops handle per-token retries.
- Open question: ensure node metadata includes signer-issued token IDs for audit correlation.

## How to test
- `go test ./internal/config/gitlab -run TestRotation` verifying revocation calls and watcher delivery.
- Integration: trigger rotation via signer CLI, confirm nodes drop credentials and audit logs record revocation.
- CI smoke: run `make test` with GitLab API mocked to assert failure handling paths.
