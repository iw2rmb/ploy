# GitLab Integration & Credentials

## Why

- GitLab is the canonical source/destination for Mods; API credentials must live
  in etcd and replicate securely to nodes (`docs/v2/README.md`).
- Eliminating Grid adapters requires a purpose-built GitLab integration aligned
  with current security guidance.

## Required Changes

- Design etcd-backed configuration paths for GitLab API keys, project metadata,
  and branch policies with fine-grained RBAC.
- Implement mutual TLS distribution so nodes fetch short-lived tokens at
  startup, never persisting plaintext secrets on disk, matching GitLab’s
  recommended automation practices.citeturn0search0
- Build CLI workflows for configuring credentials (`ploy config set`),
  validating permissions, and rotating keys without downtime.
- Replace legacy Grid GitLab hooks with native implementations covering clone,
  branch, merge request, and status update flows.

## Definition of Done

- Nodes clone repositories and manage merge requests using only the new
  etcd-provisioned credentials.
- Credential rotation can be executed via CLI with zero manual node restarts,
  and audit logs record changes.
- Security documentation enumerates scopes, renewal cadence, and incident
  response for leaked keys.

## Tests

- Unit tests for credential storage, scope validation, and RBAC enforcement.
- Integration tests hitting GitLab’s sandbox APIs to exercise clone/branch/MR
  flows using mocked tokens.
- CLI acceptance tests that simulate credential misconfiguration and verify
  user-facing guidance.
