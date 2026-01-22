# Stack Gate — Phase 3: Build Gate Image Mapping Resolver

Scope: Add an explicit stack→image mapping resolver used by Stack Gate to select the Build Gate runtime image from the expected stack (no tool-based defaults when enabled).

Documentation: `design/stack-gate.md` (“Build Gate image mapping”), `design/build-gate-images.default.yaml` (illustrative), `internal/workflow/runtime/step/gate_docker.go` (current behavior).

Legend: [ ] todo, [x] done.

## Schema and resolution rules
- [x] Define typed image mapping rule schema — Makes configuration parseable and validateable.
  - Repository: ploy
  - Component: `internal/workflow/contracts` (schema), `internal/workflow/runtime/step` (consumer)
  - Scope:
    - Add `BuildGateImageRule{Stack contracts.StackExpectation, Image string}` (YAML/JSON) for:
      - `/etc/ploy/gates/build-gate-images.yaml` (default file)
      - `build_gate.images` (Mods spec; mod-level overrides)
    - Validation rules:
      - `stack.language` required
      - `stack.release` required
      - `stack.tool` optional (tool-agnostic rules allowed)
      - `image` required
  - Snippets: `images: [{ stack: { language: java, tool: maven, release: "11" }, image: "docker.io/org/..." }]`
  - Tests: `go test ./... -run BuildGateImageRule` — invalid entries rejected with stable errors.
- [x] Implement “most specific match wins” resolution — Ensures deterministic selection for tool-specific vs tool-agnostic rules.
  - Repository: ploy
  - Component: Build Gate runtime / Stack Gate
  - Scope: Resolver matches `language+tool+release` over `language+release`; ties at same precedence level are configuration errors.
  - Snippets: Expected `{language:java,tool:maven,release:"11"}` resolves to tool-specific rule if present.
  - Tests: `go test ./... -run ResolveBuildGateImage` — specificity and tie errors covered.

## Loading and precedence
- [x] Load default mapping from `/etc/ploy/gates/build-gate-images.yaml` — Removes implicit defaults when Stack Gate is enabled.
  - Repository: ploy
  - Component: `internal/workflow/runtime/step`
  - Scope: File loader + parse with clear error on missing/invalid file when required by an enabled phase (`StepGateSpec.StackGate.Enabled == true`).
  - Snippets: N/A
  - Tests: `go test ./... -run BuildGateImagesFile` — missing file triggers reject only when Stack Gate needs it.
- [x] Merge overrides with explicit precedence — Allows mod overrides without ambiguity.
  - Repository: ploy
  - Component: `internal/workflow/contracts` (Mods spec), `internal/workflow/runtime/step` (merge)
  - Scope:
    - Merge order: default file < mod override (`build_gate.images`).
    - Reject duplicates within the same precedence level when they are equal-specificity matches for the same stack selector:
      - tool-specific selector: `{language, tool, release}`
      - tool-agnostic selector: `{language, release}` (tool empty)
  - Snippets: N/A
  - Tests: `go test ./... -run BuildGateImagesMerge` — precedence and duplicate detection.
