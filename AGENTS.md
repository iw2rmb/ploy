# AGENTS.md

Follow instructions in `~/.codex/AGENTS.md`

## CLI Build & Smoke Checks

- `make build` places the CLI in `dist/ploy`.
- `make test` executes `go test -cover ./...` including guardrail tests for
  legacy imports and command directories.
- Keep the CLI binary minimal; all orchestration logic belongs in
  `internal/workflow/...` packages.
