# roadmap-cli-refresh-05 — CLI Surface Refresh

- **Status**: Planned — 2025-10-22
- **Dependencies**: `docs/design/cli-refresh/README.md` (to be authored), `docs/v2/cli.md`

## Why

- The CLI must expose the v2 workflows (Mods, nodes, artifacts, deployment) without Grid terminology.
- Operators expect structured command groups, consistent help output, and log streaming commands that
  match the control plane APIs.

## What to do

- Reorganise `ploy` commands into workstation-first groups (cluster, mods, nodes, artifacts,
  observability) with shared persistent flags.
- Implement streaming subcommands (`ploy mods logs`, `ploy jobs follow`) that consume the SSE APIs
  delivered in the control plane.
- Update generated help, autocompletion artifacts, and documentation snippets to reflect the new
  command tree.

## Where to change

- `cmd/ploy/root.go` and subcommand files — group registration, persistent flags, streaming command
  implementations.
- `cmd/ploy/testdata/` — update golden help fixtures and streaming snapshots.
- `docs/v2/cli.md`, `docs/v2/logs.md` — document the refreshed command tree and examples.

## How to test

- `go test ./cmd/ploy -run TestHelp` for usage text.
- Snapshot tests covering streaming output where practical.
- CLI smoke: `make build && dist/ploy help`, `dist/ploy mods logs <job-id>` against dev control
  plane.
