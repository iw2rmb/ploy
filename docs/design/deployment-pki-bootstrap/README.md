# Deployment PKI Bootstrap

## Why
- `docs/v2/devops.md` promises that `ploy deploy bootstrap` provisions the cluster CA, beacon certificates, and worker leaf material during host bootstrap, but the current command only pushes the shell script and leaves PKI empty.
- Without an initial CA bundle in `/ploy/clusters/<cluster>/security/...`, follow-on flows (`ploy node add`, `ploy beacon rotate-ca`, `ploy cluster connect`) fail, blocking dry-run validation against the staging lab.
- Operators and other automation need the CLI to persist cluster descriptors (beacon URL, CA path, API keys) under `${XDG_CONFIG_HOME}/ploy/clusters/` so reconnect commands and trust bootstrap match the documentation.

## What to do
- Extend `ploy deploy bootstrap` to accept (or derive) the cluster identifier and etcd endpoints, then invoke `deploy.NewCARotationManager(...).Bootstrap` after the remote host preparation succeeds, writing CA + leaf certificates into etcd under `/ploy/clusters/<cluster>/security/`.
- Generate initial beacon/worker descriptors for the bootstrap node, recording certificate versions and revocation metadata consistent with `deploy.CARotationManager`.
- Persist the cluster descriptor locally via `config.SaveDescriptor`, including the emitted CA bundle path, beacon URL/IP, control-plane endpoint (if known), and API key so `ploy cluster list/connect` has real data.
- Surface any required bootstrap flags (`--cluster-id`, `--etcd-endpoints`, `--beacon-url`, `--api-key`) with validation and helpful errors when prerequisites are missing.
- Ensure `ploy node add --dry-run` succeeds immediately after bootstrap by confirming the CA material exists and the command can fetch certificates for preview.
- Update documentation to reflect the new behaviour and remove stale references to archived design docs (e.g., link to `.archive/deployment-worker-onboarding/` or successor work where appropriate).

## Where to change
- `cmd/ploy/deploy_command.go`, `internal/deploy/bootstrap.go`, and related helpers for CLI flag plumbing and post-bootstrap orchestration.
- `internal/deploy/ca_rotation.go` and `internal/deploy/worker_join.go` to reuse/bootstrap CA issuance paths and ensure descriptors line up with onboarding.
- `cmd/ploy/config/store.go` (and new helpers if needed) to cache cluster descriptors and CA bundles locally.
- Documentation under `docs/v2/devops.md`, `docs/v2/README.md`, and any other operator guides that describe bootstrap outputs.
- Tests in `internal/deploy` and new CLI coverage under `cmd/ploy` to exercise the bootstrap flow.

## COSMIC evaluation
| Functional process                                          | E | X | R | W | CFP |
|-------------------------------------------------------------|---|---|---|---|-----|
| Bootstrap CA, certificates, and local descriptors via CLI   | 1 | 1 | 1 | 1 | 4   |

- Assumptions: etcd connectivity is available from the operator workstation following bootstrap; bootstrap can reuse existing deployment CA manager without redesign.
- Open questions: Should the CLI mint worker certificates for additional nodes preemptively, or only for the bootstrap host?

## How to test
- Unit tests: `go test ./internal/deploy -run TestCARotation` extended with bootstrap coverage, plus new tests in `cmd/ploy` validating flag handling and descriptor writes.
- Integration smoke: `make build && dist/ploy deploy bootstrap --cluster-id <id> --address <addr> --etcd-endpoints ... --dry-run` (preview) and a live run against the VPS lab to confirm CA material lands in etcd and descriptors appear under `${XDG_CONFIG_HOME}`.
- Follow-up validation: `dist/ploy node add --dry-run` and `dist/ploy beacon rotate-ca --cluster-id <id> --dry-run` immediately after bootstrap.
