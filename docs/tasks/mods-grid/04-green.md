# mods-grid-04 — Mods Healing Branching (GREEN)

- **Status**: [ ] Planned · [ ] In Progress · [x] Done
- **Design**: [../design/mods-grid-restoration/README.md](../../design/mods-grid-restoration/README.md)
- **Blocked by**: `mods-grid-03`
- **Unblocks**: `mods-grid-05`

## Definition of Ready

- RED tests for healing branching are failing and committed.
- Build gate metadata schema updates reviewed.

## Definition of Done

- Runner feeds build gate failures into Mods planner signals and reschedules
  healing steps.
- Planner emits parallel options when KB/LLM recommend multiple fixes; runner
  tracks branch completion before moving to human gate.
- All new unit tests pass; E2E scenarios `buildgate-self-heal` and
  `parallel-healing-options` execute to completion (may still require human
  stub if unresolved).
- Documentation/design/CHANGELOG updated with verification evidence.

## Test Plan

- `go test ./...`
- `go test -tags e2e ./tests/e2e -run TestModsScenarioBuildGateSelfHeal`
- `go test -tags e2e ./tests/e2e -run TestModsScenarioParallelHealingOptions`

## Notes

- Ensure planner metadata remains backward compatible for existing consumers.
