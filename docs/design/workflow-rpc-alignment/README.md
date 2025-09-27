# Workflow RPC Alignment (Roadmap 22)

## Purpose
Define how Ploy consumes Grid's Workflow RPC using the official SDK/helper, standardise job payloads (`image`, `command`, `env`, `resources`) derived from lane specifications, and align JetStream subject naming so both projects share a single contract.

## Scope
- Replace the bespoke HTTP client under `internal/workflow/grid` with the Grid Workflow RPC SDK while keeping the in-memory stub for workstation tests.
- Adopt the Grid Workflow RPC helper layer once available so configuration, retries, and streaming reuse the upstream abstractions.
- Adapt the workflow runner to build `workflowrpc.JobSpec` payloads from lane metadata, manifest constraints, and Aster toggles.
- Synchronise JetStream subjects between Ploy and Grid (`webhook`, `jobs.<run>.events`) and surface the shared constants through `internal/workflow/contracts`.
- Refresh CLI and documentation to reference the RPC endpoints (`/v1/workflows/rpc/runs*`) instead of the legacy stage API.

## Current Status (2025-09-27)
- Design doc drafted; roadmap tasks created for SDK wiring, job spec composition, subject alignment, and helper adoption.
- Implementation, tests, and documentation updates remain pending.

## Background
Grid consolidated workflow submission, status streaming, and cancellation behind the Workflow RPC service (`docs/design/workflow-rpc/README.md` in Grid). Ploy still posts to `/workflow/stages` and listens on `grid.status.<ticket>`, so live runs fail once Grid enables the real RPC handler. Aligning the contract requires:

- Using the SDK’s typed client so Ploy inherits API evolution automatically.
- Populating `JobSpec` with lane-derived image/command/env/resources, so scheduler cache hints remain intact.
- Reflecting Grid’s current JetStream subjects (`webhook.<tenant>.<source>.<event>`, `jobs.<run_id>.events`) across tickets, checkpoints, and log retrieval.

## Behaviour & Architecture
1. **Client Abstraction**
   - New wrapper in `internal/workflow/grid` composes the SDK client, translating runner stages into RPC submissions.
   - The wrapper injects lane/manifest metadata into `JobSpec.Metadata` (lane, cache key, priority) for scheduler scoring.
   - In-memory grid remains for tests; SDK-backed client is selected when `GRID_ENDPOINT` is set.

2. **Job Payload Construction**
   - Lane definitions (`configs/lanes/*.toml`) gain optional `image`, `command`, `env`, `resources` hints.
   - Runner assembles `JobSpec` using lane defaults plus manifest/Aster overrides; empty fields fall back to lane spec. The payload layout is locked to `image`, `command`, `env`, and `resources` (see [Job Spec Schema](#job-spec-schema-2025-09-27)).
   - Cache keys continue to publish via checkpoints for downstream cache coordination.

3. **Event & Subject Alignment**
   - `internal/workflow/contracts` exposes constants for webhook inbox (`webhook.<tenant>.*`) and status stream (`jobs.<run_id>.events`).
   - Runner consumes status events via the RPC stream (`jobs.<run_id>.events`) instead of polling legacy `grid.status.<ticket>`.
   - Build-gate log retrieval uses the new event subjects to locate artifacts via job CIDs.

4. **CLI & Docs**
   - CLI help references Workflow RPC endpoints, helper configuration flows, and credential expectations.
   - Design index and roadmap entries stay in sync with the milestone status and link to Grid helper documentation for downstream adopters.

## Job Spec Schema (2025-09-27)
- `image` — OCI image for the Grid runtime adapter. Must resolve via lane defaults or manifest overrides.
- `command` — array of executable plus arguments; defaults to the lane command when manifest overrides are absent.
- `env` — map of environment variables merged from lane configuration, manifest toggles, and workflow inputs.
- `resources` — CPU/memory/IO constraints expressed via Grid's `workflowrpc.Resources`; lanes publish deterministic defaults when callers omit overrides.

Additional fields (`workdir`, `secret_refs`, `accelerators`, `metadata`) remain optional but must not omit the four required keys above. Lane validation fails fast if any key is missing so every submitted job matches Grid helper expectations.

## Clarifications (2025-09-27)
- Grid exposes workflow submission under `/v1/workflows/rpc/runs*`. Legacy `/v1/workflows/jobs*` routes were removed during the shift; the SDK and helper target only the RPC surface. Verification on 2025-09-27 confirmed both the HTTP handlers (`../grid/internal/httpapi/workflow_rpc_routes.go`) and the Go SDK (`../grid/sdk/workflowrpc/go/client.go`) point at the RPC endpoints.

## Risks & Mitigations
- **Lane metadata gaps**: introduce validation that lanes declare the minimum job spec fields; fallback defaults remain for existing specs.
- **Stub parity**: extend the in-memory grid to emulate RPC responses so workstation tests remain deterministic.
- **Cross-repo drift**: publish shared subject constants (short-term via duplicated values, long-term via generated package or shared module).

## Dependencies
- Grid Workflow RPC SDK (`grid/sdk/workflowrpc/go`).
- Grid Workflow RPC helper (`grid/sdk/workflowrpc/helper`).
- Existing lane and manifest loaders for cache and command metadata.

## Deliverables
- SDK-backed workflow client with unit tests.
- Runner tests covering `JobSpec` assembly and RPC/helper invocation failure modes.
- Updated documentation (design index, CLI README, build gate design) referencing the new contract and helper guidance.
- Roadmap tasks completed with changelog entries dated on completion.

## References
- Grid Workflow RPC design (`../grid/docs/design/workflow-rpc/README.md`).
- Grid Workflow RPC SDK implementation (`../grid/sdk/workflowrpc/go/client.go`).
- Grid Workflow RPC helper roadmap (`../grid/roadmap/workflow-rpc/04-sdk-helper-layer.md`).
- Grid Workflow RPC helper usage guide (`../grid/sdk/workflowrpc/README.md`).

## Verification (2025-09-27)
- Confirmed RPC routes are served at `/v1/workflows/rpc/runs*` in `../grid/internal/httpapi/workflow_rpc_routes.go`.
- Confirmed SDK client targets the same endpoints and payload schema in `../grid/sdk/workflowrpc/go/client.go`.
- Reviewed helper builder coverage in `../grid/sdk/workflowrpc/helper` tests to ensure retry semantics align with Ploy needs.

## Tests
- Unit tests exercising the new client/helper wrapper (success, HTTP errors, retryable outcomes).
- Runner integration test ensuring job spec carries lane metadata and cache key.
- Contract tests for subject constants and build-gate log retrieval once statuses move to `jobs.<run_id>.events`.
- Helper adoption tests validating Ploy wiring against the new `helper` layer once available.

## Roadmap Tasks
- [ ] `roadmap/workflow-rpc-alignment/01-grid-sdk-client.md`
- [ ] `roadmap/workflow-rpc-alignment/02-runner-job-spec.md`
- [ ] `roadmap/workflow-rpc-alignment/03-subject-alignment.md`
- [ ] `roadmap/workflow-rpc-alignment/04-helper-adoption.md`

## Completion Criteria
- Ploy submits workflow runs via the SDK, producing valid `JobSpec` payloads and receiving status streams from Grid.
- JetStream subjects in contracts and documentation match the Grid implementation.
- Build gate log retrieval and knowledge-base integration operate on the updated subject scheme.
- Documentation (design index, CLI, build gate) reflects the final behaviour and dates.
