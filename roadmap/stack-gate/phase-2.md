# Stack Gate — Phase 2: Stack Detector (Java MVP)

Status: **Planned (not implemented)**

## Goal

Implement deterministic, filesystem-only stack detection for Java (Maven + Gradle) with explicit “unknown vs mismatch” classification.

## What remains unchanged

- Gate execution behavior and image selection remain unchanged until Phase 4 (`internal/workflow/runtime/step/gate_docker.go`).

## Compatibility impact

- None required (new package only; no behavior wired yet).

## Implementation steps (RED → GREEN → REFACTOR)

1. Add detector package:
   - Create `internal/workflow/stackdetect/` with:
     - `detect.go` (public `Detect(ctx, workspace)`)
     - `types.go` (`Observation`, `EvidenceItem`)
     - `errors.go` (typed error for “unknown/ambiguous”)
2. Implement Java/Maven detection:
   - Parse `pom.xml` for `maven.compiler.release`, `maven.compiler.source`/`target`, `java.version` with strict precedence.
   - Implement local parent resolution via `<parent><relativePath>` when file exists under workspace.
   - If multiple modules disagree or placeholders can’t be resolved → return “unknown” with evidence.
3. Implement Java/Gradle detection:
   - Parse `build.gradle` / `build.gradle.kts` for explicit toolchain / compatibility declarations only (static forms).
   - Dynamic logic → “unknown” with evidence.
4. Tests:
   - Add fixture workspaces under `internal/workflow/stackdetect/testdata/`.
   - Add unit tests in `internal/workflow/stackdetect/*_test.go` covering:
     - each precedence rule
     - property interpolation (local only)
     - ambiguous/mixed declarations → unknown

