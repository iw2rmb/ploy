# GitLab Token Signer

This document describes the contract consumed by worker nodes during the GitLab
bootstrap flow. The implementation lives in `internal/config/gitlab/signer.go`
and is referenced by the node bootstrapper in
`internal/node/bootstrap/bootstrap.go`.

## Handshake

- **Transport**: mutual TLS between the node and signer service. Nodes must
  present a client certificate signed by the control-plane CA.
- **Request**: `{ secretName, scopes?, ttl? }`. Nodes typically omit `scopes`
  to request the default set stored alongside the secret.
- **Response**: `{ token, scopes }` where `token` includes metadata:
  `secretName`, `tokenId`, `value`, `issuedAt`, `expiresAt`, and allowed `scopes`.
- **Failure Semantics**: transient issues surface as retriable errors. Nodes are
  expected to retry with exponential or fixed backoff without blocking process
  startup.

## Token Refresh

- Nodes call `IssueToken` with the same secret and scopes resolved during the
  handshake.
- The signer enforces maximum TTLs; callers may request shorter TTLs to tighten
  refresh windows.
- Responses mirror the handshake payload to simplify cache updates on the node.

## Rotation Subscription

- Nodes subscribe via `SubscribeRotations(secretName)` to receive
  `RotationEvent` payloads: `{ secretName, revision, updatedAt }`.
- On notification, nodes must flush cached tokens and immediately trigger a new
  refresh cycle.
- The signer fan-outs rotation events using etcd watchers; see
  `gitlab/signer.go` for full event propagation details.

## Observability

- Refresh attempts should emit success/failure counters tagged by secret name.
- Nodes log refresh schedules and failure backoffs at debug level to aid
  incident triage.

## Security Considerations

- Tokens are short-lived bearer credentials and must never be persisted to
  disk.
- Client certificates must be rotated alongside signer CAs; bootstrap logic is
  expected to surface TLS errors with actionable messages.
