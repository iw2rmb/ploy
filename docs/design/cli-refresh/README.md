# CLI Refresh

## Why
- The CLI must present Ploy v2 workflows (mods, nodes, artifacts, deployment) without Grid terminology.
- Operators expect structured command groups, consistent help output, and streaming commands aligned with the new control plane APIs.
- Documentation and autocomplete artifacts should stay in sync with the refreshed command tree for workstation and cluster users.

## What to do
- Reorganise `ploy` subcommands into workstation-first groups (cluster, mods, nodes, artifacts, observability) with shared persistent flags.
- Implement streaming commands such as `ploy mods logs` and `ploy jobs follow` that consume the SSE endpoints delivered by the control plane.
- Update generated help output, autocomplete scripts, and documentation snippets to reflect the new tree and deprecation notices.

## Where to change
- [`cmd/ploy/root.go`](../../../cmd/ploy/root.go) and subcommand files for group registration, persistent flags, and streaming command implementations.
- [`cmd/ploy/testdata`](../../../cmd/ploy/testdata) fixtures to update golden help and streaming snapshots.
- Supporting packages under [`cmd/ploy`](../../../cmd/ploy) for log follow mechanics and SSE handling.
- [`docs/v2/cli.md`](../../v2/cli.md), [`docs/v2/logs.md`](../../v2/logs.md), and other operator docs for command examples and migration guidance.

## COSMIC evaluation
| Functional process              | E | X | R | W | CFP |
|---------------------------------|---|---|---|---|-----|
| Restructure CLI command tree    | 1 | 1 | 1 | 1 | 4   |
| Implement streaming log follow  | 1 | 1 | 1 | 0 | 3   |
| Regenerate help/autocomplete    | 1 | 1 | 1 | 1 | 4   |
| **TOTAL**                       | 3 | 3 | 3 | 2 | 11  |

- Assumptions: persistent flag registration writes once to the command tree; streaming outputs rely on SSE without intermediate persistence.
- Open questions: double-check whether autocomplete generation requires additional writes for shell-specific artifacts beyond the documented output.

## How to test
- `go test ./cmd/ploy -run TestHelp` to cover usage text and grouping.
- Snapshot tests for streaming output where practical.
- CLI smoke: `make build && dist/ploy help`, `dist/ploy mods logs <job-id>` against a dev control plane to validate command wiring.
