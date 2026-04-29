# Child Build Short Polling for `mig`/`heal`

## Summary
Define a deterministic contract for starting a child build-gate job from within `mig` or `heal` execution and waiting for completion using amata `polling.short`.

The contract must produce a real control-plane child job record (auditable in `jobs`), with deterministic status polling semantics and explicit auth boundaries.

## Scope
In scope:
- Ploy API and runtime contract for child build creation from running `mig`/`heal` jobs.
- Job-scoped create/status endpoints consumed by amata `polling.short`.
- Runtime env/input wiring required so `mig` and `heal` can call the endpoints.
- Status mapping and terminal/success rules used by `done_when`/`success_when`.
- Removal of `hook` job type scheduling/execution from this migration child-build flow.
- SBOM lifecycle invariants for this flow: non-healable SBOM, post-`pre_gate` SBOM+classpath capture, and post-`post_gate` final SBOM preservation.

Out of scope:
- Changes to amata `polling.short` implementation itself (already implemented in `/Users/vk/@iw2rmb/amata`).
- Amata workflow migration/spec edits (tracked in an external DD outside this repository).
- Replacing existing planned `pre_gate`/`post_gate`/`gate_retry` lifecycle.
- Legacy hook/runtime-hook behavior restoration.

## Why This Is Needed
`mig`/`heal` lanes currently cannot request an on-demand child build and synchronously wait for its result through a typed, resumable polling contract. Existing gate transitions are server-planned or inserted by failure handlers, not requested from inside a running step.

Without a dedicated request+poll contract, workflows must shell out with ad hoc HTTP logic, losing deterministic timeout/resume and schema validation guarantees already available in amata `polling.short`.

## Goals
- Allow `mig` and `heal` to request child build jobs through a stable API contract.
- Keep child builds visible as first-class control-plane jobs with canonical statuses.
- Define a single `polling.short` pattern for request + status polling.
- Keep terminal/success semantics deterministic and auditable.
- Remove hook-job insertion from this flow so child build validation is hook-free.
- Make SBOM behavior deterministic: never healed, captured after successful `pre_gate`, and preserved after successful `post_gate`.

## Non-goals
- Introducing a new top-level orchestration lane outside existing job model.
- Making `polling.short` ploy-specific.
- Backward compatibility for prior ad hoc scripts.

## Current Baseline (Observed)
- Amata already implements `polling.short` schema + runtime + checkpoint resume (`/Users/vk/@iw2rmb/amata/internal/executor/pollingshort/*`, `/Users/vk/@iw2rmb/amata/internal/runtime/builtins.go`, `/Users/vk/@iw2rmb/amata/schemas/polling.short.amata.schema.json`).
- Ploy has no parent-job-scoped endpoint for a running `mig`/`heal` container to create a child build job (`internal/server/handlers/register.go`).
- Worker status endpoint exists, but is ownership-guarded and not exposed as a child-build contract (`internal/server/handlers/jobs_status.go`, `docs/api/paths/jobs_job_id_status.yaml`).
- Runtime hook chain insertion exists in lifecycle completion paths (`internal/server/handlers/jobs_complete_service_runtime_hooks.go`).
- Healing insertion currently includes retry-SBOM in the chain (`failed_gate -> heal -> retry_sbom -> gate_retry`) (`internal/server/handlers/nodes_complete_healing.go`).
- Base planning currently drafts SBOM jobs in gate-cycle prelude (`internal/server/handlers/migs_ticket.go`).
- Healing jobs currently receive recovery HTTP/TLS env (`PLOY_SERVER_URL`, cert paths, optional `PLOY_API_TOKEN`), but this wiring is not generalized to `mig` jobs (`internal/nodeagent/recovery_runtime.go`, `internal/nodeagent/execution_orchestrator_jobs.go`).
- SBOM execution currently materializes persisted `sbom.spdx.json` and Java classpath outputs (`internal/nodeagent/execution_orchestrator_jobs.go`).
- Canonical job statuses are fixed to `Created|Queued|Running|Success|Fail|Error|Cancelled` (`internal/domain/types/statuses.go`).

## Target Contract or Target Architecture
### End-to-end repo flow
Per repo run:

1. `pre_gate` runs first.
2. On successful `pre_gate`, run SBOM and generate the only `.classpath` for the run.
3. Execute `mig`.
4. Execute `post_gate`.
5. On `post_gate` failure, start `heal -> gate_retry` loop.
6. Repeat `heal -> gate_retry` until success or retry budget exhaustion.
7. On successful flow completion, run final SBOM as the last step and preserve resulting SBOM snapshot (no new `.classpath` contract at this step).

