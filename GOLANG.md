# Golang Engineering Guide

This document codifies Go engineering rules for this repository. It complements `docs/GOLANG.md` (tooling quickstart) and aligns with our RED → GREEN → REFACTOR cadence from `AGENTS.md`.

- Primary references
  - docs: `docs/GOLANG.md` (tooling), `AGENTS.md` (TDD + coverage), `README.md` (architecture), `CHECKPOINT.md` (status)
  - externals (authoritative): Go Code Review Comments, Effective Go, fuzzing and security/govulncheck guidance, and the Uber Go Style Guide. See References.

## Versions & Layout
- Target Go: match repository requirement in `README.md` (Go 1.25+).
- Modules: single-module unless otherwise justified. Keep public APIs stable; prefer `internal/...` for non‑exported packages.
- Package boundaries: keep CLI thin; orchestration logic lives under `internal/...` packages per `AGENTS.md`.

## Formatting & Linting
- Formatting is enforced automatically by a pre‑commit hook in `.githooks/pre-commit` (run `git config core.hooksPath .githooks` once; `IMPLEMENT.sh` sets this automatically when present).
- The hook runs `goimports -w` (if available) and `gofmt -s -w` on all tracked `*.go` files and re‑stages the changes. No manual formatting needed.
- Reviewer/CI owns repo‑wide hygiene checks: `go vet ./...` and `staticcheck ./...`. Implementers do not need to run these by default.

## Error Handling
- Don’t use panic for normal errors; prefer `error` returns. Error strings are lowercase without trailing punctuation; wrap with `%w` and use `errors.Is/As` for inspection.
- Avoid double-reporting (log and return). Return errors to the caller or log at the edge.

## Documentation & Naming
- Exported identifiers require doc comments that start with the identifier name and end with a period. Use mixedCaps; avoid ALL_CAPS.

## Context & Concurrency
- Accept `context.Context` as the first parameter when work is request‑scoped; propagate deadlines/cancellation. Do not store `Context` in structs.
- Make goroutine lifetimes explicit; avoid leaks. Prefer cancellation via context and `errgroup` patterns (or channel close rules). Be careful with `t.Parallel()` in table tests—capture loop vars correctly.

## Testing, Fuzzing, Coverage
- Unit tests are table‑driven with subtests (`t.Run`). Keep clear failure messages; follow RED → GREEN → REFACTOR.
- Fuzzing: add fuzz targets for critical parsing/decoding paths (`FuzzXxx(*testing.F)`); keep targets deterministic and fast.
- Role split:
  - Implementer: run fast tests for changed packages (e.g., `go test ./internal/pkg1 ./cmd/tool`), omit repo‑wide `-race`.
  - Reviewer/CI: run `go test -race ./...` across the repo.
- Coverage targets: ≥60% overall and ≥90% on critical workflow runner packages (per `AGENTS.md`).

## Security & Supply Chain
- Scan dependencies with `govulncheck ./...` as part of release and when changing dependencies. Prefer fixes that upgrade vulnerable modules.
- Prefer standard library first; add third‑party deps only with rationale. Keep `go.mod` minimal; run `go mod tidy -v`.

## Logging & Observability
- Prefer structured logging (e.g., `log/slog`) with stable keys; never log secrets.
- Expose metrics (Prometheus) and enable pprof endpoints for long‑running services.

## HTTP/Networking
- For servers, set timeouts: `ReadHeaderTimeout`, `ReadTimeout`, `WriteTimeout`, `IdleTimeout`. Always close response bodies; use `http.Client{Timeout: ...}` and per‑request contexts.

## Database (PostgreSQL) — pgx + sqlc
- Use `pgx/v5` with `pgxpool` and typed `sqlc` queries as the default data layer (schema in `SCHEMA.sql`).
- Transactions: check errors on `Commit`; rollback on error with `defer tx.Rollback(ctx)`.
- Contexts: every query accepts a `ctx` with deadlines where appropriate.
- Migrations in `internal/store/migrations/`; queries in `internal/store/queries/`; generate code via `sqlc` and commit generated artifacts only if our CI does not run generators.

## Performance Tips
- Avoid unnecessary allocations; preallocate slices where size is known. Avoid `defer` in hot loops. Reuse buffers where safe. Measure with benchmarks and pprof before optimizing.

## CLI & Build Rules
- Use `make build` to compile, `make test` for `go test -cover ./...` if provided. Keep the CLI thin; orchestration lives in `internal/...`.

## Code Review Expectations
- Reviews enforce this guide and Go’s Code Review Comments. Prefer small, focused PRs with tests.
- Reviewer runs repo‑wide hygiene (format is verified by hook), `go vet`, `staticcheck`, and `go test -race ./...`, and summarizes results succinctly.

## Tooling Quick Commands (baseline)
- Implementer (local fast loop): `go test ./changed/pkg1 ./changed/pkg2`
- Reviewer/CI (full sweep): `goimports -w . && gofmt -s -w . && go vet ./... && staticcheck ./... && go test -race ./...`
- Fuzz: `go test -fuzz=Fuzz -run=^$ ./...`
- Vulns: `govulncheck ./...`
- Mods: `go mod tidy -v`

## References
- Go Code Review Comments (style, errors, naming, concurrency). [go.dev](https://go.dev/wiki/CodeReviewComments?utm_source=openai)
- Effective Go (formatting, comments, naming, errors, functions). [tip.golang.org](https://tip.golang.org/doc/effective_go?utm_source=openai)
- Fuzzing tutorial (FuzzXxx, workflows). [go.dev](https://go.dev/doc/tutorial/fuzz?utm_source=openai)
- govulncheck (blog + tutorial: usage and CI). [go.dev](https://go.dev/blog/govulncheck?utm_source=openai)
- slog (structured logging) docs/blog. [pkg.go.dev](https://pkg.go.dev/log/slog?utm_source=openaiq)
