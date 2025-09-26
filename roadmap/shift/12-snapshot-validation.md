# Snapshot Validation Matrix
- [x] Done (2025-09-26)

## Why / What For
Prove the snapshot toolkit can service the representative databases we expect in the Grid era (Postgres, MySQL, and document stores) and keep fixtures aligned with CLI commands ahead of live JetStream/IPFS wiring.

## Required Changes
- Extend built-in fixtures under `configs/snapshots/` to include MySQL and document-store examples alongside the existing Postgres captures.
- Support masking strategies required by those fixtures, including `last4` for partially redacted phone numbers.
- Add regression coverage that loads the real snapshot catalog and executes `Plan`/`Capture` for every spec using the stub publishers.

## Definition of Done
- `ploy snapshot plan|capture` succeeds against all in-repo specs for Postgres, MySQL, and document-store fixtures without manual edits.
- Masking rules report consistent metadata and fingerprints, including masked outputs for `last4` fields.
- Design doc “Next Steps” marks the validation task as complete so later slices can focus on JetStream/Grid wiring.

## Tests
- `go test ./internal/workflow/snapshots` covering the built-in catalog regression.
- Repository-wide `go test -cover ./...` to maintain ≥60% overall and ≥90% on the snapshot package.
