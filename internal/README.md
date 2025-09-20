# Internal Shared Libraries

Modules shared by API and CLI. This folder contains the core building blocks for builds, orchestration, storage, policies, Git, and the CLI surface.

## Directory Overview (current)

```
internal/
├── arf/            # ARF migration target: engine/models/recipes (consolidating from api/arf)
├── bluegreen/      # Blue‑green deployment helpers
├── build/          # Build service (see internal/build/README.md)
├── builders/       # Builder facades and debug utilities
├── cleanup/        # TTL cleanup service (config, handler, TTL workers)
├── cli/            # CLI modules (see internal/cli/README.md)
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
├── mods/           # Mods subsystem — see internal/mods/README.md
├── monitoring/     # Health, metrics, tracing instrumentation
├── orchestration/  # Nomad orchestration (render/submit/monitor, HCL templates, retry transport)
├── policy/         # Policy enforcement (enforcer + config)
├── preview/        # Preview router (SHA‑based preview host mapping)
├── routing/        # Routing metadata (tags, KV helpers)
├── security/       # Security scanners and helpers
├── storage/        # Object storage abstraction (see internal/storage/README.md)
├── supply/         # Supply‑chain integration facade
├── testing/        # Test builders, fixtures, mocks, helpers (see internal/testing/README.md)
├── utils/          # Shared helpers (files/strings/http), image size utilities
├── validation/     # Input validation (env vars, resources, app name)
└── version/        # Version constants
```

## Highlights

- Build service: see [internal/build/README.md](build/README.md) for triggers, builder orchestration, and supply-chain helpers.
- Orchestration: renders Nomad HCL specs, submits jobs, and monitors health with retries and timeouts. Templates embedded via `templates_embed.go`.
- Storage: robust SeaweedFS client with middleware for retries, metrics, and caching; integrity verification helpers.
- Mods: end‑to‑end “healing” orchestration including planning, execution, diffing, KB learning, MR integration, and job submission/logging.
- CLI: cohesive `ploy`/`ployman` command surface—see [internal/cli/README.md](cli/README.md).
- Testing: shared mocks, builders, and integration harness—see [internal/testing/README.md](testing/README.md).

## Notes

- When adding new internal modules, prefer small, focused packages and wire them through `internal/config` and `internal/orchestration` seams when applicable.
