# shift-mods-grid-05 — Mods Lane Consolidation (REFRACTOR)

- **Status**: [x] Planned · [ ] In Progress · [ ] Done
- **Design**: [../design/mods-grid-restoration/README.md](../../design/mods-grid-restoration/README.md)
- **Blocked by**: `shift-mods-grid-04`
- **Unblocks**: SHIFT lane migration design (TBD)

## Definition of Ready

- Healing scenarios pass GREEN tasks.

## Definition of Done

- Mods-specific lanes moved into SHIFT repository with matching import hooks in Ploy.
- Documentation updated to reference SHIFT as the source of truth.

## Test Plan

- `go test ./internal/workflow/lanes`
- `go test -tags e2e ./tests/e2e`

## Notes

- Replace `tests/e2e/mods_scenarios_test.go`'s in-memory Grid harness with real Grid smoke once SHIFT lanes publish.
- Placeholder for follow-up; flesh out scope after GREEN delivery.
