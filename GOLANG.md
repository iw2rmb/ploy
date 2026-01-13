# Golang Engineering Guide

This document codifies Go engineering rules for this repository. It complements `docs/GOLANG.md` (tooling quickstart) and aligns with our RED → GREEN → REFACTOR cadence from `AGENTS.md`.

- Primary references
  - docs: `docs/GOLANG.md` (tooling), `AGENTS.md` (TDD + coverage), `README.md` (architecture)
  - externals (authoritative): Go Code Review Comments, Effective Go, fuzzing and security/govulncheck guidance, and the Uber Go Style Guide. See References.

## Versions & Layout
- Target Go: match repository requirement in `README.md` (Go 1.25+).
- Modules: single-module unless otherwise justified. Keep public APIs stable; prefer `internal/...` for non‑exported packages.
- Package boundaries: keep CLI thin; orchestration logic lives under `internal/...` packages per `AGENTS.md`.

## Docker Engine Requirements

Worker nodes (`ployd-node`) require Docker Engine v29.0+ for container execution.
The SDK migration from `github.com/docker/docker` to moby Engine v29 modules is
complete.

### Supported Daemon Versions
- **Minimum**: Docker Engine **v29.0** (API v1.44+)
- **Recommended**: Latest v29.x patch release
- **API floor**: v1.44 (enforced by Engine v29; clients negotiating <v1.44 are rejected)
- **Unsupported**: Engine v28.x and earlier—API negotiation may succeed but
  behaviour is untested and may exhibit drift.

### Go SDK Modules (Contributors)

The Docker Go SDK migrated from `github.com/docker/docker` (now **removed** from
`go.mod`) to the moby Engine v29 modules:

| Module path                              | Purpose                                    |
|------------------------------------------|--------------------------------------------|
| `github.com/moby/moby/client`            | Client construction and Docker API calls   |
| `github.com/moby/moby/api/types/container` | ContainerConfig, HostConfig, WaitResponse |
| `github.com/moby/moby/api/types/mount`   | Bind and volume mount specifications       |
| `github.com/moby/moby/api/pkg/stdcopy`   | Log stream demuxing (stdout/stderr frames) |

Do **not** import `github.com/docker/docker`; the module has been removed.

### Client Construction Pattern

All Docker client construction uses `client.FromEnv` (reads environment) and
`client.WithAPIVersionNegotiation` (auto-negotiates API version with daemon):

```go
// Standard pattern: read DOCKER_HOST, DOCKER_TLS_VERIFY, DOCKER_CERT_PATH from
// environment and auto-negotiate API version with the daemon.
cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
if err != nil {
    return fmt.Errorf("configure docker client: %w", err)
}
```

Implementation: `internal/workflow/runtime/step/container_docker.go:59-66`.

### Environment Variables (Optional)

These variables are read by `client.FromEnv` and rarely need explicit setting:

| Variable             | Default                          | Description                              |
|----------------------|----------------------------------|------------------------------------------|
| `DOCKER_HOST`        | `unix:///var/run/docker.sock`    | Docker daemon address                    |
| `DOCKER_TLS_VERIFY`  | (unset)                          | Set to `"1"` to enable TLS verification  |
| `DOCKER_CERT_PATH`   | (unset)                          | Path to TLS certificates when TLS enabled |
| `DOCKER_API_VERSION` | (auto-negotiated)                | Override API version; normally unnecessary |

### Verification

```bash
# Check Docker Engine version on a node (must report v29.0+).
docker version --format '{{.Server.Version}}'

# Run the Docker health check test (uses moby client interface).
go test ./internal/worker/lifecycle -run 'DockerChecker' -v
```

### Cross-References

- Local Docker cluster: `docs/how-to/deploy-locally.md`
- Environment variables: `docs/envs/README.md`
- Container runtime: `internal/workflow/runtime/step/container_docker.go`
- Health checker: `internal/worker/lifecycle/health.go`
- Gate executor: `internal/workflow/runtime/step/gate_docker.go`

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

## Git Operations & Workspace Hydration

### Base Hydration Strategy

Ploy uses **shallow git clones** (via `internal/worker/hydration.GitFetcher`) to create the logical "base snapshot" for each run on each node. This strategy minimizes network transfer and disk usage while providing consistent starting points for applying per-step diffs during multi-step Mods runs.

**Key behaviors:**
- **Shallow clones**: Uses `git clone --depth 1` to fetch only the latest commit, avoiding full history transfer.
- **Base reference**: When `base_ref` is provided, clones that specific branch/tag; otherwise uses the repository's default branch.
- **Commit pinning**: When `commit_sha` is provided, fetches and checks out that exact commit after cloning, ensuring deterministic base snapshots across nodes even if `base_ref` has moved forward.
- **Multi-node consistency**: Each node clones the same `base_ref`/`commit_sha` independently, guaranteeing identical base states before applying ordered diffs.

**Implementation location:** `internal/worker/hydration/git_fetcher.go`

**Target reference behavior:** The `target_ref` field is intentionally **not** checked out during hydration. The workspace remains on `base_ref` so that subsequent diff application produces the correct final state for each step.

**Example flow:**
1. Node receives run with `base_ref=main` and `commit_sha=abc123`
2. GitFetcher performs: `git clone --depth 1 --branch main --single-branch <repo>`
3. GitFetcher then: `git fetch origin abc123 --depth 1 && git checkout FETCH_HEAD`
4. Result: Workspace at exact commit `abc123`, ready for diff application

**Testing:** See `internal/worker/hydration/git_fetcher_test.go` for validation of shallow clone depth, commit pinning, and deterministic base snapshot behavior.

## Database (PostgreSQL) — pgx + sqlc
- Use `pgx/v5` with `pgxpool` and typed `sqlc` queries as the default data layer (schema in `internal/store/schema.sql`).
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
