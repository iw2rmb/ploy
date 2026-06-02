internal/server - server packages

This folder groups the server-side packages that make up the control plane.
The top-level directory is a pure container; there are no `.go` files at
this level; each concern lives in its own subpackage.

- auth: mTLS-derived role authorization middleware and helpers used by the HTTP server and handlers. Roles: control-plane, worker, cli-admin.
- httpserver: HTTP server wrapper with timeouts, panic recovery, and route mounting.
- handlers: Control-plane HTTP handlers (runs, nodes, PKI, repos/migs) and a RegisterRoutes helper to mount them.
- events: In-memory SSE fanout service that publishes run events/logs via internal/stream.
- metrics: Prometheus metrics HTTP server.
- scheduler: Background tasks and TTL worker orchestration.
- recovery: Stale-job recovery and run-status reconciliation.
- blobpersist: Blob persistence service used by handlers for spec bundles, logs, and diffs.
- pki: Certificate authority and rotation logic.
- config: Server-only configuration types, defaults, and loader.
  Node-agent configuration lives under `internal/nodeagent/config.go`.

Related packages
- internal/stream: Shared SSE hub and HTTP helpers for event streaming.
- internal/worker: Node-side execution primitives used by the node agent.
- internal/nodeagent: Node daemon that composes worker primitives and pushes data to the server.
