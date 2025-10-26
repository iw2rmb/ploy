# SSH Transport Manager Split

## Why
- `pkg/sshtransport/manager.go` has 445 LOC that conflate type definitions, cache helpers, exported manager APIs, and tunnel lifecycle logic, making it hard to reason about.
- The monolithic layout hampers incremental evolution (e.g., alternate cache implementations or tunnel backoff tuning) because unrelated concerns live side-by-side.
- Several unexported helpers lack focused tests or comments; isolating them into cohesive files makes enforcement of the "comment every function" rule and targeted tests practical.

## What to do
1. Extract type and collaborator declarations into `pkg/sshtransport/types.go`, retaining `Node`, `Config`, interfaces, `noopCache`, and `normaliseNode` so they can evolve independently.
2. Keep `pkg/sshtransport/manager.go` focused on the `Manager` struct, constructor, and exported APIs (`SetNodes`, `HasTargets`, `Close`, `DialContext`), trimming the file to <250 LOC.
3. Move tunnel lifecycle helpers (`ensureTunnel`, `observe`, `registerFailure`, `backoffDuration`, `allocateLocal`) and the `tunnelState` struct into `pkg/sshtransport/manager_tunnels.go`, adding one-line comments per function and grouping retry/backoff logic.
4. Touch `pkg/sshtransport/manager_test.go` only to adjust imports or helper references if package-level items move; keep behavior identical while covering any newly exported helpers if needed.
5. Update documentation only if public API comments change; no control-plane API surface is affected.

## Where to change
- [`pkg/sshtransport/manager.go`](../../pkg/sshtransport/manager.go) — shrink to exported manager construction + orchestration.
- [`pkg/sshtransport/manager_tunnels.go`](../../pkg/sshtransport/manager_tunnels.go) — new file owning tunnel lifecycle helpers.
- [`pkg/sshtransport/types.go`](../../pkg/sshtransport/types.go) — new file for shared structs and interfaces used by tests and other packages.
- [`pkg/sshtransport/manager_test.go`](../../pkg/sshtransport/manager_test.go) — ensure tests still compile and, if necessary, add focused cases for helper files.

## COSMIC evaluation
- Conceptual complexity: 2 (straightforward file splits, no new behavior).
- Operations: 0 (no external systems touched).
- Size: 1 (two new files, light edits elsewhere).
- Migration: 0 (no data/state migration).
- Integration: 0 (no new dependencies).
- Confidence: 1 (existing tests provide coverage once updated).
- **Total: 4** — within the allowed scope.

## How to test
1. `go test ./pkg/sshtransport` — fast validation specific to the package.
2. `make test` — repository-wide guardrail covering downstream imports and ensuring coverage stays ≥60% overall.
