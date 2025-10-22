# GitLab CLI Ops

## Why
- Operators require a single CLI workflow to set GitLab API keys, inspect signer health, and force credential rotations.
- Documentation must show workstation and cluster procedures aligned with the signer contracts.

## What to do
- Add CLI commands for configuring GitLab credentials, checking signer status, and triggering rotations.
- Surface audit metadata (last rotation, failing nodes) in CLI output sourced from rotation doc [`../gitlab-rotation-revocation/README.md`](../gitlab-rotation-revocation/README.md).
- Update operator docs with step-by-step workflows and failure recovery guidance.

## Where to change
- [`cmd/ploy/config`](../../../cmd/ploy/config) for new subcommands and output formatting.
- [`cmd/ploy/testdata`](../../../cmd/ploy/testdata) to update golden help and signer responses.
- [`docs/v2/devops.md`](../../v2/devops.md) for operator walkthroughs.
- Upstream design docs: [`../gitlab-token-signer/README.md`](../gitlab-token-signer/README.md) and [`../gitlab-node-refresh/README.md`](../gitlab-node-refresh/README.md).

## COSMIC evaluation
| Functional process                   | E | X | R | W | CFP |
|--------------------------------------|---|---|---|---|-----|
| CLI credential setup and rotation ops | 1 | 1 | 1 | 0 | 3 |
| **TOTAL**                            | 1 | 1 | 1 | 0 | 3 |

- Assumption: CLI writes only configuration requests; actual secret storage remains in signer.
- Open question: confirm CLI handles interactive prompts vs. env var inputs for automation.

## How to test
- `go test ./cmd/ploy/config -run TestGitLabCommands` to cover new commands and error flows.
- Snapshot tests for CLI help updates under `cmd/ploy/testdata`.
- Manual smoke: `make build && dist/ploy config gitlab status` against dev signer to confirm wiring.
