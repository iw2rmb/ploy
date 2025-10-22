# roadmap-gitlab-integration-04 — GitLab Credential Wiring

- **Status**: Planned — 2025-10-22
- **Dependencies**: design doc `docs/design/gitlab-integration/README.md` (to be authored), `docs/v2/README.md`

## Why

- Ploy nodes need short-lived GitLab tokens to clone repositories and open merge requests without
  persisting secrets locally.
- Operators require a single `ploy config set gitlab.api_key` workflow that stores credentials in
  etcd and propagates them to nodes over mTLS.

## What to do

- Implement an etcd-backed config service that stores encrypted GitLab API keys and exposes a signer
  issuing short-lived tokens to authenticated nodes.
- Extend the node bootstrap flow so workers authenticate via mTLS, fetch scoped tokens, refresh them
  automatically, and rotate on config changes.
- Add CLI commands for configuring GitLab credentials, inspecting signer status, and triggering
  manual rotation.

## Where to change

- `internal/config/gitlab` (new) — credential storage, signer, rotation hooks.
- `internal/node/bootstrap` — integrate signer handshake, in-memory refresh loop.
- `cmd/ploy/config` — CLI flows for set/get/list GitLab credentials.
- `docs/v2/devops.md`, `docs/v2/mod.md` — document operator workflow and troubleshooting.

## How to test

- `go test ./internal/config/gitlab/... ./internal/node/bootstrap/...` covering signer issuance,
  rotation events, and failure backoff.
- Integration harness: spawn a dev signer + node, verify tokens refresh before expiry and rotate on
  etcd updates.
- CLI smoke: `make build && dist/ploy config gitlab --help`, end-to-end config set + node bootstrap.
