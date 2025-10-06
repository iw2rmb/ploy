# shift-mods-grid-02 — Mods Grid Materialisation (GREEN)

- **Status**: [ ] Planned · [ ] In Progress · [x] Done
- **Design**: [../design/mods-grid-restoration/README.md](../../design/mods-grid-restoration/README.md)
- **Blocked by**: `shift-mods-grid-01`
- **Unblocks**: `shift-mods-grid-03`

## Definition of Ready

- RED tests from `shift-mods-grid-01` are failing and committed.
- Repository access secrets made available locally.

## Definition of Done

- Workflow tickets accept repository metadata; validation updated.
- Runner materialises repo into workspace and populates Mods stage job specs (OpenRewrite/LLM/Human lanes).
- `TestModsScenarioSimpleOpenRewrite` progresses past RED guard and reaches build gate (may still fail pending healing work).
- Documentation and env var references updated per design doc.

## Test Plan

- `go test ./...`
- `go test -tags e2e ./tests/e2e -run TestModsScenarioSimpleOpenRewrite`
- Manual: `ploy mod run --tenant <tenant> --repo-url <repo> --repo-base-ref <base>
  --repo-target-ref <target>` (workspace smoke test)

## Notes

- Ensure job spec composition reuses lane registry defaults without duplicating SHIFT assets yet.
