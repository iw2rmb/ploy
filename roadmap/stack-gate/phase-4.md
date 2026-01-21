# Stack Gate — Phase 4: Gate Integration (Inbound/Outbound)

Scope: Execute Stack Gate before/after each step: detect declared stack, enforce expectations, resolve the gate runtime image from expectation mapping, and persist Stack Gate metadata as part of Build Gate results.

Documentation: `design/stack-gate.md`, `internal/workflow/runtime/step/gate_docker.go`, `internal/workflow/contracts/build_gate_metadata.go`, `internal/workflow/contracts/step_manifest.go`.

Legend: [ ] todo, [x] done.

## Contracts and metadata
- [ ] Extend Build Gate metadata to include Stack Gate fields — Preserves expected/detected stack and image selection for debugging and determinism.
  - Repository: ploy
  - Component: `internal/workflow/contracts`
  - Scope: `internal/workflow/contracts/build_gate_metadata.go` add `stack_gate.{enabled,expected,detected,runtime_image,result}` (exact struct layout TBD).
  - Snippets: `stack_gate: { enabled:true, result:"mismatch", expected:{...}, detected:{...} }`
  - Tests: `go test ./internal/workflow/contracts -run BuildGateStageMetadata` — validation and JSON stability for new fields.

## Gate execution behavior
- [ ] Add Stack Gate pre-check to Docker gate executor — Fails early on mismatch/unknown without running a build.
  - Repository: ploy
  - Component: `internal/workflow/runtime/step`
  - Scope: `internal/workflow/runtime/step/gate_docker.go` run `stackdetect.Detect(workspace)`, match vs expectation from `contracts.StepGateSpec`, classify `pass|mismatch|unknown`.
  - Snippets: N/A
  - Tests: `go test ./internal/workflow/runtime/step -run GateDocker.*StackGate` — mismatch/unknown do not execute containers.
- [ ] Resolve gate runtime image from expectation mapping — Ensures inbound/outbound run under the expected stack.
  - Repository: ploy
  - Component: `internal/workflow/runtime/step`
  - Scope: `internal/workflow/runtime/step/gate_docker.go` call Phase 3 resolver; reject when no rule resolves (no defaults).
  - Snippets: Expected `{language:java,tool:maven,release:"11"}` → image `...:11`.
  - Tests: `go test ./internal/workflow/runtime/step -run GateDocker.*ImageResolution` — selected image matches mapping.
- [ ] Disable tool-based image defaults for Stack Gate-enabled phases — Avoids “declared stack vs runtime image” drift.
  - Repository: ploy
  - Component: `internal/workflow/runtime/step`
  - Scope: `internal/workflow/runtime/step/gate_docker.go` bypass `pom.xml`/`build.gradle` image selection and `maven:...-17`/`gradle:...-jdk17` fallbacks when Stack Gate is enabled.
  - Snippets: N/A
  - Tests: `go test ./internal/workflow/runtime/step -run GateDocker.*NoDefaults` — enabled path never hits defaults.
