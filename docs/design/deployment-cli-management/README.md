# Deployment CLI Management

## Why
- Operators need CLI commands to inspect cluster status, list nodes, and invoke deployment automation.
- The CLI should surface clear status output for bootstrap, onboarding, and rotation actions.

## What to do
- Implement CLI subcommands (`ploy cluster connect`, `ploy node list`, etc.) that wrap deployment automation hooks.
- Display status, progress, and error summaries pulled from deployment scripts.
- Cross-link to automation docs: [`../deployment-bootstrap-host/README.md`](../deployment-bootstrap-host/README.md) and [`../deployment-worker-onboarding/README.md`](../deployment-worker-onboarding/README.md).

## Where to change
- [`cmd/ploy/deploy`](../../../cmd/ploy/deploy) and [`cmd/ploy/node`](../../../cmd/ploy/node) for command implementations.
- [`cmd/ploy/testdata`](../../../cmd/ploy/testdata) to update help/output snapshots.
- [`docs/v2/cli.md`](../../v2/cli.md) and [`docs/v2/devops.md`](../../v2/devops.md) for user guidance.

## COSMIC evaluation
| Functional process                        | E | X | R | W | CFP |
|-------------------------------------------|---|---|---|---|-----|
| Expose CLI cluster management commands    | 1 | 1 | 1 | 0 | 3   |
| **TOTAL**                                 | 1 | 1 | 1 | 0 | 3   |

- Assumption: CLI commands invoke automation but store no new persistent data.
- Open question: confirm status output must include remote host logs or just summaries.

## How to test
- `go test ./cmd/ploy/deploy -run TestClusterCommands` verifying flags and output.
- Help snapshot updates in `cmd/ploy/testdata`.
- Manual smoke: `make build && dist/ploy cluster connect --dry-run` to confirm progress output.
