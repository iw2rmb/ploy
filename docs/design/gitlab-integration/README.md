# GitLab Integration

## Why
- Nodes must fetch short-lived GitLab tokens over mTLS so repository cloning and MR automation never store long-lived secrets locally.
- Operators need a single `ploy config set gitlab.api_key` workflow that saves credentials in etcd and propagates rotation events cluster-wide.
- The control plane has to surface signer status so workstation deployments and clusters behave consistently.

## What to do
- Create an etcd-backed credential service that encrypts stored GitLab API keys, issues scoped short-lived tokens, and watches for rotation events.
- Extend node bootstrap so workers authenticate with beacon-issued certificates, request tokens from the signer, refresh them before expiry, and flush them on credential change.
- Expose CLI commands for configuring GitLab credentials, inspecting signer health, and forcing rotation; update docs to guide operators through the workflow.

## Where to change
- [`internal/config/gitlab`](../../../internal/config/gitlab) (new) for credential storage, signer, and rotation hooks.
- [`internal/node/bootstrap`](../../../internal/node/bootstrap) for signer handshake, refresh loop, and metrics.
- [`cmd/ploy/config`](../../../cmd/ploy/config) plus supporting packages to wire CLI config flows and status output.
- [`docs/v2/devops.md`](../../v2/devops.md), [`docs/v2/mod.md`](../../v2/mod.md), and related operator docs to document bootstrap steps and troubleshooting.

## COSMIC evaluation
| Functional process            | E | X | R | W | CFP |
|-------------------------------|---|---|---|---|-----|
| Issue short-lived GitLab token| 1 | 1 | 1 | 1 | 4   |
| Refresh node credential cache | 1 | 1 | 1 | 1 | 4   |
| Rotate cluster credentials    | 1 | 1 | 1 | 2 | 5   |
| **TOTAL**                     | 3 | 3 | 3 | 4 | 13  |

- Assumptions: signer stores encrypted API keys in etcd and writes rotation events once per credential change.
- Open questions: need to confirm whether per-node audit logging adds an extra write beyond cache updates.

## How to test
- `go test ./internal/config/gitlab/... ./internal/node/bootstrap/...` covering token issuance, rotation, and failure backoff logic.
- Integration harness: spin up signer + node with mutual TLS, confirm automatic refresh, rotation on etcd updates, and token revocation.
- CLI smoke: `make build && dist/ploy config gitlab ...` to run end-to-end credential set/list commands against the test signer.
