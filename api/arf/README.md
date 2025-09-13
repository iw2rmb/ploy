# ARF (Automated Remediation Framework)

ARF provides recipe catalog/registry, Security, and minimal sandbox helpers. Transformation execution, planning, and healing are unified under Mods + LangGraph. SBOM is a separate package.

## What’s Implemented

- OpenRewrite execution is handled by Mods orw-apply. ARF does not dispatch transformations.
- Asynchronous transforms with Consul‑backed status tracking.
- Recipe registry/catalog: list, get, search, upload, validate, download.
- Model registry for LLMs (stored in Consul).
- Security endpoints (Phase 4 slice). SBOM endpoints live under `/v1/sbom/*`.
- Sandbox management endpoints (minimal).

## API Surface (v1)

- Recipes: `GET /v1/arf/recipes`, `GET /v1/arf/recipes/:id`, `GET /v1/arf/recipes/search`,
  `POST /v1/arf/recipes` (create custom), `POST /v1/arf/recipes/upload`, `POST /v1/arf/recipes/validate`, `GET /v1/arf/recipes/:id/download`.
  
  Note: LLM model registry endpoints have been removed from ARF. Use the LLMS registry under `/v1/llms/models/*` (including default model management via `/v1/llms/models/default`).
- Security: `POST /v1/arf/security/scan`, `POST /v1/arf/security/remediation`, `GET /v1/arf/security/{report|report/:id|compliance}`.
- SBOM moved to separate package:
  - `POST /v1/sbom/generate`, `POST /v1/sbom/analyze`
  - `GET /v1/sbom/{report|:id|compliance}`
- Sandboxes: `GET/POST /v1/arf/sandboxes`, `DELETE /v1/arf/sandboxes/:id`.
 

Removed/unsupported: hybrid pipeline and strategy selection, LLM dispatcher, local OpenRewrite engine, legacy benchmark endpoints, ARF healing coordinator and learning/metrics. Healing and planning are unified under Mods + LangGraph.

## Transform Workflows

Transformation execution is unified under the Mods API and CLI. See `docs/api/mods.md` for `/v1/mods/*` endpoints and use `ploy mod run`.

## Operational Rules

- Do not call Nomad directly. Jobs are created by the API; platform tooling manages scheduling/logs.
- SeaweedFS stores inputs/outputs; manual cache operations are not part of ARF’s public API.
- Use the CLI for convenience; it targets the endpoints above.

## CLI Quickstart

```bash
# List recipes
ploy arf recipes list

# Execute code transformations via Mods
ploy mod run -f ./mods.yaml
```

## Notes & Limitations

- Recipe update/delete routes return Not Implemented (immutable for now).
- Some Phase 3 endpoints (LLM generation, hybrid pipeline) are intentionally not exposed.
- Security/SBOM endpoints are available for basic workflows; details may evolve.
