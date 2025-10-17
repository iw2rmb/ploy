# mods-grid-03 — Mods Healing Branching (RED)

- **Status**: [ ] Planned · [ ] In Progress · [x] Done
- **Design**: [../design/mods-grid-restoration/README.md](../../design/mods-grid-restoration/README.md)
- **Blocked by**: `mods-grid-02`
- **Unblocks**: `mods-grid-04`

## Definition of Ready

- Simple OpenRewrite path reaches build gate with real job specs.
- Knowledge Base catalog accessible in workstation environment.

## Definition of Done

- Failing tests capture expectations for build-gate feedback, planner retries, and parallel healing branches.
- E2E scenarios `buildgate-self-heal` and `parallel-healing-options` move past documentation-only failure into
  test assertions that currently fail.
- Design doc updated with any new acceptance criteria discovered.

## Test Plan (Expected Failures)

- `go test ./internal/workflow/runner -run TestModsHealingRetry` (new)
- `go test -tags e2e ./tests/e2e -run TestModsScenarioBuildGateSelfHeal`
- `go test -tags e2e ./tests/e2e -run TestModsScenarioParallelHealingOptions`

## Notes

- Capture build gate metadata fixture for reuse in GREEN task.
