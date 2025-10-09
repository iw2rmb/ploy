# shift-mods-grid-05 — Mods Lane Consolidation (REFRACTOR)

- **Status**: [ ] Planned · [x] In Progress · [ ] Done
- **Design**: [../design/mods-grid-restoration/README.md](../../design/mods-grid-restoration/README.md)
- **Blocked by**: `shift-mods-grid-04`
- **Unblocks**: SHIFT lane migration design (TBD)

## Definition of Ready

- Healing scenarios pass GREEN tasks.

## Definition of Done

- Mods-specific lanes moved into the public
  [`ploy-lanes-catalog`](https://github.com/iw2rmb/ploy-lanes-catalog)
  repository with matching import hooks in Ploy.
- Documentation updated to reference SHIFT as the source of truth.

## Test Plan

- `go test ./internal/workflow/lanes`
- `go test -tags e2e ./tests/e2e`

## Notes

- Verified SHIFT lane catalog via `PLOY_LANES_DIR=$PLOY_LANES_DIR go run
  ./cmd/ploy lanes describe --lane mods-plan --manifest 2025-09-26`.
- `tests/e2e/mods_scenarios_test.go` now builds the CLI and, when `PLOY_GRID_ID`,
  `PLOY_GRID_API_KEY`, and `PLOY_LANES_DIR` are configured, runs `ploy mod run` against Grid via
  `TestModsScenariosLiveGrid`; additional scenarios can be enabled through
  `PLOY_E2E_LIVE_SCENARIOS`.
- Placeholder for additional follow-up once SHIFT lane publication completes.
