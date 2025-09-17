# Ploy API Server

The Ploy API server exposes REST endpoints for deployment, builds, recipes, mods orchestration, and platform management. HTTP routes are wired in `api/server` (see `api/server/server.go` and `api/server/routes.go`).

## Directory Overview (current)

```
api/
├── main.go                # API server entry point
├── server/                # HTTP server: routes, handlers, initializers, storage resolver
├── config/                # Configuration loading and validation
├── health/                # Health/readiness/liveness endpoints and types
├── metrics/               # Prometheus metrics integration
├── builders/              # Lane builders (unikraft, oci, vm, wasm, jail, java_osv, jib)
├── nomad/                 # Nomad client, render, submit (+ enhanced submit)
├── certificates/          # Certificate lifecycle (manager, wildcard)
├── dns/                   # DNS providers (Cloudflare, Namecheap) + handler
├── acme/                  # ACME client, handlers, renewal, storage
├── domains/               # Domain configuration handlers
├── routing/               # Traefik config helpers
├── templates/             # Template endpoints
├── supply/                # SBOM generation, signing, verification
├── opa/                   # OPA verification helpers
├── mods/                  # Mods API (run, status, logs, artifacts, debug)
├── llms/                  # Model registry CRUD, list, stats
├── git/                   # Git service and push event handling
├── analysis/              # Static analysis engine + analyzers (java, python)
├── sbom/                  # SBOM HTTP endpoints and analyzer helpers
├── nvd/                   # NVD database/types/lookup/converter
├── platform/              # Platform handler endpoints
├── recipes/               # Recipes API, registry adapter, models/
├── consul_envstore/       # Consul-backed environment store
├── coordination/          # Leader election and TTL cleanup
├── selfupdate/            # Self-update executor and endpoints
├── runtime/               # Runtime integrations (WASM)
├── wasm/                  # WASM component wiring
└── version/               # Version endpoint
```

### Nomad Wrapper Policy (VPS)

On VPS environments, all Nomad interactions are routed through the job manager wrapper at `/opt/hashicorp/bin/nomad-job-manager.sh`.

- Submission and validation prefer the wrapper and fall back to SDK/CLI only when absent (non‑VPS/local).
- Benefits: unified retries/backoff for 429/5xx, HCL→JSON conversion/validation, consistent logging, service cleanup.
- Do not call the raw `nomad` CLI directly in API code on the VPS; use the wrapper or orchestration facade.
