# Security Engine
The Security Engine provides vulnerability scanning, recipe catalog helpers, and minimal sandbox tooling. Transformation execution, planning, and healing remain in Mods + LangGraph. SBOM functionality lives under its dedicated package.
## What’s Implemented

- OpenRewrite execution is handled by Mods orw-apply. Security Engine does not dispatch transformations.
- Asynchronous transforms with Consul‑backed status tracking.
- Recipe registry/catalog: list, get, search, upload, validate, download.
- Model registry for LLMs (stored in Consul).
- Security endpoints (Phase 4 slice). SBOM endpoints live under `/v1/sbom/*`.
- Sandbox management endpoints (minimal).

- Recipes: `GET /v1/recipes`, `GET /v1/recipes/:id`, `GET /v1/recipes/search`,
  `POST /v1/recipes` (create custom), `POST /v1/recipes/upload`, `POST /v1/recipes/validate`, `GET /v1/recipes/:id/download`.
  
  Note: LLM model registry endpoints live under `/v1/llms/models/*` (including default model management via `/v1/llms/models/default`).
- Security: `POST /v1/security/scan`, `POST /v1/security/remediation`, `GET /v1/security/{report|report/:id|compliance}`.
- SBOM: see `/v1/sbom/*` endpoints for generation and analysis.
- Sandboxes: `GET/POST /v1/security/sandboxes`, `DELETE /v1/security/sandboxes/:id` (legacy; Mods covers primary workflows).
- SBOM moved to separate package:
  - `POST /v1/sbom/generate`, `POST /v1/sbom/analyze`
  - `GET /v1/sbom/{report|:id|compliance}`
- Sandboxes: `GET/POST /v1/arf/sandboxes`, `DELETE /v1/arf/sandboxes/:id`.
 

Removed/unsupported: hybrid pipeline and strategy selection, LLM dispatcher, local OpenRewrite engine, legacy benchmark endpoints, Security Engine healing coordinator and learning/metrics. Healing and planning are unified under Mods + LangGraph.

## Transform Workflows

Transformation execution is unified under the Mods API and CLI. See `docs/api/mods.md` for `/v1/mods/*` endpoints and use `ploy mod run`.

## Operational Rules

- Do not call Nomad directly. Jobs are created by the API; platform tooling manages scheduling/logs.
- SeaweedFS stores inputs/outputs; manual cache operations are not part of Security Engine’s public API.
- Use the CLI for convenience; it targets the endpoints above.

## CLI Quickstart

```bash
# List recipes
ploy recipes list

# Execute code transformations via Mods
ploy mod run -f ./mods.yaml
```

## Notes & Limitations

- Recipe update/delete routes return Not Implemented (immutable for now).
- Some Phase 3 endpoints (LLM generation, hybrid pipeline) are intentionally not exposed.
- Security/SBOM endpoints are available for basic workflows; details may evolve.
