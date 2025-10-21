# API Surfaces

## Why
- Ploy v2 introduces dedicated APIs for the control plane, worker nodes, and beacon mode (`docs/v2/README.md` and `docs/v2/api.md`).
- Clean, versioned APIs replace Grid-era RPCs and support multi-node orchestration without backward compatibility constraints.

## Required Changes
- Define REST (or gRPC) schemas for control-plane submission, node job lifecycle, log streaming, and beacon discovery, including authentication and authorization layers.
- Document API versioning strategy, error envelopes, and pagination/streaming semantics for large job logs.
- Implement OpenAPI (or equivalent) specifications and auto-generate server/client scaffolding.
- Establish rate limiting, mutual TLS requirements, and audit logging across all surfaces.

## Definition of Done
- Published API specification checked into `docs/v2/api.md` and linked from the CLI docs, including examples for every endpoint.
- Control plane, node, and beacon services expose the stable API versions with automated contract tests.
- Observability dashboards track request success rates, latency, and error codes per endpoint family.

## Tests
- Contract tests that validate server responses against generated clients.
- Security tests covering auth failures, rate limit enforcement, and TLS misconfiguration.
- Load tests simulating concurrent Mods submissions to ensure latency and throughput targets are met.
