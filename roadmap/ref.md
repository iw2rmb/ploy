# Internal Workflow Redundancy / Overengineering Refactor

Scope: remove duplicated gate flow logic, centralize file-existence helpers, eliminate repeated `pyproject.toml` reads, flatten gate planning control flow, and replace panic-based `ModsSpec.ToMap()` with error-returning API.

Documentation: `AGENTS.md`; `internal/workflow/step/runner.go`; `internal/workflow/step/gate_only.go`; `internal/workflow/step/gate_docker.go`; `internal/workflow/step/gate_docker_stack_gate.go`; `internal/workflow/stackdetect/detector.go`; `internal/workflow/stackdetect/python.go`; `internal/workflow/stackdetect/maven.go`; `internal/workflow/stackdetect/rust.go`; `internal/workflow/contracts/mods_spec_wire.go`; `docs/build-gate/README.md`; `docs/migs-lifecycle.md`

Legend: [ ] todo, [x] done.

## Phase 1: Deduplicate Gate Stage Flow
- [x] Extract shared hydration and gate execution stage helpers.
  - Repository: `ploy`
  - Component: `internal/workflow/step`
  - Scope:
    - implement one private hydration helper used by both `Runner.Run` and `RunGateOnly`
    - implement one private gate helper that:
      - calls `Gate.Execute`
      - stores metadata
      - applies `StaticChecks[0].Passed` rule
      - returns wrapped `ErrBuildGateFailed` with caller-provided message
    - keep timing field behavior unchanged
  - Files:
    - `internal/workflow/step/runner.go`
    - `internal/workflow/step/gate_only.go`
    - optional new helper file: `internal/workflow/step/runner_gate_stage.go`
  - Tests:
    - `go test ./internal/workflow/step -run 'TestRun|TestRunGateOnly'`

## Phase 2: Centralize File Existence Utility
- [x] Replace duplicated `fileExists` functions with one shared helper.
  - Repository: `ploy`
  - Component: `internal/workflow`
  - Scope:
    - add `internal/workflow/fsutil/file.go` with `FileExists(path string) bool`
    - remove package-local `fileExists` from:
      - `internal/workflow/step/gate_docker.go`
      - `internal/workflow/stackdetect/detector.go`
    - update all call sites in `step` and `stackdetect`
  - Tests:
    - `go test ./internal/workflow/step ./internal/workflow/stackdetect`

## Phase 3: Eliminate Repeated `pyproject.toml` Reads
- [x] Move pyproject parsing to scan phase and reuse cached result.
  - Repository: `ploy`
  - Component: `internal/workflow/stackdetect`
  - Scope:
    - extend `scanResult` with pyproject parse/cache fields
    - read `pyproject.toml` once in `scanWorkspace`
    - reuse cached data in:
      - `DetectTool` tool selection (`pip` vs `poetry`)
      - Python detection flow
    - remove duplicated Poetry checks and repeated file reads
    - preserve precedence and error semantics
  - Files:
    - `internal/workflow/stackdetect/detector.go`
    - `internal/workflow/stackdetect/python.go`
  - Tests:
    - `go test ./internal/workflow/stackdetect -run 'TestDetectTool|TestDetectPython|TestDetector'`

## Phase 4: Flatten Gate Plan Control Flow
- [ ] Simplify stack-gate execution planning internals without behavior changes.
  - Repository: `ploy`
  - Component: `internal/workflow/step`
  - Scope:
    - reduce terminal-state wrapper layering in `gate_docker_stack_gate.go`
    - consolidate repeated failure metadata builders into smaller focused helpers
    - keep existing error codes/messages and `RuntimeImage` propagation intact
  - Files:
    - `internal/workflow/step/gate_docker_stack_gate.go`
    - `internal/workflow/step/gate_docker.go` (minimal wiring updates only)
  - Tests:
    - `go test ./internal/workflow/step -run 'TestDockerGate|TestGate|TestResolve'`

## Phase 5: Replace Panic-Based `ModsSpec.ToMap`
- [ ] Change `ModsSpec.ToMap()` to return `(map[string]any, error)` and update call sites.
  - Repository: `ploy`
  - Component: `internal/workflow/contracts` + direct callers
  - Scope:
    - update `ModsSpec.ToMap()` signature and implementation
    - remove panic path
    - update tests and all `.ToMap()` callers to handle error explicitly
  - Files:
    - `internal/workflow/contracts/mods_spec_wire.go`
    - all `ModsSpec.ToMap()` call sites found by `rg -n "\\.ToMap\\(\\)"`
  - Tests:
    - `go test ./internal/workflow/contracts`
    - `go test ./...` (compile guard for all call sites)

## Phase 6: Docs and Final Verification
- [ ] Sync docs for changed internals and run full validation suite.
  - Repository: `ploy`
  - Component: docs + workflow packages
  - Scope:
    - update `docs/build-gate/README.md` to reflect simplified internal gate planning flow
    - update `docs/migs-lifecycle.md` only where gate execution path wording depends on old duplication
    - keep behavior contract docs unchanged where runtime behavior did not change
  - Validation:
    - `make test`
    - `make vet`
    - `make staticcheck`

## Open Questions
- None.
