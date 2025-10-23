# Node HTTP Server Migration to Fiber

## Motivation

Worker and bootstrap nodes currently expose local HTTP endpoints using the Go standard library
(`net/http`). The existing `internal/node/httpapi` package manually wires handlers into a
`http.ServeMux`, implements Server-Sent Event streaming with custom helpers, and duplicates middleware
responsibilities (logging, request IDs, panic handling) across components. As the node footprint
expands—adding health probes, artifact endpoints, control-plane callbacks—maintaining bespoke routing
and middleware becomes brittle.

Adopting Fiber (`github.com/gofiber/fiber/v2`) provides:

- A cohesive router with expressive path syntax, parameter parsing, and grouped middleware.
- First-class support for JSON, streaming responses, and websocket upgrades.
- Unified middleware stack (logging, metrics, CORS, authentication) without custom boilerplate.
- Performance characteristics similar to or better than our current net/http setup, with lower
  allocation pressure thanks to Fiber's fasthttp core.

## Scope

### In Scope

- Replace node-local HTTP server implementations (log streaming, job control surfaces) with Fiber.
- Share a common `fiber.App` bootstrapper across worker and bootstrap node binaries.
- Introduce middleware for request logging, panic recovery, auth headers, and metrics emission.
- Update tests to exercise Fiber handlers (using `fiber.App.Test`).

### Out of Scope

- Control-plane API migration (still relies on `net/http` until separately refactored).
- CLI-side networking changes.
- Protocol redesign; the same REST endpoints and SSE contracts remain.

## Proposed Design

1. **App Factory**
   - Create `internal/node/httpapi/server.go` returning a configured `*fiber.App`, replacing the
     current `New(...) http.Handler`.
   - Mount routes under `/node/v2` with group-specific middleware (auth, rate limits).

2. **Middleware Stack**
   - Logging middleware writing structured entries via our existing logging package.
   - Recovery middleware to trap panics and convert to HTTP 500 responses.
   - Metrics middleware emitting request duration/status counters.
   - Optional authentication middleware validating local tokens or mTLS context.

3. **Handlers**
   - Port existing job log streaming to Fiber's response API (e.g., `c.Context().SetBodyStreamWriter`).
   - Ensure SSE headers (`Content-Type: text/event-stream`) and Last-Event-ID parsing continue to work.
   - Wrap existing business logic (`logstream.Hub`) without rewriting underlying functionality.

4. **Bootstrap Integration**
   - Worker and bootstrap binaries instantiate the Fiber app and call `app.Listener(...)` (with TLS if
     configured).
   - Provide graceful shutdown hooks leveraging Fiber's `Shutdown` support and context cancellation.

5. **Configuration**
   - Add options struct for timeouts, read/write limits, and TLS paths.
   - Expose CLI flags or environment variables to override defaults where needed.

6. **Testing**
   - Replace `net/http/httptest` usage with Fiber's testing helpers.
   - Add integration test ensuring SSE streaming remains compliant (Last-Event-ID, reconnects).
   - Benchmark before/after to validate memory and latency characteristics.

## Risks & Mitigations

- **Dependency Footprint**: Fiber pulls in fasthttp, increasing binary size. Mitigate by auditing the
  final binary and stripping unused features (disable websocket or template modules if unnecessary).
- **SSE Compatibility**: Fiber’s streaming APIs differ from net/http; we must confirm our downstream
  clients handle the event framing identically. Add regression tests with the CLI log streaming code.
- **Operational Differences**: Observability tooling (e.g., pprof via `net/http/pprof`) must be wired
  separately. Introduce optional debug listeners or mirror the /debug endpoints using net/http on a
  different port.

## Rollout Plan

1. Implement the Fiber app alongside the existing handler and guard it behind a build flag or config
   toggle.
2. Run workers in the lab using the Fiber path, verify log streaming and health endpoints.
3. Deprecate the net/http path once production bake completes.
4. Remove legacy code and flags, leaving Fiber as the sole implementation.

## Open Questions

- Should we expose a separate management listener (e.g., gRPC) alongside Fiber for future APIs?
- Do we need to support HTTP/2 over TLS (fasthttp is HTTP/1.1 only); if so, consider a proxy.
- How do we standardize middleware between node and control-plane servers to avoid divergence?

