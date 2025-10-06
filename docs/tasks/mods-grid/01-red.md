# shift-mods-grid-01 — Mods Grid Materialisation (RED)

- **Status**: [ ] Planned · [ ] In Progress · [x] Done
- **Design**: [../design/mods-grid-restoration/README.md](../../design/mods-grid-restoration/README.md)
- **Blocked by**: None
- **Unblocks**: `shift-mods-grid-02`

## Definition of Ready

- Design doc reviewed and accepted.
- Test repo credentials available via env vars (`PLOY_E2E_*`).

## Definition of Done

- Added unit coverage for repository materialisation hooks (`WorkspacePreparer`).
- Updated CLI tests to exercise `ploy mod run` flag parsing.
- Documented the RED expectations in the design doc.

## Test Plan (Expected Failures)

- `go test ./internal/workflow/runner -run TestModsWorkspaceMaterialisation -tags=e2e` (new failing test)
- `go test -tags e2e ./tests/e2e -run TestModsScenarioSimpleOpenRewrite`

## Notes

- Capture workspace layout expectations in fixtures for downstream tasks.
