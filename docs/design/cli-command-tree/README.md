# CLI Command Tree

## Why
- Ploy CLI must present v2 workflows without legacy Grid terminology.
- Operators expect consistent grouping, persistent flags, and help output that matches documentation.

## What to do
- Reorganise command groups into workstation-first hierarchy with shared persistent flags.
- Remove deprecated Grid aliases and ensure help output reflects the new layout.
- Reference streaming and autocomplete follow-ups: [`../cli-streaming/README.md`](../cli-streaming/README.md) and [`../cli-help-autocomplete/README.md`](../cli-help-autocomplete/README.md).

## Where to change
- [`cmd/ploy/root.go`](../../../cmd/ploy/root.go) and subcommand files for group registration and persistent flag setup.
- [`cmd/ploy/mods`](../../../cmd/ploy/mods) etc. to restructure command packages.
- [`docs/v2/cli.md`](../../v2/cli.md) to mirror the new tree.
- Upstream docs: [`../observability-log-streaming/README.md`](../observability-log-streaming/README.md) for streaming command dependencies.

## COSMIC evaluation
| Functional process             | E | X | R | W | CFP |
|--------------------------------|---|---|---|---|-----|
| Restructure CLI command tree   | 1 | 1 | 1 | 1 | 4   |
| **TOTAL**                      | 1 | 1 | 1 | 1 | 4   |

- Assumption: command tree writes occur once during init without runtime persistence.
- Open question: confirm legacy aliases require explicit deprecation warnings in help text.

## How to test
- `go test ./cmd/ploy -run TestHelp` to validate grouping and persistent flags.
- Golden updates under `cmd/ploy/testdata` verifying help output.
- Manual smoke: `make build && dist/ploy help` to review command hierarchy end-to-end.
