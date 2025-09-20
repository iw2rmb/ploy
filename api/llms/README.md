# LLM Model Registry API

REST handlers powering `/v1/llms/models` for managing the controller’s large-language-model registry. The package exposes CRUD endpoints, filtering, default-model selection, and (placeholder) stats reporting on top of the shared storage interface.

## Responsibilities
- Parse/validate incoming model definitions using `internal/validation.LLMModelValidator`.
- Persist models under `llms/models/` in the storage backend (`internal/storage.Storage`).
- Provide list/get/create/update/delete HTTP endpoints plus a default-model shortcut and stubbed stats endpoint.
- Handle filtering (`provider`, `capability`) and pagination (`limit`, `offset`) for listing operations.

## Package Layout
- `handler.go` – Route registration and top-level Fiber handler wiring.
- `list.go` – Implements `ListModels` with storage iteration, filtering, and offset/limit handling.
- `model_crud.go` – CRUD endpoints (`GetModel`, `CreateModel`, `UpdateModel`, `DeleteModel`).
- `default.go` – Default-model helpers (`GET/PUT /default`) with fallback selection rules.
- `stats.go` – Placeholder stats endpoint returning mock metrics (intended for future telemetry integration).
- `handler_test.go` – Unit tests using storage mocks to exercise happy-path and error conditions.

## Endpoints
| Method | Path | Notes |
|--------|------|-------|
| GET | `/v1/llms/models` | Optional query params: `provider`, `capability`, `limit`, `offset`. |
| GET | `/v1/llms/models/:id` | Returns model JSON by ID. |
| POST | `/v1/llms/models` | Creates a new model (400 on validation failure, 409 on duplicate ID). |
| PUT | `/v1/llms/models/:id` | Replaces an existing model (ID in body must match URL). |
| DELETE | `/v1/llms/models/:id` | Removes a model from storage. |
| GET | `/v1/llms/models/:id/stats` | Stubbed stats payload (always zeroed until metrics pipeline lands). |
| GET | `/v1/llms/models/default` | Fetches configured default, falls back to first/code-capable model. |
| PUT | `/v1/llms/models/default` | Sets the default model by ID. |

All responses are JSON; errors follow `{ "error": "message" }` conventions with appropriate HTTP status codes (400/404/409/500).

## Dependencies
- **Fiber** (`github.com/gofiber/fiber/v2`) for routing and JSON responses.
- **Storage** (`internal/storage`) for persistence (SeaweedFS in production).
- **Validation** (`internal/validation.LLMModelValidator`) for provider-specific rules and capability enforcement.
- **Model types** (`internal/llms/models`) for request/response payloads.

## Notes & Future Work
- `GetModelStats` currently returns a mock response; wiring actual usage metrics will require telemetry ingestion.
- Default model metadata is stored at `llms/models/__default` as `{"id": "..."}`; corruption or missing IDs trigger the fallback selection logic.
- Listing currently loads each model individually; if registry scale grows significantly we should move to manifest caching in storage.

## Related Docs
- `internal/storage/README.md` – Storage client used by these handlers.
- `internal/validation/README.md` – Validation rules applied to model definitions.
- `cmd/ployman/README.md` – CLI commands that call these endpoints.
