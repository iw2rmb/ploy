# Kubernetes Jobs Runtime For Claimed Jobs

## Summary
Introduce a Kubernetes Job execution backend for nodeagent so claimed Ploy jobs (`pre_gate`, `mig`, `heal`, `gate_retry`, `post_gate`, `mr`) can run as Kubernetes Jobs instead of local Docker containers, while keeping Ploy control-plane orchestration (`jobs` table, `next_id`, claim/complete APIs) unchanged.

## Scope
In scope:
- Nodeagent runtime changes to support a new `k8s-job` execution backend.
- Mapping one claimed `job_id` to one Kubernetes `Job`.
- K8s-based log collection, terminal state detection, and completion upload to existing `/v1/jobs/{id}/complete`.
- K8s equivalent of startup crash reconciliation currently implemented for Docker containers.
- Build Gate and standard step execution parity under Kubernetes runtime.

Out of scope:
- Replacing DB-backed orchestration with Kubernetes-native workflow CRDs (Argo, Tekton, etc.).
- Changing run/job scheduling semantics in server/store.
- Multi-cluster execution, cross-namespace tenancy model, or quota management redesign.
- Reworking API schemas for claim/complete.

## Why This Is Needed
Current execution is tightly coupled to local Docker daemon lifecycle in nodeagent, which increases operational coupling and limits environments where container execution should be delegated to Kubernetes-native control loops and scheduling.

Concrete pressure points:
- Runtime initialization binds to Docker directly in nodeagent ([execution_orchestrator.go](/Users/v.v.kovalev/@iw2rmb/ploy/internal/nodeagent/execution_orchestrator.go)).
- Container lifecycle is hard-wired to Docker APIs in step runtime ([container_docker.go](/Users/v.v.kovalev/@iw2rmb/ploy/internal/workflow/step/container_docker.go)).
- Crash reconcile logic scans Docker containers by labels ([crash_reconcile.go](/Users/v.v.kovalev/@iw2rmb/ploy/internal/nodeagent/crash_reconcile.go)).

## Goals
- Preserve existing orchestration authority in Ploy server/store.
- Allow nodeagent to execute claimed jobs via Kubernetes Jobs with deterministic idempotency.
- Keep current claim/complete behavior and job status transitions unchanged.
- Preserve gate/healing semantics and metadata behavior regardless of runtime backend.
- Keep failure handling explicit and deterministic (including duplicate replay and partial failures).

## Non-goals
- No control-plane migration to Kubernetes as source of truth.
- No backward-compatibility requirement for undocumented runtime internals.
- No attempt to encode Ploy DAG (`next_id`) into Kubernetes dependencies.

## Current Baseline (Observed)
- Claim authority is server-side DB logic (`ClaimJob`) and run/repo status transition ([jobs.sql](/Users/v.v.kovalev/@iw2rmb/ploy/internal/store/queries/jobs.sql), [nodes_claim_service.go](/Users/v.v.kovalev/@iw2rmb/ploy/internal/server/handlers/nodes_claim_service.go)).
- Nodeagent executes claimed jobs per `job_type` dispatch ([execution_orchestrator.go](/Users/v.v.kovalev/@iw2rmb/ploy/internal/nodeagent/execution_orchestrator.go)).
- Runtime is assembled from Docker container runtime + Docker gate executor ([execution_orchestrator.go](/Users/v.v.kovalev/@iw2rmb/ploy/internal/nodeagent/execution_orchestrator.go), [interfaces.go](/Users/v.v.kovalev/@iw2rmb/ploy/internal/workflow/step/interfaces.go)).
- Gate execution path is Docker-based and deeply integrated with gate metadata/report upload ([execution_orchestrator_gate.go](/Users/v.v.kovalev/@iw2rmb/ploy/internal/nodeagent/execution_orchestrator_gate.go)).
- Startup reconciliation discovers running/recent terminal work by Docker container labels and timestamps ([crash_reconcile.go](/Users/v.v.kovalev/@iw2rmb/ploy/internal/nodeagent/crash_reconcile.go)).

