# Workflow Refactor Notes (internal/workflow)

Cross-cutting contract decisions live in `roadmap/refactor/contracts.md` (IDs, StepIndex).

## Type Hardening

- Remove `mod_index` from the external Mods spec contract entirely.
  - `mod_index` must be assigned internally only; it must never be accepted from YAML/spec input.
  - Delete parsing/serialization support and reject specs that include it (`internal/workflow/contracts/mods_spec.go:381`).
  - Solution: remove `mod_index` parsing/serialization and treat its presence as a validation error; assign any needed ordering internally (not from YAML).
- Reject fractional numbers when parsing spec integers.
  - `internal/workflow/contracts/mods_spec.go:600` (`build_gate.healing.retries`): `float64` is truncated to `int`.
  - Prefer `types.IntFromAny` / `types.Int64FromAny` (`internal/domain/types/numbers.go`) when parsing map-backed JSON/YAML numbers to reject fractional inputs instead of truncating.
  - Solution: replace all `int(float64)` truncations in spec parsing with `types.IntFromAny` (and return a field-path error on failure).
- Preserve explicit `retries: 0` on round-trip.
  - `internal/workflow/contracts/mods_spec.go:783`: retries only serialized when `> 0`, so `0` is dropped and re-parses as default `1`.
  - Solution: represent retries as `*int` (distinguish unset vs 0) or implement custom marshal/unmarshal so that an explicit `0` remains explicit.
- Reduce “stringly typed” fields with small enums/newtypes.
  - `internal/workflow/contracts/build_gate_metadata.go:98` (`BuildGateLogFinding.Severity`).
  - `internal/workflow/contracts/job_meta.go:75` (`BuildMeta.Metrics map[string]interface{}`).
  - `internal/workflow/contracts/contracts.go:31` (`SubjectsForRun(runID string)` should take `types.RunID` from `internal/domain/types/ids.go`).
  - Server-injected spec fields like `job_id` should be `types.JobID` (not `string`) and validated on parse (`internal/workflow/contracts/mods_spec.go:372`).
  - Solution: follow `roadmap/refactor/contracts.md` § "IDs and Newtypes (`internal/domain/types`)".

## Algorithms / Simplifications

- Make diff path normalization safer.
  - `internal/workflow/runtime/step/stub.go:197`: `normalizeDiffPaths` uses `strings.ReplaceAll` on the full diff body; it can corrupt hunks if `baseDir` text appears in file content. Prefer rewriting only header/path lines (`diff --git`, `---`, `+++`).
  - Solution: parse the diff as lines and only rewrite recognized header/path lines; never rewrite arbitrary hunk bodies.
- Make graph ordering deterministic and defensive.
  - `internal/workflow/graph/types.go:206`: sort key is only `StepIndex`; equal indices become nondeterministic (map iteration). Add a tie-breaker (e.g., job ID) and/or detect duplicate step indices.
  - `internal/workflow/graph/types.go:143`: `AddNode` overwrites on duplicate node ID without error.
  - Solution: follow `roadmap/refactor/contracts.md` § "StepIndex (Ordering Invariant)" (deterministic tie-breakers; no silent overwrite).
- Align container mount behavior to manifest semantics.
  - `internal/workflow/runtime/step/container_spec.go:55`: always mounts `Inputs[0]` RW and ignores `StepInput.Mode`; additional inputs are ignored.
  - Solution: implement `StepInput.Mode` for each input (mount all inputs with correct RO/RW), and add a unit test that asserts mount flags for multiple inputs.

## Likely Bugs / Risks

- Docker wait select handling.
  - `internal/workflow/runtime/step/container_docker.go:178`: `select` on `waitResult.Error` can receive `nil` (closed channel) and fall through to “wait interrupted”. Prefer selecting with `err, ok := <-ch` or checking both channels in a loop.
  - Solution: handle channel close explicitly (`err, ok := <-ch`) and prioritize the result channel over ctx cancellation once a result is available.
- Gate pass logic only checks first static check.
  - `internal/workflow/runtime/step/stub.go:357`: assumes `StaticChecks[0].Passed` represents overall success; future multi-check gates likely need `all(check.Passed)` semantics.
  - Solution: treat gate pass as `all(Passed)` and handle empty checks defensively (explicit policy: pass/fail).
