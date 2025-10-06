# Static Check Adapter Registry

- [x] Completed 2025-10-05

## Why / What For

Wire language-specific static analysis adapters (Go vet/staticcheck, Java Error
Prone, ESLint, Ruff, Roslyn) so build gate runs can aggregate diagnostics across
stacks.

## Required Changes

- Implement a `StaticCheckRegistry` mapping languages to adapter implementations
  executed inside the build gate stage.
- Provide configuration hooks for repo manifests to enable/disable adapters and
  set severity thresholds.
- Ensure adapter output is normalised into the build gate metadata schema for
  checkpoint publication.

## Definition of Done

- Registry dispatches adapters based on lane defaults (`StaticCheckLaneConfig`),
  manifest overrides (`StaticCheckManifest`), and CLI skip hooks exposed through
  `StaticCheckSpec`.
- Failing diagnostics populate checkpoint metadata with language/tool/severity
  information using normalized `StaticCheckReport` entries.
- Repository documentation explains adapter configuration and CLI overrides
  across `docs/design/build-gate/README.md` and the design index.

## Tests

- `internal/workflow/buildgate/static_checks_test.go` exercises registry
  planning, severity thresholds, manifest overrides, and skip controls with fake
  adapters.
- Adapter-specific fixtures remain planned for future slices once language
  adapters land.
