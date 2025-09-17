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

## SBOM Endpoints

The SBOM module provides endpoints under `/v1/sbom`.

### POST /v1/sbom/generate

Generate a Software Bill of Materials (SBOM) using Syft for a file artifact or a container image. The API delegates to the Syft‑based generator in `api/supply/sbom.go`.

- Request (JSON)
  - `artifact` (string, required): Path to a file artifact (e.g., `/path/to/app.bin`) or a container image reference (e.g., `repo/app:1.2.3`).
  - `format` (string, optional): Output format, defaults to `spdx-json`. Accepts Syft‑supported formats.
  - `lane` (string, optional): Deployment lane identifier for metadata.
  - `app_name` (string, optional): Application name for metadata.
  - `sha` (string, optional): Build SHA for metadata.

- Response (JSON)
  - `status`: Always `"completed"` on success.
  - `generated_at`: RFC3339 timestamp.
  - `format`: Resolved output format (e.g., `spdx-json`).
  - `location`: Absolute path to the generated SBOM file.

- Behavior
  - If `artifact` contains a colon (`:`), it is treated as a container image reference and the SBOM is written to `/tmp/<sanitized-image>.sbom.json`.
  - Otherwise, the SBOM is generated next to the file with suffix `.sbom.json`.
  - Backward compatibility: If `artifact` is omitted, the endpoint returns a minimal successful envelope for legacy tests, but no real generation occurs.

- Examples

  File artifact
  - Request
    - `POST /v1/sbom/generate`
    - Body: `{ "artifact": "/opt/builds/app.bin", "format": "spdx-json", "lane": "E", "app_name": "app", "sha": "abc123" }`
  - Response
    - `{ "status": "completed", "generated_at": "2025-09-13T18:30:00Z", "format": "spdx-json", "location": "/opt/builds/app.bin.sbom.json" }`

  Container image
  - Request
    - `POST /v1/sbom/generate`
    - Body: `{ "artifact": "registry.local/app:1.2.3", "format": "spdx-json" }`
  - Response
    - `{ "status": "completed", "generated_at": "2025-09-13T18:30:00Z", "format": "spdx-json", "location": "/tmp/registry.local-app-1.2.3.sbom.json" }`

### POST /v1/sbom/analyze

Analyze an existing SBOM and return a basic risk summary. This endpoint currently performs lightweight parsing and metric computation; vulnerability correlation is a stub in this module and is typically covered by Grype + enrichment pipelines.

- Request (JSON)
  - `sbom_path` (string, required): Path to an existing SBOM file.

- Response (JSON)
  - Includes `summary`, `vulnerabilities` (mock structure), and `generated_at` used by tests.

