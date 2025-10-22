# Deployment Worker Onboarding

## Why
- Workers must join clusters automatically with verified registration, health checks, and metadata updates.
- Manual onboarding creates drift and risks stale metadata.

## What to do
- Automate worker join flow invoked by `ploy node add`, verifying registration and health probes.
- Update cluster metadata entries with worker descriptors and labels.
- Coordinate certificate distribution with [`../deployment-ca-rotation/README.md`](../deployment-ca-rotation/README.md).

## Where to change
- [`cmd/ploy/deploy`](../../../cmd/ploy/deploy) and [`cmd/ploy/node`](../../../cmd/ploy/node) for CLI entry points.
- [`internal/deploy`](../../../internal/deploy) to execute onboarding scripts and metadata writes.
- [`internal/controlplane/registry`](../../../internal/controlplane/registry) (or similar) for worker descriptors.
- Update documentation in [`docs/v2/devops.md`](../../v2/devops.md) with onboarding runbooks.

## COSMIC evaluation
| Functional process                             | E | X | R | W | CFP |
|------------------------------------------------|---|---|---|---|-----|
| Automate worker joins and metadata updates     | 1 | 1 | 1 | 1 | 4   |
| **TOTAL**                                      | 1 | 1 | 1 | 1 | 4   |

- Assumption: metadata stored in etcd; onboarding writes once per node.
- Open question: confirm failure rollback strategy if health checks fail post-registration.

## How to test
- `go test ./internal/deploy -run TestWorkerJoin` verifying metadata writes and error paths.
- Integration: add worker in staging, confirm registration, health checks, and metadata entries.
- Smoke: `make build && dist/ploy node add --dry-run` to preview onboarding steps.
