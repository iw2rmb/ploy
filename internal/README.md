# Internal Shared Libraries

Modules shared by API and CLI. This folder contains the core building blocks for builds, orchestration, storage, policies, Git, and the CLI surface.

## Directory Overview (current)

```
internal/
├── arf/            # ARF migration target: engine/models/recipes (consolidating from api/arf)
├── bluegreen/      # Blue‑green deployment helpers
├── build/          # Build service (lanes A–G, artifacts, logs, SBOM, signing, sandbox, Dockerfile templates)
├── builders/       # Builder facades and debug utilities
├── cleanup/        # TTL cleanup service (config, handler, TTL workers)
├── cli/            # CLI modules (recipes, platform, deploy, ARF, SBOM, common utils)
├── config/         # Config loader/validator, registry/factory, Consul source, cache
├── debug/          # Debug HTTP handlers
├── detect/         # Project/language detection (java, node, scala, dotnet, rust, project facts)
├── distribution/   # Binary distribution pipeline, metadata, rollback utilities
├── domain/         # Domain subsystem handlers
├── env/            # Environment handlers (HTTP)
├── envstore/       # Environment KV store abstraction + implementations
├── errors/         # Standard error types and helpers
├── git/            # Git ops (repo, provider integrations, validation, security, stats)
├── lane/           # Lane detection rules and helpers
├── lifecycle/      # App lifecycle operations (create/destroy/rollback)
├── mods/           # Mods orchestration (planner/reducer/LLM exec/ORW, KB, MR, events, images, gates)
├── monitoring/     # Health, metrics, tracing instrumentation
├── orchestration/  # Nomad orchestration (render/submit/monitor, HCL templates, retry transport)
├── policy/         # Policy enforcement (enforcer + config)
├── preview/        # Preview router (SHA‑based preview host mapping)
├── routing/        # Routing metadata (tags, KV helpers)
├── security/       # Security scanners and helpers
├── storage/        # Object storage abstraction (SeaweedFS provider, middleware: retry/metrics/cache, factory)
├── supply/         # Supply‑chain integration facade
├── testing/        # Test builders, fixtures, mocks, helpers, integration client
├── utils/          # Shared helpers (files/strings/http), image size utilities
├── validation/     # Input validation (env vars, resources, app name)
└── version/        # Version constants
```

## Highlights

- Build service: unified sandboxed build execution; lane A–G flows; SBOM/signing; Dockerfile generation under `internal/build/templates/`.
- Orchestration: renders Nomad HCL specs, submits jobs, and monitors health with retries and timeouts. Templates embedded via `templates_embed.go`.
- Storage: robust SeaweedFS client with middleware for retries, metrics, and caching; integrity verification helpers.
- Mods: end‑to‑end “healing” orchestration including planning, execution, diffing, KB learning, MR integration, and job submission/logging.
- CLI: cohesive command implementations for recipes, platform deploys, model registry, and ARF workflows.

## Notes

- ARF is being migrated into `internal/arf` from `api/arf` incrementally; see `internal/arf/README.md` for status.
- When adding new internal modules, prefer small, focused packages and wire them through `internal/config` and `internal/orchestration` seams when applicable.

