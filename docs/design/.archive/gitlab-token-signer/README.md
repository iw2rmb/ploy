# GitLab Token Signer

## Why
- Control plane needs a scoped credential service that issues short-lived GitLab tokens without exposing operator API keys.
- Etcd must store encrypted API keys so rotations propagate across clusters with auditability.

## What to do
- Build a signer package that stores encrypted API keys, issues scoped short-lived tokens, and rotates secrets on demand.
- Record rotation watchers that notify nodes and CLI flows through etcd change streams.
- Document the Credential API contract for other design docs: link to `../../gitlab-node-refresh/README.md` for consumer expectations.

## Where to change
- [`internal/config/gitlab`](../../../../internal/config/gitlab) for signer structs, token issuance, and encryption helpers.
- [`internal/controlplane`](../../../../internal/controlplane) wiring to expose signer RPCs and rotation notifications.
- Upstream dependency: [`../../gitlab-node-refresh/README.md`](../../gitlab-node-refresh/README.md) consumes the issued tokens.
- Update required environment variables in [`docs/envs/README.md`](../../../envs/README.md) for signer secrets and TTLs.

## Credential API Contract

- `PUT /v2/gitlab/signer/secrets` rotates the encrypted GitLab API key stored in etcd. The payload requires `secret`, `api_key`, and `scopes`. Successful responses return the etcd revision and timestamp for auditing.
- `POST /v2/gitlab/signer/tokens` issues a short-lived credential. Callers provide the `secret` identifier, optional `scopes`, and `ttl_seconds`. Responses include the scoped token, expiration timestamp, and applied TTL.
- `GET /v2/gitlab/signer/rotations?since=<rev>&timeout=<duration>` long-polls for rotation events. Clients should pass the last observed revision so the signer can stream the next rotation. Consumers must handle `204` (timeout) gracefully and re-issue the watch.
- Downstream components (nodes, CLI) must follow the refresh behaviour outlined in [`../../gitlab-node-refresh/README.md`](../../gitlab-node-refresh/README.md).

## COSMIC evaluation
| Functional process               | E | X | R | W | CFP |
|----------------------------------|---|---|---|---|-----|
| Issue short-lived GitLab tokens  | 1 | 1 | 1 | 1 | 4   |
| **TOTAL**                        | 1 | 1 | 1 | 1 | 4   |

- Assumption: encryption keys reuse existing control plane KMS wiring; no extra storage beyond etcd entries.
- Open question: confirm TTL defaults to 15 minutes or align with GitLab API limits.

## Implementation Notes (2025-10-22)

- AES encryption keys are sourced from `PLOY_GITLAB_SIGNER_AES_KEY`; the decoded key must be 16/24/32 bytes to satisfy AES-GCM requirements.
- Default token TTL remains 15 minutes and can be overridden with `PLOY_GITLAB_SIGNER_DEFAULT_TTL`; issued tokens can never exceed the limit set by `PLOY_GITLAB_SIGNER_MAX_TTL` (default 12 hours).
- Rotation events are exposed through `/v2/gitlab/signer/rotations`, streaming etcd change notifications to node and CLI consumers.

## How to test
- `go test ./internal/config/gitlab -run TestSigner` covering token issuance, expiry, and rotation watcher fan-out.
- Integration: apply fake API key via etcd, request token via signer RPC, assert TTL and scope.
- Smoke: `make test` to ensure new package wiring keeps coverage ≥60% overall.
