# Control Plane Jobs Split

## Context
- `/Users/vk/@iw2rmb/docs/WEBSEARCH.md` is absent, so no prior web research exists for this scope; no external search was required for this purely internal refactor.
- `rg --files | xargs wc -l` shows `docs/api/OpenAPI.yaml` (876 LOC) as the largest file overall, but the heaviest Go source file is `internal/api/httpserver/controlplane_jobs.go` (~574 LOC) containing all job handlers, DTOs, and helper utilities.
- The file mixes unrelated concerns (submission/listing, node-facing actions, stream endpoints, DTO transforms, and etcd lookups), making review and targeted edits risky.

## Goals
1. Split `controlplane_jobs.go` into cohesive files grouped by responsibility while keeping the package API stable.
2. Preserve handler behavior and HTTP surface, ensuring existing tests continue to pass without regressions.
3. Unblock future maintenance by isolating DTO/utility code from HTTP routing.

## Non-Goals
- Changing request/response schemas or control-plane routing.
- Reworking scheduler behavior or etcd storage.
- Modifying CLI/user-facing documentation beyond referencing this refactor if needed.

## Proposed Layout
- `controlplane_jobs_routes.go`: top-level router helpers (`handleJobs`, `handleJobSubpath`, query validation) to keep entrypoints small.
- `controlplane_jobs_mutations.go`: handlers that mutate scheduler state (`handleJobSubmit`, `handleJobComplete`, `handleJobHeartbeat`, `handleClaim`).
- `controlplane_jobs_queries.go`: read-only/list handlers (`handleJobList`, `handleJobGet`).
- `controlplane_job_streams.go`: streaming/log/event endpoints plus `lookupJobKey`, `modsPrefix`, and SSE helpers specific to streaming.
- `controlplane_job_dto.go`: `jobDTO`, `nodeSnapshotDTO`, conversion helpers (`jobDTOFrom`, `copyMap`, etc.), and DTO-only types.

Each new file will keep the `httpserver` package name; all moved functions remain exported only within the package.

## Implementation Notes
- Maintain the existing function signatures to avoid re-wiring other packages.
- Move shared helpers (e.g., `parseLastEventID`) only if they are exclusive to jobs; otherwise leave them in their current files.
- Ensure every function retains the leading comment rule from `/Users/vk/@iw2rmb/docs/AGENTS.md` when relocated.
- Keep imports minimal per file; expect the compiler/goimports to clean up per-file requirements.

## Test Plan (pre-planned per workflow rules)
1. `go test ./internal/api/httpserver -run '(Job|Stream)' -count=1`
2. `make test` for the full guardrail suite and coverage thresholds (≥60% overall, ≥90% critical packages).

These commands will run before and after the refactor to enforce the RED→GREEN→REFACTOR cadence; no new behavior-specific tests are needed because the change is structural.

## Dependencies & References
- [`internal/api/httpserver/controlplane_jobs.go`](../../../internal/api/httpserver/controlplane_jobs.go)
- [`internal/api/httpserver/controlplane_jobs_test.go`](../../../internal/api/httpserver/controlplane_jobs_test.go)
- [`internal/controlplane/scheduler`](../../../internal/controlplane/scheduler)
- [`internal/node/logstream`](../../../internal/node/logstream)

## Status
- Reserved by Codex on 2025-10-26. Implementation pending.
