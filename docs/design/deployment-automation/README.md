# Deployment Automation

## Why
- Operators require a single CLI-driven workflow to bootstrap beacon nodes, add workers, rotate certificates, and manage cluster metadata.
- The embedded shell script must converge supported dependency versions (etcd 3.6.x, IPFS Cluster 1.1.4, Docker 28.x, Go 1.25) across environments.
- Workstation and cluster deployments should share the same automation, reducing manual drift.

## What to do
- Finalise the embedded bootstrap script used by `ploy deploy bootstrap` and `ploy node add`, ensuring idempotent installs, CA generation, DNS updates, and cluster descriptor writes.
- Implement CLI subcommands (`ploy cluster connect`, `ploy node list`, `ploy beacon rotate-ca`, etc.) that invoke the automation and surface status.
- Document smoke tests and troubleshooting flows, including verification commands and rollback steps.

## Where to change
- [`cmd/ploy/deploy`](../../../cmd/ploy/deploy) and related packages for bootstrap wiring and cluster management commands.
- [`internal/deploy`](../../../internal/deploy) (new) to template scripts, manage CA artifacts, and run remote steps over SSH.
- [`docs/v2/devops.md`](../../v2/devops.md) and the embedded script [`docs/v2/implement.sh`](../../v2/implement.sh) to align prerequisite versions and operational guidance.

## COSMIC evaluation
| Functional process           | E | X | R | W | CFP |
|-----------------------------|---|---|---|---|-----|
| Bootstrap cluster automation | 1 | 1 | 2 | 2 | 6   |
| Add worker node              | 1 | 1 | 1 | 2 | 5   |
| Rotate CA certificates       | 1 | 1 | 1 | 1 | 4   |
| **TOTAL**                    | 3 | 3 | 4 | 5 | 15  |

- Assumptions: counts cover happy-path automation without failure retries; workstation and cluster flows reuse the same processes.
- Open questions: need confirmation on additional writes for DNS updates and metadata replication during bootstrap.

## How to test
- Integration harness: execute `ploy deploy bootstrap` and `ploy node add` against local VMs or containers, verifying services start and register correctly.
- CLI smoke: rotate CA and rejoin nodes to confirm certificate distribution and metadata updates.
- Static analysis: run shellcheck on the embedded script and `make test` to ensure Go components pass unit tests.
