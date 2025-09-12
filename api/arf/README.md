# ARF (Automated Remediation Framework)

ARF provides automated code transformations with OpenRewrite, optional LLM‑assisted healing, and supporting registries. This document reflects the current, shipped API and behavior.

## What’s Implemented

- OpenRewrite execution via Nomad with SeaweedFS artifacts storage (see `openrewrite_dispatcher.go`).
- Asynchronous transforms with Consul‑backed status tracking.
- Recipe registry/catalog: list, get, search, upload, validate, download.
- Model registry for LLMs (stored in Consul).
- Security and SBOM endpoints (Phase 4 slice).
- Sandbox management endpoints (minimal).

## API Surface (v1)

- Recipes: `GET /v1/arf/recipes`, `GET /v1/arf/recipes/:id`, `GET /v1/arf/recipes/search`,
  `POST /v1/arf/recipes` (create custom), `POST /v1/arf/recipes/upload`, `POST /v1/arf/recipes/validate`, `GET /v1/arf/recipes/:id/download`.
- Models (Deprecated): `GET/POST/PUT /v1/arf/models`, `DELETE /v1/arf/models/:name`, `POST /v1/arf/models/:name/set-default`.
  - Use LLMS registry endpoints instead: `/v1/llms/models/*`.
  - Default model management: `GET /v1/llms/models/default`, `PUT /v1/llms/models/default { id }`.
- Security: `POST /v1/arf/security/scan`, `POST /v1/arf/security/remediation`, `GET /v1/arf/security/{report|report/:id|compliance}`.
- SBOM: `POST /v1/arf/sbom/{generate|analyze}`, `GET /v1/arf/sbom/{report|compliance|:id}`.
- Sandboxes: `GET/POST /v1/arf/sandboxes`, `DELETE /v1/arf/sandboxes/:id`.
 

Removed/unsupported: legacy benchmark endpoints; ARF healing coordinator and healing metrics; direct Nomad/Consul CLI instructions in this README. Healing is unified under Transflow.

## Transform Workflows

Transformation execution is unified under the Transflow API and CLI. See `docs/api/transflow.md` and `docs/transflow/README.md` for details on `/v1/transflow/*` endpoints and `ploy transflow run`.

## Operational Rules

- Do not call Nomad directly. Jobs are created by the API; platform tooling manages scheduling/logs.
- SeaweedFS stores inputs/outputs; manual cache operations are not part of ARF’s public API.
- Use the CLI for convenience; it targets the endpoints above.

## CLI Quickstart

```bash
# List recipes
ploy arf recipes list

# Execute code transformations via Transflow
ploy transflow run -f ./transflow.yaml
```

## Notes & Limitations

- Recipe update/delete routes return Not Implemented (immutable for now).
- Some Phase 3 endpoints (LLM generation, hybrid pipeline) are intentionally not exposed.
- Security/SBOM endpoints are available for basic workflows; details may evolve.
