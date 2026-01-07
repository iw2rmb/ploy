# Workflow Refactor Notes (internal/workflow)

## Type Hardening

- Reject fractional numbers when parsing spec integers.
  - `internal/workflow/contracts/mods_spec.go:381` (`mod_index`): `float64` is truncated to `int`.
  - `internal/workflow/contracts/mods_spec.go:600` (`build_gate.healing.retries`): `float64` is truncated to `int`.
- Preserve explicit `retries: 0` on round-trip.
  - `internal/workflow/contracts/mods_spec.go:783`: retries only serialized when `> 0`, so `0` is dropped and re-parses as default `1`.
- Reduce “stringly typed” fields with small enums/newtypes.
  - `internal/workflow/contracts/build_gate_metadata.go:98` (`BuildGateLogFinding.Severity`).
  - `internal/workflow/contracts/job_meta.go:75` (`BuildMeta.Metrics map[string]interface{}`).
  - `internal/workflow/contracts/contracts.go:31` (`SubjectsForRun(runID string)` should likely take `types.RunID`).

## Algorithms / Simplifications

- Make diff path normalization safer.
  - `internal/workflow/runtime/step/stub.go:197`: `normalizeDiffPaths` uses `strings.ReplaceAll` on the full diff body; it can corrupt hunks if `baseDir` text appears in file content. Prefer rewriting only header/path lines (`diff --git`, `---`, `+++`).
- Make graph ordering deterministic and defensive.
  - `internal/workflow/graph/types.go:206`: sort key is only `StepIndex`; equal indices become nondeterministic (map iteration). Add a tie-breaker (e.g., job ID) and/or detect duplicate step indices.
  - `internal/workflow/graph/types.go:143`: `AddNode` overwrites on duplicate node ID without error.
- Align container mount behavior to manifest semantics.
  - `internal/workflow/runtime/step/container_spec.go:55`: always mounts `Inputs[0]` RW and ignores `StepInput.Mode`; additional inputs are ignored.

## Likely Bugs / Risks

- Docker wait select handling.
  - `internal/workflow/runtime/step/container_docker.go:178`: `select` on `waitResult.Error` can receive `nil` (closed channel) and fall through to “wait interrupted”. Prefer selecting with `err, ok := <-ch` or checking both channels in a loop.
- Gate pass logic only checks first static check.
  - `internal/workflow/runtime/step/stub.go:357`: assumes `StaticChecks[0].Passed` represents overall success; future multi-check gates likely need `all(check.Passed)` semantics.
