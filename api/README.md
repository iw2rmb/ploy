# Ploy API

Fiber-based HTTP server exposing the control-plane endpoints for builds, deployments, mods, recipes, domains, certificates, SBOMs, LLM registry, and supporting operations. Routes are registered in `api/server/routes.go`; each sub-package carries its own handlers and business logic.

## Directory Overview
```
api/
├── main.go          # Server entry point (CLI wiring, graceful shutdown)
├── server/          # HTTP server, dependency graph, route registration
├── config/          # Configuration loading & validation (env/files)
├── health/          # Health/readiness/liveness handlers
├── metrics/         # Prometheus instrumentation
├── build/           # Legacy build adapters (mostly lane D shims)
├── nomad/           # Nomad client and job submission helpers
├── mods/            # Mods API routes (run/status/logs/artifacts)
├── llms/            # LLM model registry API
├── recipes/         # Recipe catalog API
├── sbom/            # SBOM endpoints & helpers
├── dns/, domains/   # DNS + domain management handlers
├── certificates/    # Certificate issuance & management
├── supply/          # Supply chain (signing, provenance)
├── git/             # Git helper endpoints (push hooks, status)
├── analysis/        # Static analysis endpoints
├── security/        # Security automation
├── consul_envstore/ # Consul-backed env var store
├── coordination/    # Cleanup, leader election endpoints
├── selfupdate/      # Self-update routes
├── templates/       # Template CRUD endpoints
├── routing/         # Traefik config helpers
├── version/         # Version endpoint
└── nvd/, opa/, acme/, platform/, etc.  # Supporting modules
```

Module READMEs (if present) dive deeper into package-specific behaviour (e.g. `api/mods/README.md`, `api/llms/README.md`).

## Core Concepts
- **Dependency Injection**: `api/server/server.go` builds a dependency graph (storage clients, orchestrators, handlers) and hands them to route setup; makes components testable and replaceable.
- **Fiber Middleware**: Common middleware (logging, recovery, compression) configured in `server/`. JSON responses standardise `{ "error": "..." }` on failure.
- **Nomad Wrapper**: VPS deployments run all Nomad operations through `/opt/hashicorp/bin/nomad-job-manager.sh`; the orchestration facade in `internal/orchestration` is injected accordingly.
- **Storage Integration**: Handlers use `internal/storage.Storage` for consistent SeaweedFS access: artifacts, SBOMs, LLM models, mods outputs.
- **Metrics & Health**: `/metrics`, `/health`, `/ready`, `/live`, and specific health endpoints (storage, certificates, deployment) expose service state.

## Local Development
Run the API server locally:
```bash
go run ./api
```
Environment variables: `PLOY_CONTROLLER`, storage endpoints, Nomad/Consul addresses, TLS cert locations. See `api/config/` for defaults and `internal/build/README.md` / `internal/orchestration/README.md` for background.

## Testing
- Package-level tests live next to handlers (e.g. `api/mods/*.go` + `_test.go`).
- End-to-end tests sit under `tests/e2e/` and hit these routes over HTTP.
- Use `GO_ENV=test` and local SeaweedFS/Nomad stubs when running integration tests.

## Related Documentation
- `internal/build/README.md` – Build pipeline invoked by `/apps/:app/builds`.
- `internal/orchestration/README.md` – Job submission/log streaming helpers used across API handlers.
- `internal/storage/README.md` – SeaweedFS abstraction backing API persistence.
- `api/mods/README.md` – Mods-specific endpoints.
- `api/llms/README.md` – LLM model registry API.
- `docs/REPO.md` – High-level repository map.
