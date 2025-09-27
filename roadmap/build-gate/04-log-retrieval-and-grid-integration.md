# Log Retrieval & Grid Integration
- [ ] Pending

## Why / What For
Fetch build logs from Grid artifact streams (or IPFS fallback) and classify errors for Knowledge Base integration, completing the build gate workflow across workstation and Grid modes.

## Required Changes
- Implement Grid artifact retrieval for build gate logs with IPFS fallback in offline mode.
- Add log parsers that normalise compiler/dependency errors into Knowledge Base codes.
- Surface remediation hints in checkpoint metadata and CLI summaries.

## Definition of Done
- Runner downloads build logs for failed stages in both Grid and stub modes.
- Parsed errors map to Knowledge Base codes and appear in checkpoint metadata.
- CLI summaries display actionable remediation (with overrides to suppress output when desired).

## Tests
- Unit tests for log parser fixtures covering compiler, dependency, and infrastructure errors.
- Grid stub tests verifying artifact retrieval paths populate metadata.
- `go test -cover ./...` maintains coverage thresholds.