### Healing trigger rules
- Only `post_gate` and `gate_retry` failure can trigger `heal`.
- `pre_gate` failure is terminal for the repo attempt and must not create `heal` children.

### Job-scoped child build API
Add job-scoped endpoints on the control plane (worker auth scope):

1. `POST /v1/jobs/{parent_job_id}/builds`
- Purpose: create a child build job requested by currently running `mig`/`heal` job.
- Required request fields:
  - `build_kind` (fixed to `gate_retry` in v1)
  - `reason` (short machine-readable reason)
- Response:
  - `child_job_id`
  - `status_url` (absolute URL for polling)
  - `status` (initial canonical status)

2. `GET /v1/jobs/{parent_job_id}/builds/{child_job_id}`
- Purpose: return canonical child job status for polling.
- Response:
  - `job_id`
  - `status` (`Created|Queued|Running|Success|Fail|Error|Cancelled`)
  - `terminal` (`true` for `Success|Fail|Error|Cancelled`)
  - `success` (`true` only for `Success`)

### Auth and ownership
- Calls are authorized as worker-scoped job calls.
- Create endpoint must verify:
  - `parent_job_id` exists and is `Running`.
  - `parent_job_id` belongs to the calling worker identity.
  - parent job type resolved from `parent_job_id` is `mig` or `heal` only.
- Status endpoint must verify parent-child linkage and caller ownership for the parent job.

### Child job persistence and execution contract
- Child build is persisted as a normal control-plane `jobs` row with `job_type=gate_retry`.
- Child job metadata must include parent linkage:
  - `trigger.parent_job_id`
  - `trigger.kind=child_gate_request`
- Child job must remain visible in existing run/repo job listings and logs.
- This child-build flow must not schedule or execute `hook` jobs.

### Child-gate artifact contract
- Gate/build lineage artifacts are materialized in the parent job `/out` after each child build completion.
- Paired artifact names:
  - `/out/re_build-gate-{n}.log`
  - `/out/errors-{n}.yaml`
- Artifact index `{n}` is monotonic per run/repo lineage and starts from the first gate baseline as `1`.
- Every child gate increments `{n}` by exactly `1`, so total paired artifact count equals `child_gates_executed + 1`.
- Materialization happens after every child build, preserving deterministic order for later `heal`/analysis steps.

### SBOM lifecycle invariants
- SBOM jobs in this flow are never healable. On SBOM failure, fail the repo attempt (or cancel remainder by existing terminal policy); do not insert `heal`/`gate_retry` for SBOM failure.
- Execute SBOM after successful `pre_gate` and persist resulting SBOM snapshot plus generated `.classpath` artifact (single classpath source for the run).
- Execute SBOM after successful `post_gate` as the last stage for final-state preservation; requirement is persisted resulting SBOM snapshot only.
- Do not insert retry-SBOM between `heal` and `gate_retry`.

### `polling.short` usage contract for workflow integration
Use a single typed pattern:

```yaml
- type: polling.short
  request:
    method: POST
    url: "{{ env.PLOY_SERVER_URL }}/v1/jobs/{{ env.PLOY_JOB_ID }}/builds"
    headers:
      Authorization: "Bearer {{ env.PLOY_API_TOKEN }}"
      Content-Type: "application/json"
    body:
      build_kind: "gate_retry"
      reason: "child_build_validation"
  confirm:
    method: GET
    url: "{{ ctx.value.request.response.value.status_url }}"
    headers:
      Authorization: "Bearer {{ env.PLOY_API_TOKEN }}"
  done_when: "ctx.value.confirm.response.value.terminal == true"
  success_when: "ctx.value.confirm.response.value.success == true"
```

Rules:
- `done_when` and `success_when` must resolve to booleans.
- Terminal status is exactly `Success|Fail|Error|Cancelled`.
- Success is exactly `status == Success`.
- Timeouts/checkpoint resume semantics are inherited from implemented amata `polling.short` contract.

## Implementation Notes
- Control-plane API:
  - register new job-scoped child-build routes in `internal/server/handlers/register.go` and OpenAPI docs under `docs/api/paths`.
  - add child-build create/status handlers in `internal/server/handlers` with ownership checks aligned to existing worker job status rules.
- Persistence/orchestration:
  - create child `gate_retry` jobs with parent linkage in `jobs.meta`.
  - keep status transitions in canonical job lifecycle; no custom status enum.
- Lifecycle cleanup:
  - remove hook planning/insertion from this migration child-build path (`jobs_complete_service_runtime_hooks.go` integration point).
  - update completion routing so only failed `post_gate`/`gate_retry` can evaluate healing insertion (`internal/workflow/lifecycle/orchestrator.go` integration point).
