# roadmap-testing-hardening-10 — Coverage & Reliability

- **Status**: Planned — 2025-10-22
- **Dependencies**: `docs/design/testing-hardening/README.md` (to be authored), `docs/v2/testing.md`

## Why

- Ploy must sustain ≥60% overall coverage and ≥90% coverage on workflow runner packages as mandated
  by the global workflow rules.
- Integration and smoke tests need to exercise end-to-end Mods flows (scheduler → runtime → artifacts
  → GC) before v2 ships.

## What to do

- Expand unit tests across control plane, runtime, and CLI packages focusing on error paths and
  regression scenarios.
- Add integration suites that run representative Mods (OpenRewrite sample) with log streaming, SHIFT
  failure loops, and artifact publishing.
- Wire coverage gates into `make test` and update CI pipelines to track coverage deltas.

## Where to change

- `tests/integration/...` — extend harness to launch control plane + nodes locally, including SHIFT
  stubs and IPFS fixtures.
- `cmd/ploy`, `internal/controlplane`, `internal/workflow/runtime` — add targeted table-driven tests.
- CI configuration (`.github/workflows/*.yml` or equivalent) — enforce coverage thresholds and report
  trends.

## How to test

- `make test` — ensures `go test -cover ./...` meets thresholds and new suites pass.
- Add dedicated CI jobs for integration and smoke tests; verify they run on PRs and fail on
  regressions.
- Manual audit: run `go tool cover -html=coverage.out` to confirm critical packages meet 90% target.
