# Stack Gate — Phase 2: Stack Detector (Java MVP)

Scope: Implement deterministic, filesystem-only detection for declared Java stack (Maven/Gradle + release), including evidence reporting and an explicit “unknown” outcome for ambiguous workspaces.

Documentation: `design/stack-gate.md`, `internal/workflow/stackdetect` (new), `design/java-version-detect.sh` (reference).

Legend: [ ] todo, [x] done.

## Detector framework
- [ ] Create `internal/workflow/stackdetect` package — Provides a single entrypoint for stack detection used by Stack Gate.
  - Repository: ploy
  - Component: `internal/workflow/stackdetect`
  - Scope: Add `Detect(ctx, workspace)`, `Observation`, `EvidenceItem`, and typed errors for “unknown/ambiguous”.
  - Snippets: `Observation{Language:"java", Tool:"maven", Release:ptr("17"), Evidence:[...] }`
  - Tests: `go test ./internal/workflow/stackdetect -run Detect` — empty workspace returns `unknown` deterministically.

## Java/Maven detection
- [ ] Detect Maven Java release from `pom.xml` — Enables Stack Gate to validate declared baseline/target for Maven projects.
  - Repository: ploy
  - Component: `internal/workflow/stackdetect`
  - Scope: Parse (strict precedence) `maven.compiler.release`, then `maven.compiler.source/target`, then `java.version`; implement local parent `<relativePath>` property resolution only.
  - Snippets: Evidence items like `{path:"pom.xml", key:"maven.compiler.release", value:"11"}`
  - Tests: `go test ./internal/workflow/stackdetect -run Maven` — precedence + local parent resolution + “unknown” on unresolved placeholders.

## Java/Gradle detection
- [ ] Detect Gradle Java release from `build.gradle(.kts)` — Supports Gradle projects with explicit toolchain declarations.
  - Repository: ploy
  - Component: `internal/workflow/stackdetect`
  - Scope: Recognize only explicit/static `JavaLanguageVersion.of(N)` and `sourceCompatibility/targetCompatibility` forms; dynamic logic returns “unknown”.
  - Snippets: Evidence items like `{path:"build.gradle.kts", key:"toolchain.languageVersion", value:"17"}`
  - Tests: `go test ./internal/workflow/stackdetect -run Gradle` — static forms pass; dynamic forms classify as unknown.

## Fixtures and test coverage
- [ ] Add fixture workspaces under `testdata/` — Keeps detection tests stable and realistic.
  - Repository: ploy
  - Component: `internal/workflow/stackdetect`
  - Scope: `internal/workflow/stackdetect/testdata/` with minimal Maven/Gradle variants (properties, parents, ambiguous cases).
  - Snippets: N/A
  - Tests: `go test ./internal/workflow/stackdetect` — all detector tests pass and are deterministic.
