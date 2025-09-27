# Static Check Adapter Registry
- [ ] Pending

## Why / What For
Wire language-specific static analysis adapters (Go vet/staticcheck, Java Error Prone, ESLint, Ruff, Roslyn) so build gate runs can aggregate diagnostics across stacks.

## Required Changes
- Implement a `StaticCheckRegistry` mapping languages to adapter implementations executed inside the build gate stage.
- Provide configuration hooks for repo manifests to enable/disable adapters and set severity thresholds.
- Ensure adapter output is normalised into the build gate metadata schema for checkpoint publication.

## Definition of Done
- Registry dispatches adapters based on repo manifest configuration and lane defaults.
- Failing diagnostics populate checkpoint metadata with language/tool/severity information.
- Repository documentation explains adapter configuration and CLI overrides.

## Tests
- Unit tests per adapter verifying diagnostic parsing and severity mapping.
- Integration-style tests ensuring metadata aggregation works across multiple adapters.
