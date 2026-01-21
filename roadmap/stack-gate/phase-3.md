# Stack Gate — Phase 3: Build Gate Image Mapping Resolver

Scope: Add an explicit stack→image mapping resolver used by Stack Gate to select the Build Gate runtime image from the expected stack (no tool-based defaults when enabled).

Documentation: `design/stack-gate.md` (“Build Gate image mapping”), `design/build-gate-images.default.yaml` (illustrative), `internal/workflow/runtime/step/gate_docker.go` (current behavior).

Legend: [ ] todo, [x] done.

## Schema and resolution rules
- [ ] Define typed image mapping rule schema — Makes configuration parseable and validateable.
  - Repository: ploy
  - Component: config/contracts (TBD), Build Gate runtime
  - Scope: Add `BuildGateImageRule{StackExpectation, Image}` and validation (image non-empty, language non-empty).
  - Snippets: `images: [{ stack: { language: java, tool: maven, release: "11" }, image: "docker.io/org/..." }]`
  - Tests: `go test ./... -run BuildGateImageRule` — invalid entries rejected with stable errors.
- [ ] Implement “most specific match wins” resolution — Ensures deterministic selection for tool-specific vs tool-agnostic rules.
  - Repository: ploy
  - Component: Build Gate runtime / Stack Gate
  - Scope: Resolver matches `language+tool+release` over `language+release`; ties at same precedence level are configuration errors.
  - Snippets: Expected `{language:java,tool:maven,release:"11"}` resolves to tool-specific rule if present.
  - Tests: `go test ./... -run ResolveBuildGateImage` — specificity and tie errors covered.

## Loading and precedence
- [ ] Load default mapping from `/etc/ploy/gates/build-gate-images.yaml` — Removes implicit defaults when Stack Gate is enabled.
  - Repository: ploy
  - Component: nodeagent/gate runtime
  - Scope: File loader + parse with clear error on missing/invalid file when required by an enabled phase.
  - Snippets: N/A
  - Tests: `go test ./... -run BuildGateImagesFile` — missing file triggers reject only when Stack Gate needs it.
- [ ] Merge overrides with explicit precedence — Allows cluster and mod overrides without ambiguity.
  - Repository: ploy
  - Component: nodeagent config + Mods spec
  - Scope: Merge order: default file < cluster inline (`gates.build_gate.images`, if present in config model) < mod override (`build_gate.images` in spec); same-specificity duplicates within one precedence level reject.
  - Snippets: N/A
  - Tests: `go test ./... -run BuildGateImagesMerge` — precedence and duplicate detection.
