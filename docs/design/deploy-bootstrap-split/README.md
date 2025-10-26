# Deploy Bootstrap Decomposition

## Why
- [`internal/deploy/bootstrap.go`](../../../internal/deploy/bootstrap.go) is 449 LOC and mixes option plumbing, SSH helpers, workstation CA installers, and resolver prompts, so the entrypoint diff churn is high for otherwise isolated tweaks.
- Helper functions such as `buildSSHArgs` and `randomHexString` are shared with [`internal/deploy/provision.go`](../../../internal/deploy/provision.go) but buried in the same file, making dependency review awkward.
- Workstation configuration logic (macOS/Linux CA install, resolver prompts) is dormant yet noisy; co-locating it with the bootstrap entrypoint camouflages the actual bootstrap flow during code review.

## What to do
1. Keep `bootstrap.go` focused on orchestration (`RunBootstrap`) while ensuring it only imports the helpers/types it needs from sibling files.
2. Introduce focused files under `internal/deploy`:
   - `bootstrap_types.go` — constants plus `Options`, `IOStreams`, `Runner`, `RunnerFunc`, and `systemRunner` (with one-line function comments per coding rules).
   - `bootstrap_helpers.go` — SSH/scp builders and `randomHexString`, exported only within the package.
   - `workstation_config.go` — `configureWorkstationOptions`, `configureWorkstation`, OS-specific CA installers, resolver helpers, `promptYesNo`, and `runCommand` with tightened logging comments.
3. Move code verbatim, adjusting only import blocks and comments required for lint compliance; avoid renaming exported identifiers to keep consumers untouched.
4. Ensure each new file starts with a brief package comment describing its scope, and keep helper visibility the same (`unexported`).
5. Update any affected tests or build tags only if compilation errors surface after the split (expected minimal).

## Files / docs touched
- [`internal/deploy/bootstrap.go`](../../../internal/deploy/bootstrap.go) — shrink to orchestration-only content or, if clearer, leave thin stubs delegating into new files.
- [`internal/deploy/bootstrap_types.go`](../../../internal/deploy/bootstrap_types.go) — new.
- [`internal/deploy/bootstrap_helpers.go`](../../../internal/deploy/bootstrap_helpers.go) — new.
- [`internal/deploy/workstation_config.go`](../../../internal/deploy/workstation_config.go) — new.
- [`docs/design/QUEUE.md`](../QUEUE.md) — entry already flipped for this doc.

## COSMIC evaluation

| Functional process | E | X | R | W | CFP |
|--------------------|---|---|---|---|-----|
| Reorganize bootstrap helpers/types into cohesive files | 0 | 0 | 0 | 0 | 0 |
| TOTAL              | 0 | 0 | 0 | 0 | 0 |

Assumptions: this is an internal refactor; no new control-plane or CLI behaviours and no additional env vars.

## How to test
- `go test ./internal/deploy/...` ensures the bootstrap and provisioning packages compile and keep ≥60%/≥90% coverage thresholds.
