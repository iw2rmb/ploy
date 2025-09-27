# Build Gate Runner Orchestration
- [x] Completed 2025-09-27

## Why / What For
Unify sandbox execution, static check orchestration, and log ingestion behind a single API so workflow stages and Mods healing flows can consume consistent build gate outcomes and metadata.

## Required Changes
- Introduce `buildgate.Runner` to coordinate the sandbox runner, static check registry, and log ingestor while capturing outcomes in a single structure.
- Produce sanitised `buildgate.Metadata` that prefers ingested log digests/findings when available and mirrors static check reports into checkpoint metadata.
- Expose typed errors when dependencies (sandbox runner, static check registry, log ingestor) are missing so misconfiguration is caught during RED-phase tests.

## Definition of Done
- `buildgate.Runner.Run` returns sandbox outcomes, static check reports, optional log ingestion details, and sanitised metadata suitable for checkpoint publication.
- Metadata sanitisation trims and filters results before returning, keeping static check reports and log findings aligned with checkpoint schema expectations.
- Documentation references the runner milestone across the build gate design record, design index, and SHIFT tracker.

## Tests
- `internal/workflow/buildgate/runner_test.go` covers dependency validation, end-to-end aggregation of sandbox/static-check/log results, and metadata sanitisation.
- Repository-wide `go test -cover ./...` remains green with ≥90% coverage for `internal/workflow/buildgate`.
