internal/server — server packages

This folder groups the server-side packages that make up the control plane.

- auth: mTLS-derived role authorization middleware and helpers used by the HTTP server and handlers. Roles: control-plane, worker, cli-admin.
- http: HTTP server wrapper with TLS/mTLS listeners, timeouts, and route mounting.
- handlers: Control-plane HTTP handlers (runs, nodes, PKI, repos/migs) and a RegisterRoutes helper to mount them.
- events: In-memory SSE fanout service that publishes run events/logs via internal/stream.
- metrics: Prometheus metrics HTTP server.
- scheduler: Background tasks and TTL worker orchestration.
- status: Health/status providers for diagnostics.
- config: Server-only configuration types, defaults, loader, and watcher.
  Node-agent configuration lives under `internal/nodeagent/config.go`.

Related packages
- internal/stream: Shared SSE hub and HTTP helpers for event streaming.
- internal/worker: Node-side execution primitives used by the node agent.
- internal/nodeagent: Node daemon that composes worker primitives and pushes data to the server.
