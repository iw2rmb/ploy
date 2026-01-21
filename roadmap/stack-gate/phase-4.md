# Stack Gate — Phase 4: Gate Integration (Inbound/Outbound)

Status: **Planned (not implemented)**

## Goal

Wire Stack Gate into the node agent gate execution: detect declared stack, enforce expectations, select the Build Gate runtime image from expectations, and persist stack gate metadata.

## What remains unchanged

- Mods image stack-selection remains based on Build Gate tool detection until Stack Gate becomes the canonical mechanism (see `docs/mods-lifecycle.md` → “## 1.2 Stack-Aware Image Selection”).

## Compatibility impact

- When Stack Gate is enabled for a phase, tool-detection-based image selection is not used for that phase (no fallback).

## Implementation steps (RED → GREEN → REFACTOR)

1. Extend gate contracts and metadata:
   - Update `internal/workflow/contracts/build_gate_metadata.go`:
     - Add `stack_gate.enabled`, `stack_gate.expected`, `stack_gate.detected`, `stack_gate.runtime_image`, `stack_gate.result`.
2. Add Stack Gate execution to the Docker gate executor:
   - Update `internal/workflow/runtime/step/gate_docker.go` to:
     - read the effective expectation from `contracts.StepGateSpec`
     - run `stackdetect.Detect(workspace)`
     - match detected vs expected (mismatch vs unknown vs pass)
     - resolve runtime image using Phase 3 resolver
     - run the build under the resolved image when match passes
   - Ensure mismatch/unknown are reported as policy failures distinct from build failures.
3. Remove implicit defaults when Stack Gate is enabled:
   - In `internal/workflow/runtime/step/gate_docker.go`, for Stack Gate-enabled phases:
     - do not use `maven:3-eclipse-temurin-17` / `gradle:8.8-jdk17` fallbacks
     - do not choose image by `pom.xml`/`build.gradle` markers
4. Tests:
   - Add/extend unit tests in `internal/workflow/runtime/step/gate_docker_test.go` to cover:
     - mismatch/unknown early failure (no container run)
     - image selection from expectation mapping
     - metadata fields populated deterministically

