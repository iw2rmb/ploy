# Mods API

HTTP handlers powering `/v1/mods` endpoints, bridging the controller to the Mods orchestration stack. These routes provide artifact download, log streaming, run submission, and status inspection for automated code modification workflows.

## Key Takeaways
- Wraps Mods runner calls with Fiber handlers, ensuring request validation, storage-backed artifact retrieval, and consistent responses.
- Exposes endpoints for triggering Mods runs, checking status, streaming logs, and retrieving produced artifacts (plans, diffs, SBOMs).
- Integrates with internal storage, orchestration, and event reporting layers; handlers are lightweight and focus on HTTP concerns.

## Feature Highlights
- **Run submission** (`POST /v1/mods/run`) – Accepts run payloads, kicks off planner/reducer pipelines via orchestration helpers.
- **Status & logs** (`GET /v1/mods/:id/status`, `/logs`, `/logs/stream`) – Provide synchronous snapshots or live streaming for Mods runs.
- **Artifacts** (`GET /v1/mods/:id/artifacts/*.zip`) – Download planner/reducer outputs, diffs, SBOMs, etc., persisted in SeaweedFS.
- **Debug helpers** (`POST /v1/mods/:id/debug`) – Augment run diagnostics, returning human-readable hints.

## Files
- `handler.go` – Routes registration (list, status, logs, artifacts, run) and dependency wiring.
- `run.go` / `run_test.go` – Parse run requests, call orchestration layer, transform responses/errors.
- `status.go` / `status_test.go` – Status polling, including storage lookups and structured output.
- `logs.go` / `logs_test.go` – Log tailing/streaming via orchestration log streamer helpers.
- `artifacts.go` / `artifacts_test.go` – Artifact listing and download proxies from SeaweedFS or storage abstraction.
- `debug.go` / `debug_test.go` – Additional debug endpoints (e.g., summarising planner fail states).
- `types.go` – Request/response DTOs shared across handlers.

## Usage Notes
- Handlers rely on the backend storage interface; ensure `internal/storage` is configured with the mods artifact prefix.
- Streaming endpoints upgrade the connection via Fiber’s `c.Context().SetBodyStreamWriter`; clients should handle chunked responses.
- `run.go` uses context deadlines and returns immediate 202 responses for async operations—callers must poll status endpoints.
- Tests use storage/mocks to validate happy paths and ensure HTTP status codes align with orchestrator behaviour.

## Related Docs
- `internal/mods/README.md` – Mods runner/orchestration details consumed by these endpoints.
- `internal/storage/README.md` – Artifact persistence used by Mods.
- `internal/orchestration/README.md` – Job submission/log streaming utilities invoked by run/log handlers.
