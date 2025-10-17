# Log Retrieval & Grid Integration

- [x] Completed 2025-09-27

## Why / What For

Fetch build logs from Grid artifact streams (or IPFS fallback) and classify
errors for Knowledge Base integration, completing the build gate workflow across
workstation and Grid modes.

## Required Changes

- Implement Grid artifact retrieval for build gate logs with IPFS fallback in
  offline mode.
- Add log parsers that normalise compiler/dependency errors into Knowledge Base
  codes.
- Surface remediation hints in checkpoint metadata and CLI summaries.

## Definition of Done

- Runner downloads build logs for failed stages in both Grid and stub modes.
- Parsed errors map to Knowledge Base codes and appear in checkpoint metadata.
- CLI summaries display actionable remediation (with overrides to suppress
  output when desired).

## Implementation Notes

- Added `internal/workflow/buildgate.LogRetriever` with configurable artifact
  fetchers (`LogSourceGrid`, `LogSourceIPFS`, `LogSourceStub`) and truncation
  safeguards so workstation tests and Grid runs download logs consistently while
  producing deterministic SHA-256 digests.
- Introduced `internal/workflow/buildgate.LogIngestor` and `DefaultLogParser`,
  mapping canonical Knowledge Base codes for Git authentication failures, Go
  module conflicts, linker resolution issues, and disk pressure into
  `Metadata.LogFindings` for checkpoint publication.
- Extended `Metadata.Sanitize` to normalise log findings alongside static check
  results, ensuring downstream consumers receive trimmed codes, evidence, and
  severity values.
- Documentation now cross-references the retriever/ingestor components and
  records the milestone in the build gate design index and workstation roadmap tracker.

## Tests

- `internal/workflow/buildgate/log_retriever_test.go` verifies primary/fallback
  ordering, truncation behaviour, and digest calculation.
- `internal/workflow/buildgate/log_parser_test.go` covers canonical Knowledge
  Base mappings and duplicate suppression.
- `internal/workflow/buildgate/log_ingestion_test.go` exercises retriever/parser
  wiring and default parser fallback, while updated metadata sanitiser tests
  confirm finding normalisation.
- `go test -cover ./...` maintains coverage thresholds.

## References

- Build Gate design (`docs/design/build-gate/README.md`).
- Grid log streaming design (`../grid/docs/design/log-streaming/README.md`).
- Grid Workflow RPC helper guide (`../grid/sdk/workflowrpc/README.md`).
