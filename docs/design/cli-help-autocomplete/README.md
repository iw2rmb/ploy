# CLI Help Autocomplete

## Why
- Help output and autocomplete scripts must mirror the refreshed command tree.
- Operators expect updated shell completion files for bash, zsh, and fish.

## What to do
- Regenerate help text and autocomplete artifacts after the command tree refresh.
- Ensure deprecation messages surface for removed Grid terminology.
- Update docs pointing to generated artifacts, referencing [`../cli-command-tree/README.md`](../cli-command-tree/README.md).

## Where to change
- [`cmd/ploy/root.go`](../../../cmd/ploy/root.go) to trigger autocomplete generation hooks.
- [`cmd/ploy/autocomplete`](../../../cmd/ploy/autocomplete) or equivalent for script outputs.
- [`cmd/ploy/testdata`](../../../cmd/ploy/testdata) for golden help snapshots.
- [`docs/v2/cli.md`](../../v2/cli.md) updating install instructions.
- Upstream doc dependency: [`../cli-command-tree/README.md`](../cli-command-tree/README.md).

## COSMIC evaluation
| Functional process               | E | X | R | W | CFP |
|----------------------------------|---|---|---|---|-----|
| Regenerate help/autocomplete     | 1 | 1 | 1 | 1 | 4   |
| **TOTAL**                        | 1 | 1 | 1 | 1 | 4   |

- Assumption: autocomplete scripts write to local filesystem during generation only.
- Open question: confirm CI pipelines upload scripts for distribution.

## How to test
- `go test ./cmd/ploy -run TestHelp` verifying regenerated help text.
- Golden diff review for autocomplete scripts via `make build` or dedicated target.
- Manual smoke: install generated completions and confirm top-level commands resolve.
