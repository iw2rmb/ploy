# Deployment Bootstrap Host

## Why
- Operators need a repeatable bootstrap that converges dependency versions (etcd 3.6.x, IPFS Cluster 1.1.4, Docker 28.x, Go 1.25).
- The process must run via CLI commands for workstations and clusters with idempotent checks.

## What to do
- Finalise the embedded bootstrap script invoked by `ploy deploy bootstrap` to install dependencies and verify host readiness.
- Implement preflight checks for OS packages, ports, and disk before continuing.
- Reference follow-up docs: [`../deployment-ca-rotation/README.md`](../deployment-ca-rotation/README.md) and [`../deployment-worker-onboarding/README.md`](../deployment-worker-onboarding/README.md).

## Where to change
- [`cmd/ploy/deploy`](../../../cmd/ploy/deploy) to invoke bootstrap routines.
- [`internal/deploy`](../../../internal/deploy) for script templating and execution over SSH.
- Script assets at [`internal/deploy/assets/bootstrap.sh`](../../../internal/deploy/assets/bootstrap.sh) aligning versions and checks.
- Update dependency mentions in [`docs/v2/devops.md`](../../v2/devops.md).

## COSMIC evaluation
| Functional process                               | E | X | R | W | CFP |
|--------------------------------------------------|---|---|---|---|-----|
| Bootstrap host dependencies and prerequisites    | 1 | 1 | 1 | 1 | 4   |
| **TOTAL**                                        | 1 | 1 | 1 | 1 | 4   |

- Assumption: bootstrap writes temporary files only within working directory.
- Open question: confirm remote execution supports both workstation SSH and cluster nodes.

## How to test
- `make build && dist/ploy deploy bootstrap --dry-run` targeting local VM to verify checks.
- Unit tests under `internal/deploy` covering command templates and failure cases.
- Shellcheck on generated script assets to enforce lint correctness.