- SBOM lifecycle:
  - route SBOM failures to terminal repo failure policy without healing insertion.
  - schedule SBOM immediately after successful `pre_gate` and persist SBOM + the run's only `.classpath`.
  - schedule final SBOM after successful `post_gate` for resulting-state SBOM preservation only.
  - remove retry-SBOM insertion from healing chain in this flow (`nodes_complete_healing.go` integration point).
- Artifact materialization:
  - after every child build completion, materialize `/out/re_build-gate-{n}.log` and `/out/errors-{n}.yaml` into the parent job workspace with contiguous indexing.
- Node/runtime wiring:
  - inject `PLOY_JOB_ID` into both `mig` and `heal` manifests.
  - keep `PLOY_SERVER_URL` and auth token/cert wiring available for both lanes so `polling.short` requests are possible.

## Milestones
1. Child-build API contract.
- Scope: add create/status endpoint specs + handler scaffolding + request/response schema tests.
- Expected result: worker can create and read child-build status via typed endpoints.
- Testable outcome: endpoint tests cover ownership, invalid parent state, and status projection.

2. Child job persistence and lifecycle.
- Scope: create persisted child `gate_retry` rows with parent linkage, canonical status transitions, and no hook-job insertion in this flow.
- Expected result: child job appears in repo/run job listings, moves through normal lifecycle, and remains hook-free.
- Testable outcome: integration tests show child job row creation, terminal completion, metadata linkage, and zero hook jobs created.

3. Runtime env wiring for `mig`/`heal`.
- Scope: inject required env vars for endpoint invocation in both lanes.
- Expected result: `polling.short` step has all identifiers and auth material at runtime.
- Testable outcome: manifest/unit tests validate env presence for both job types.

4. Child-gate artifact lineage.
- Scope: materialize deterministic `/out/re_build-gate-{n}.log` and `/out/errors-{n}.yaml` pairs in parent job for baseline + each child gate.
- Expected result: artifact numbering is contiguous and auditable per run/repo.
- Testable outcome: for `k` child gates, artifacts include exactly `k+1` paired indices with no gaps/duplicates.

5. SBOM lifecycle realignment.
- Scope: enforce non-healable SBOM policy, move/confirm SBOM execution after `pre_gate` and after `post_gate`, and remove retry-SBOM from healing chain.
- Expected result: deterministic SBOM preservation without SBOM healing loops.
- Testable outcome: integration tests show (a) SBOM failure does not create heal/gate_retry, (b) successful `pre_gate` produces persisted SBOM + the run's only `.classpath`, (c) successful `post_gate` yields final persisted SBOM snapshot without a second classpath contract.

## Acceptance Criteria
- `mig` and `heal` can trigger a child build through a typed POST contract.
- Child build is persisted as a normal job and visible in existing status/log surfaces.
- Polling endpoint returns canonical status plus explicit `terminal` and `success` booleans.
- Workflow `done_when`/`success_when` mapping is deterministic and matches canonical status rules.
- This flow schedules and executes zero `hook` jobs.
- Repo flow is explicit: `pre_gate -> sbom(pre_gate) -> mig -> post_gate -> (heal -> gate_retry)* -> sbom(final)`.
- Only failed `post_gate`/`gate_retry` can create heal children; failed `pre_gate` is terminal.
- Gate artifact lineage materializes deterministic parent `/out/re_build-gate-{n}.log` and `/out/errors-{n}.yaml` pairs with total count `child_gates_executed + 1`.
- SBOM failures in this flow never trigger healing.
- Successful `pre_gate` is followed by persisted SBOM and the run's only generated `.classpath`.
- Successful `post_gate` is followed by final persisted SBOM snapshot only.
- No custom/legacy status shape is introduced.

## Risks
- Incorrect ownership checks could allow cross-job status access.
- Missing runtime env wiring in one lane (`mig` or `heal`) will break workflow portability.
- Child-job fan-out may increase node load if rate limiting is not enforced.
- Workflow authors may encode weak `done_when`/`success_when` expressions despite typed contract.

## References
- `design/bump.md` (this document)
- `internal/server/handlers/register.go`
- `internal/server/handlers/jobs_status.go`
- `internal/server/handlers/jobs_complete_service_runtime_hooks.go`
- `internal/server/handlers/migs_ticket.go`
- `internal/server/handlers/nodes_complete_healing.go`
- `internal/workflow/lifecycle/orchestrator.go`
- `internal/nodeagent/execution_orchestrator_jobs.go`
- `internal/nodeagent/recovery_runtime.go`
- `internal/domain/types/statuses.go`
- `docs/api/paths/jobs_job_id_status.yaml`
- Amata engine documentation (external repository).
- Amata polling-short executor implementation (external repository).
- Amata polling-short schema contract (external repository).
