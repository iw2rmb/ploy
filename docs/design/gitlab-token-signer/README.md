# GitLab Token Signer

## Why
- Control plane needs a scoped credential service that issues short-lived GitLab tokens without exposing operator API keys.
- Etcd must store encrypted API keys so rotations propagate across clusters with auditability.

## What to do
- Build a signer package that stores encrypted API keys, issues scoped short-lived tokens, and rotates secrets on demand.
- Record rotation watchers that notify nodes and CLI flows through etcd change streams.
- Document the Credential API contract for other design docs: link to `../gitlab-node-refresh/README.md` for consumer expectations.

## Where to change
- [`internal/config/gitlab`](../../../internal/config/gitlab) for signer structs, token issuance, and encryption helpers.
- [`internal/controlplane`](../../../internal/controlplane) wiring to expose signer RPCs and rotation notifications.
- Upstream dependency: [`../gitlab-node-refresh/README.md`](../gitlab-node-refresh/README.md) consumes the issued tokens.
- Update required environment variables in [`docs/envs/README.md`](../../envs/README.md) for signer secrets and TTLs.

## COSMIC evaluation
| Functional process               | E | X | R | W | CFP |
|----------------------------------|---|---|---|---|-----|
| Issue short-lived GitLab tokens  | 1 | 1 | 1 | 1 | 4   |
| **TOTAL**                        | 1 | 1 | 1 | 1 | 4   |

- Assumption: encryption keys reuse existing control plane KMS wiring; no extra storage beyond etcd entries.
- Open question: confirm TTL defaults to 15 minutes or align with GitLab API limits.

## How to test
- `go test ./internal/config/gitlab -run TestSigner` covering token issuance, expiry, and rotation watcher fan-out.
- Integration: apply fake API key via etcd, request token via signer RPC, assert TTL and scope.
- Smoke: `make test` to ensure new package wiring keeps coverage ≥60% overall.
