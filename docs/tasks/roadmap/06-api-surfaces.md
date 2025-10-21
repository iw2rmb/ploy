# API Surfaces

## Why

- Ploy v2 introduces dedicated APIs for the control plane, worker nodes, and
  beacon mode (`docs/v2/README.md` and `docs/v2/api.md`).
- Clean, versioned APIs replace Grid-era RPCs and support multi-node
  orchestration without backward compatibility constraints.

## Task Breakdown

### Task 1: Establish API Schemas

- Goal: Define REST or gRPC contracts for control-plane submission, node job
  lifecycle, log streaming, and beacon discovery across all services.
- Deliverables: Drafted OpenAPI (or equivalent) documents in `docs/v2/api.md`
  with examples for each endpoint and authentication requirements represented.
- Dependencies: Inputs from workflow orchestration owners on required request
  and response fields.

### Task 2: Document Versioning and Error Handling

- Goal: Finalize API versioning approach, error envelopes, and pagination or
  streaming semantics for high-volume log data.
- Deliverables: Versioning guidance mirrored in CLI docs, standardized error
  responses, and log pagination guidance referenced from `docs/v2/README.md`.
- Dependencies: Task 1 schemas and CLI UX decisions for surfaced errors.

### Task 3: Security and Compliance Controls

- Goal: Specify mutual TLS, rate limiting thresholds, auth flows, and audit
  logging expectations for each surface.
- Deliverables: Security appendix in `docs/v2/api.md` detailing TLS profiles,
  rate limiting defaults, and required audit events.
- Dependencies: Task 2 error handling outcomes to align logging fields.

### Task 4: Generated Server and Client Scaffolding

- Goal: Implement code generation pipelines that output server and client
  stubs from the agreed specifications.
- Deliverables: Automated generation scripts wired into `make build`, initial
  scaffolded packages for control plane, node, and beacon services, and build
  documentation updates.
- Dependencies: Task 1 specifications finalized and reviewed.

### Task 5: Service Integration and Contract Testing

- Goal: Expose stable API versions on the control plane, node, and beacon
  services with regression safeguards.
- Deliverables: Services upgraded to use generated scaffolds, CLI integration
  points updated, and contract tests covering happy-path and error scenarios.
- Dependencies: Task 4 scaffolding plus environment variables audited per
  `docs/envs/README.md`.

### Task 6: Observability and Performance Instrumentation

- Goal: Track API adoption with dashboards, latency/error metrics, and load
  validation at target concurrency.
- Deliverables: Dashboards wired into the observability stack, synthetic load
  tests that verify throughput targets, and alerts for SLA breaches.
- Dependencies: Task 5 integration to supply production-like telemetry.

## Definition of Done

- Published API specification checked into `docs/v2/api.md` and linked from the
  CLI docs, including examples for every endpoint.
- Control plane, node, and beacon services expose the stable API versions with
  automated contract tests.
- Observability dashboards track request success rates, latency, and error codes
  per endpoint family.

## Tests

- Contract tests that validate server responses against generated clients.
- Security tests covering auth failures, rate limit enforcement, and TLS
  misconfiguration.
- Load tests simulating concurrent Mods submissions to ensure latency and
  throughput targets are met.
