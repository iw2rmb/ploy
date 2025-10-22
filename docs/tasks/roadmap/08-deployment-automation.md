# roadmap-deployment-automation-08 — Bootstrap & Node Ops

- **Status**: Planned — 2025-10-22
- **Dependencies**: `docs/design/deployment-automation/README.md` (to be authored), `docs/v2/devops.md`

## Why

- Operators need a single CLI-driven workflow to bootstrap beacon nodes, add workers, rotate
  certificates, and manage cluster metadata across environments.
- The embedded shell script must converge dependencies (Docker, etcd, IPFS Cluster) to the supported
  versions documented for v2.

## What to do

- Finalise the embedded bootstrap script consumed by `ploy deploy bootstrap` and `ploy node add`,
  ensuring idempotent installs, CA generation, and cluster descriptor updates.
- Implement CLI subcommands (`ploy cluster connect`, `ploy node list`, `ploy beacon rotate-ca`) that
  wrap the new deployment APIs.
- Document smoke tests and troubleshooting workflows in `docs/v2/devops.md`, including version checks
  for etcd 3.6.x, IPFS Cluster 1.1.4, Docker 28.x, and Go 1.25.

## Where to change

- `cmd/ploy/deploy` and `cmd/ploy/node` — bootstrap wiring, cluster management commands.
- `internal/deploy` (new) — helper package for templating scripts, tracking CA artifacts, and
  executing remote steps.
- `docs/v2/devops.md` — align with the scripted workflow and update prerequisite versions.
- Embedded script under `docs/v2/implement.sh` — converge operating system actions with the CLI.

## How to test

- Integration harness: run `ploy deploy bootstrap` against local VMs or containers, validate etcd,
  IPFS, and node services start successfully.
- CLI smoke: rotate CA and rejoin nodes to confirm certificate distribution.
- Static analysis: shellcheck the embedded script and run `make test` to ensure Go wiring passes.