## Target Contract or Target Architecture
### Authority and invariants
- Server remains authoritative for orchestration:
  - Only `/v1/nodes/{id}/claim` decides which job runs next.
  - Only `/v1/jobs/{id}/complete` decides terminal status transitions.
- Kubernetes is execution substrate only.
- Exactly one active Kubernetes `Job` per claimed `job_id`.
- Runtime backend must be idempotent: if nodeagent restarts, it reattaches to existing K8s `Job` for that `job_id` instead of creating duplicates.

### Backend selection
- Add `execution_backend` in nodeagent config with values:
  - `docker` (default)
  - `k8s-job`
- Backend is node-scoped; no per-job backend switching.

### K8s Job identity contract
- Job name: deterministic from `job_id` (sanitized), e.g. `ploy-job-<job_id>`.
- Required labels:
  - `ploy.run_id`
  - `ploy.repo_id`
  - `ploy.job_id`
  - `ploy.job_type`
  - `ploy.node_id`
- Required annotations:
  - `ploy.next_id` (for observability only)
  - `ploy.attempt`

### Runtime data contract (`/workspace`, `/in`, `/out`)
- Nodeagent runs in Kubernetes with a shared RWX PVC mounted at a configured path.
- For each claimed job, nodeagent prepares deterministic directories on PVC:
  - `<root>/<job_id>/workspace`
  - `<root>/<job_id>/in`
  - `<root>/<job_id>/out`
- Nodeagent retains existing hydration and cross-phase input assembly behavior, but target path is the PVC.
- Spawned K8s Job mounts these paths at `/workspace`, `/in` (ro), `/out`.

### Completion contract
- Nodeagent watches Kubernetes Job/Pod state.
- On terminal state:
  - collect logs
  - collect exit code
  - upload artifacts/diffs/metadata using existing uploaders
  - call existing completion API
- `409 Conflict` behavior for startup replay remains idempotent success.

### Reconciliation contract
- Replace Docker-based startup discovery with Kubernetes-based discovery by `ploy.node_id` + terminal timestamp window equivalent to current 120s.
- Reattach running jobs and replay recent terminal jobs using existing status uploader semantics.

## Implementation Notes
### Core module changes
- `internal/nodeagent`:
  - Add backend config parsing/validation (`docker|k8s-job`).
  - Extract runtime construction from `initializeRuntime` into backend factory.
  - Add Kubernetes reconciliation implementation parallel to current Docker reconcile flow.
  - Add Kubernetes executor/watcher for standard jobs and gate jobs.
- `internal/workflow/step`:
  - Add `ContainerRuntime` implementation for Kubernetes Job-backed execution.
  - Add `GateExecutor` implementation for Kubernetes backend (or a backend-agnostic gate executor using selected runtime).

### Data flow changes
- Claim flow unchanged.
- Before execution:
  - prepare workspace on PVC (rehydration/diff application as today).
  - materialize `/in` recovery inputs as today for heal/gate_retry.
- Execution:
  - create or reattach deterministic Kubernetes `Job`.
  - stream logs from Pod(s).
  - wait for terminal status.
- Post execution:
  - upload diffs/artifacts/status through unchanged control-plane APIs.

### Failure and retry rules
- Kubernetes `Job.spec.backoffLimit=0` to avoid hidden retries that violate Ploy retry semantics.
- Nodeagent, not Kubernetes, remains responsible for whether a claimed job is retried/replayed.
- If K8s API create returns AlreadyExists:
  - treat as reattach path if labels/annotations match `job_id`.
  - otherwise fail job as infrastructure error.

### Security/runtime assumptions
- Nodeagent service account requires minimal RBAC: get/list/watch/create/delete Jobs/Pods in its namespace.
- Existing mTLS/bearer path to Ploy server remains unchanged.
- No new server API required.

