# Internal Workflow Cleanup (Redundancy and Boilerplate Reduction)

Scope: Remove duplicated parsing/building logic and simplify over-abstracted paths in `internal/workflow/**` while preserving behavior. The plan focuses on architecture-level reuse, smaller API surface, and lower maintenance cost.

Documentation: `AGENTS.md`; `docs/testing-workflow.md`; `internal/workflow/contracts/*`; `internal/workflow/step/*`; `internal/workflow/stackdetect/*`; `internal/workflow/manifests/*`

Legend: [ ] todo, [x] done.

## Phase 0: Safety Rails and RED
- [x] Add characterization tests for affected contracts before refactors — Locks current behavior before structural cleanup.
  - Repository: `ploy`
  - Component: `internal/workflow/contracts`, `internal/workflow/step`, `internal/workflow/stackdetect`, `internal/workflow/manifests`
  - Scope: Add/extend tests for stack-gate terminal metadata shape, release coercion edge-cases, command polymorphism, manifest decode/validation errors, and Java ambiguity behavior.
  - Snippets: `go test ./internal/workflow/... -run 'StackGate|Parse|Manifest|Detect'`
  - Tests: `make test` — New tests fail first where behavior is not yet preserved by shared helpers.

## Phase 1: Contracts Parser Consolidation
- [x] Centralize release value coercion into one helper used by all parsers — Eliminates duplicated number/string coercion logic.
  - Repository: `ploy`
  - Component: `internal/workflow/contracts`, `internal/workflow/step`
  - Scope: Extract a single `parseReleaseValue` helper in `contracts`, use it from `stack_gate_spec_parse.go`, `mods_spec_parse.go` build-gate stack parsing, and Build Gate YAML rule parsing in `step/build_gate_image_resolver.go` through contracts helper.
  - Snippets: `func ParseReleaseValue(v any, field string) (string, error)`
  - Tests: `go test ./internal/workflow/contracts ./internal/workflow/step` — Existing + new release coercion tests stay green.

- [x] Unify command polymorphic parsing into the `CommandSpec` type path — Removes dual command parse implementations.
  - Repository: `ploy`
  - Component: `internal/workflow/contracts`
  - Scope: Replace `parseCommandSpec` duplicate flow in `mods_spec_parse.go` with one canonical constructor/unmarshal helper owned by `command_spec.go`; keep identical error semantics.
  - Snippets: `func ParseCommandSpec(v any) (CommandSpec, error)`
  - Tests: `go test ./internal/workflow/contracts -run 'CommandSpec|ModsSpec'` — JSON/YAML/map-backed parsing parity verified.

- [x] Resolve numeric coercion helper drift by reusing or removing unused helpers — Shrinks API surface and avoids dead complexity.
  - Repository: `ploy`
  - Component: `internal/workflow/contracts`
  - Scope: Route integer extraction in spec parsing (`retries`, similar fields) through `intFromAny`/`int64FromAny`, or delete helpers if no production caller remains after refactor.
  - Snippets: `r, ok := intFromAny(v)`
  - Tests: `go test ./internal/workflow/contracts -run 'numbers|ModsSpec'` — No behavior regression on numeric edge cases.

## Phase 2: Step Package De-duplication
- [x] Introduce a shared terminal metadata builder for gate planning failures — Removes repeated `BuildGateStageMetadata` construction branches.
  - Repository: `ploy`
  - Component: `internal/workflow/step`
  - Scope: Refactor `gate_docker_stack_gate.go` to use a compact builder for `StaticChecks`, `LogFindings`, optional runtime image, and optional `StackGateResult` across all terminal-return branches.
  - Snippets: `newGateTerminal(language, tool, code, message, opts...)`
  - Tests: `go test ./internal/workflow/step -run 'Gate|StackGate'` — Terminal payloads remain byte-for-byte equivalent where expected.

- [x] Extract shared Build Gate limit/env parsing utilities — Replaces repeated env parse cascades.
  - Repository: `ploy`
  - Component: `internal/workflow/step`
  - Scope: In `gate_docker.go`, consolidate CPU/memory/disk env parsing into reusable helpers and keep one place for parse precedence (`RAMInBytes`, `FromHumanSize`, integer fallback).
  - Snippets: `parseBytesLimitEnv(key string) (int64, string)`
  - Tests: `go test ./internal/workflow/step -run 'Gate.*Limit|Docker'` — Existing limit parsing behavior preserved.

- [x] Convert cert mount option branches to table-driven mapping — Reduces repetitive mount code paths.
  - Repository: `ploy`
  - Component: `internal/workflow/step`
  - Scope: In `container_spec.go`, replace repeated option checks for `ploy_*_cert_path` with one table of `{optionKey,targetPath,readOnly}` entries.
  - Snippets: `[]struct{ key, target string; ro bool }`
  - Tests: `go test ./internal/workflow/step -run 'container_spec|mount'` — Mount set remains unchanged.

- [x] Replace function-wrapper diff generator with direct concrete implementation — Removes unnecessary abstraction layer.
  - Repository: `ploy`
  - Component: `internal/workflow/step`
  - Scope: Drop `diffGeneratorFuncs` indirection in `diff.go`; keep `DiffGenerator` interface but return a concrete struct with method implementations.
  - Snippets: `type filesystemDiffGenerator struct{}`
  - Tests: `go test ./internal/workflow/step -run 'Diff'` — Diff generation and normalization behavior unchanged.

## Phase 3: Manifests and Detection Cleanup
- [x] Share manifest decode+validate pipeline between file and registry loaders — Removes duplicate TOML/validation flow.
  - Repository: `ploy`
  - Component: `internal/workflow/manifests`
  - Scope: Extract helper used by both `LoadFile` and `loadDirectory` for read/decode/validate and standardized error wrapping.
  - Snippets: `func decodeAndValidateManifest(path string, label string) (rawManifest, error)`
  - Tests: `go test ./internal/workflow/manifests` — Error text and validation semantics preserved.

- [x] Unify Java ambiguity/tool selection logic across `Detect` and `DetectTool` — Prevents duplicate branch logic and message drift.
  - Repository: `ploy`
  - Component: `internal/workflow/stackdetect`
  - Scope: Extract shared Java tool resolution and ambiguity error builder used by both flows in `detector.go`.
  - Snippets: `func detectJavaTool(s scanResult) (tool string, err error)`
  - Tests: `go test ./internal/workflow/stackdetect -run 'Detect|Ambiguous|Java'` — Ambiguity evidence remains stable.

- [x] Reduce repetitive manifest conversion helpers with focused shared mappers — Cuts conversion boilerplate while keeping explicit types.
  - Repository: `ploy`
  - Component: `internal/workflow/manifests`
  - Scope: Introduce internal mapping helpers for repeated slice conversion patterns in `compilation.go` and `file.go` (flows/fixtures/lanes/exposures/requires) without changing wire structs.
  - Snippets: `mapSlice(in, func(T) U) []U`
  - Tests: `go test ./internal/workflow/manifests -run 'Encode|Compile|Registry'` — Encoded TOML and compiled payload parity maintained.

## Phase 4: GREEN and Verification
- [x] Run full workflow package validation after refactor — Confirms cleanup did not alter runtime semantics.
  - Repository: `ploy`
  - Component: `internal/workflow/**`
  - Scope: Execute unit tests, vet/static checks, and coverage checks for touched packages; fix regressions before merge.
  - Snippets: `make test`; `make vet`; `make staticcheck`; `make coverage`
  - Tests: All pass; coverage remains at or above project thresholds.