## Milestones
### Milestone 1: Backend skeleton and config
Scope:
- Add `execution_backend` config and runtime backend factory.
- Keep Docker as default.
Expected Results:
- Nodeagent starts with either backend selected; no behavior change on `docker`.
Testable outcome:
- Unit tests for config parsing/defaulting and factory dispatch.

### Milestone 2: K8s standard job runtime
Scope:
- Implement Kubernetes-backed `ContainerRuntime` for `mig`, `heal`, `mr`.
- PVC path preparation for `/workspace`, `/in`, `/out`.
Expected Results:
- Claimed standard jobs can complete through K8s backend and report terminal status.
Testable outcome:
- Integration test with fake Kubernetes client for create/watch/log/exit handling.

### Milestone 3: K8s gate runtime parity
Scope:
- Implement gate execution through K8s backend preserving gate metadata/report paths.
Expected Results:
- `pre_gate/post_gate/gate_retry` behavior parity with Docker mode (pass/fail/cancel paths).
Testable outcome:
- Existing gate orchestration tests adapted to run against backend abstraction.

### Milestone 4: Startup reconciliation parity
Scope:
- K8s-based startup reconcile for running/recent terminal jobs.
Expected Results:
- Node restart recovers in-flight jobs and replays recent terminal completions idempotently.
Testable outcome:
- Reconcile tests covering running reattach, terminal replay, and 409 idempotent success.

## Acceptance Criteria
- With `execution_backend=docker`, all existing nodeagent tests in current scope continue to pass.
- With `execution_backend=k8s-job`, end-to-end claim->execute->complete works for:
  - `mig`
  - `heal`
  - `pre_gate`
  - `gate_retry`
  - `post_gate`
- No server/store schema or claim/complete API changes are required.
- Duplicate Kubernetes Job creation for same `job_id` does not occur under restart/reconcile scenarios.
- Startup replay keeps current idempotency behavior for completion conflicts.

## Risks
- PVC semantics and I/O performance may differ by storage class and affect workspace-heavy runs.
- Log ordering and chunking from Pod logs can differ from current Docker log stream behavior.
- Kubernetes watch edge cases (missed events, transient API errors) can cause delayed completion unless carefully handled.
- Gate resource usage telemetry may not be equivalent to Docker stats without explicit Pod metrics integration.

## References
- [internal/nodeagent/execution_orchestrator.go](/Users/v.v.kovalev/@iw2rmb/ploy/internal/nodeagent/execution_orchestrator.go)
- [internal/nodeagent/execution_orchestrator_jobs.go](/Users/v.v.kovalev/@iw2rmb/ploy/internal/nodeagent/execution_orchestrator_jobs.go)
- [internal/nodeagent/execution_orchestrator_gate.go](/Users/v.v.kovalev/@iw2rmb/ploy/internal/nodeagent/execution_orchestrator_gate.go)
- [internal/nodeagent/crash_reconcile.go](/Users/v.v.kovalev/@iw2rmb/ploy/internal/nodeagent/crash_reconcile.go)
- [internal/workflow/step/interfaces.go](/Users/v.v.kovalev/@iw2rmb/ploy/internal/workflow/step/interfaces.go)
- [internal/workflow/step/container_docker.go](/Users/v.v.kovalev/@iw2rmb/ploy/internal/workflow/step/container_docker.go)
- [internal/server/handlers/nodes_claim_service.go](/Users/v.v.kovalev/@iw2rmb/ploy/internal/server/handlers/nodes_claim_service.go)
- [internal/store/queries/jobs.sql](/Users/v.v.kovalev/@iw2rmb/ploy/internal/store/queries/jobs.sql)
- [docs/migs-lifecycle.md](../docs/migs-lifecycle.md)
- [docs/build-gate/README.md](../docs/build-gate/README.md)
