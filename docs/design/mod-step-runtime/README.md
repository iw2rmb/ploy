# Mod Step Runtime Pipeline Design Spec

- **Identifier**: `roadmap-mod-step-runtime`
- **Status**: [x] Draft · [ ] In progress · [ ] Completed — last updated
  2025-10-21
- **Linked Tasks**:
  - [ ] `roadmap-mod-step-runtime-02` –
        `../../tasks/roadmap/02-mod-step-runtime.md`
- **Blocked by**:
  - `../control-plane/README.md`
- **Unblocks**:
  - `roadmap-mod-step-runtime-02` / `../../tasks/roadmap/02-mod-step-runtime.md`
- **Last Verification**: 2025-10-21 — Reviewed `../../v2/README.md`,
  `../../v2/job.md`, `../../v2/shift.md`, `../../v2/mod.md`
- **Upstream Dependencies**:
  - `../../v2/README.md`
  - `../../v2/job.md`
  - `../../v2/shift.md`
  - `../../v2/mod.md`

## Intent

Deliver a workstation-first Mods runtime that executes step manifests inside OCI
containers on Ploy nodes. Each step must hydrate repository state, mount
cumulative diffs, hand off execution to the container image, and retain the
container for inspection while enforcing SHIFT build gate validation after every
step.

## Context

`internal/workflow/runner` still speaks Grid, even though the control plane now
offers job scheduling. Nodes cannot yet execute step manifests locally, SHIFT is
invoked via legacy CLI hooks, and artifacts continue to assume Grid storage.
`docs/v2/README.md`, `docs/v2/job.md`, and `docs/v2/mod.md` outline the desired
behaviour: OCI images per step, cumulative diffs, SHIFT enforcement, and
artifact capture fed into IPFS. We need a runtime that matches these docs so
Ploy nodes replace Grid.

## Goals

- Define a typed step manifest contract consumed by CLI and nodes, covering
  inputs, outputs, environment, and runtime constraints.
- Implement a node-local step runner that hydrates repo snapshots plus prior
  diffs, launches containers, streams logs, and retains containers for
  inspection.
- Integrate SHIFT as a Go API/service call for every step, short-circuiting
  downstream stages when validation fails while surfacing actionable
  diagnostics.
- Capture step outputs (diff tarballs, logs, metrics) so they are ready for IPFS
  publication without referencing Grid storage (publication wiring lands in
  `roadmap-mod-step-runtime-03`).

## Non-Goals

- Replacing IPFS artifact publishing itself (covered by
  roadmap-ipfs-artifact-store).
- Implementing remote node scheduling (handled by the control plane design).
- Shipping CLI UX for multi-cluster federation or new healing flows.

## Current State

- Mods pipeline builds stages through `internal/workflow/runner` but hands
  execution to Grid via `GridClient.ExecuteStage`.
- No manifest schema exists for step-level execution; CLI stubs rely on legacy
  job metadata.
- Workspace hydration uses temporary directories but lacks snapshot + diff
  layering semantics.
- SHIFT integration depends on sandbox runners embedded in Grid; no reusable API
  invocation exists.
- Artifact capture relies on Grid references and cannot publish to IPFS via
  node-local clients.

## Proposed Architecture

### Overview

1. CLI compiles Mods manifests into a `StepManifest` list (plan, apply,
   validate) and submits to the control plane.
2. Control plane enqueues step jobs that reference manifest IDs, repo snapshot
   CIDs, and cumulative diff CIDs.
3. Node runtime claims a job, hydrates the workspace by materialising the
   baseline snapshot and replaying previous diff bundles, then mounts both into
   a temporary directory exposed to the container.
4. Step runner creates a container via the local Docker Engine API (using
   `containerd` compatible semantics), binds the hydrated workspace read-write,
   injects manifest-defined environment variables, and runs the command.
   `auto-remove` stays disabled so containers remain for inspection.
5. After container exit, runner captures stdout/stderr, exit code, timing, and
   generates a diff tarball by comparing the workspace against the hydrated
   baseline. Diff tarball and logs are staged for the IPFS-backed artifact
   publisher delivered in `roadmap-mod-step-runtime-03`.
6. Runner invokes the SHIFT API with the hydrated workspace path and
   manifest-provided build gate config. On failure, the job is marked failed
   with SHIFT diagnostics; on success, the pipeline proceeds.
7. Runner emits job completion payload to control plane with artifact metadata
   (diff CID, log CID, SHIFT report CID) and transitions container to an
   `inspection_ready` state so CLI commands can `docker start --attach` or
   `docker cp`.

### Interfaces & Contracts

- **StepManifest** (new `internal/workflow/contracts` struct):
  - `id`, `name`, `image`, `command`, `args`, `working_dir`, `env`, `inputs`,
    `outputs`, `artifacts`, `shift_profile`, `resources`.
  - `inputs` reference snapshot/diff CIDs with mount modes (`ro` baseline, `rw`
    overlay).
  - `outputs` annotate expected diff paths and metrics (for reporting).
  - Diff artifacts generated from manifests MUST be packaged as tarballs before
    publication.
  - `shift_profile` describes the sandbox profile to run post-step (static
    checks deferred to a later iteration).
- **StepJobSpec** (new `internal/workflow/runtime` contract):
  - Houses manifest ID, workspace hydration plan, container runtime settings,
    retention policy.
- Control plane job payload updated to include manifest references + hydration
  instructions.
- CLI updates: `ploy mod run` compiles manifest per Mod, serializes into control
  plane submission.
- Node API: new `internal/workflow/runtime/step` package offering
  `Runner.Run(ctx, StepJob) (Result, error)`.
- SHIFT integration: `internal/workflow/buildgate` exposes
  `Client.Validate(ctx, Spec) (Report, error)` reused in runtime.

### Data Model & Persistence

- Job payload holds:
  - `snapshot_cid`: baseline repository snapshot from IPFS.
  - `prior_diffs`: ordered list of diff CIDs to replay.
  - `manifest_id`: reference to step manifest.
  - `shift_config`: inline profile or manifest reference.
- Runner stores execution metadata in etcd job record:
  - `container_id`, `workspace_path`, `started_at`, `completed_at`, `exit_code`,
    `retry_attempt`.
  - Artifact metadata: diff CID, log CID, SHIFT report CID, metrics JSON CID.
- Artifact publisher attaches diff/log/report bundles to IPFS using
  deterministic names `{job-id}-{kind}.tar.xz`.
- Container retention tracked via control plane: jobs completing with
  `retain_container=true` move to `inspection_ready`. GC respects retention TTL
  from manifest.

### Failure Modes & Recovery

- **Container exit != 0**: job marked failed; SHIFT still runs to collect
  diagnostics unless manifest disables post-failure validation. Runner stores
  exit code, tail logs, and errors.
- **SHIFT failure**: pipeline stops, job status `failed`, SHIFT diagnostics
  returned via artifacts.
- **Workspace hydration error**: runner marks job `failed` with
  `error.reason = hydration_failed`, container never created.
- **Artifact publish failure**: job transitions to `failed`; diff/logs remain
  locally until GC. Runner retries publish per manifest retry policy.
- **Docker daemon unavailable**: runner surfaces
  `ErrContainerRuntimeUnavailable`; control plane can retry or quarantine node.
- **Inspection retention expiration**: GC removes container and workspace after
  TTL; CLI warns if retention expired.

## Dependencies & Interactions

- Extends `internal/workflow/contracts` with manifest structs and job payload
  fields.
- Adds `internal/workflow/runtime/step` for container orchestration and
  hydration utilities.
- Updates `internal/workflow/runner` to use control plane job payload instead of
  Grid.
- Requires Docker Engine or compatible runtime on nodes; uses Go Docker client.
- Reuses `internal/workflow/buildgate` runner via new façade to SHIFT service.
- Coordinates with artifact publisher task (roadmap-ipfs-artifact-store) to
  ensure diff/log CIDs are pushed to IPFS.

## Risks & Mitigations

- **Improper mount layering corrupts workspace**
  - Impact: Steps see inconsistent state
  - Mitigation: Use copy-on-write overlay approach: hydrate baseline read-only,
    stage diffs into dedicated overlay, add tests covering diff ordering.
- **SHIFT invocation adds significant latency**
  - Impact: Longer pipeline runtime
  - Mitigation: Allow manifests to flag `shift_skip_on_failure` for non-critical
    steps; parallelize static checks once re-enabled.
- **Container retention exhausts disk**
  - Impact: Node destabilisation
  - Mitigation: Manifest-level retention TTL plus CLI inspection commands to
    prune; GC monitors container count.
- **Artifact publish fails mid-run**
  - Impact: Lost diffs/logs
  - Mitigation: Retry with exponential backoff; persist local copies until
    publish succeeds.

## Observability & Telemetry

- Metrics: step duration, SHIFT duration, diff size, container exit codes,
  publish latency.
- Logs: structured events for hydration, container lifecycle, SHIFT results.
- Traces: optional span for container execution + SHIFT call with job ID
  attributes.
- CLI inspection commands fetch logs and container IDs for debugging.

## Test Strategy

- **Unit**: manifest validation (table-driven), container config assembly, SHIFT
  error propagation, hydration planner.
- **Integration**: run sample OCI images (e.g., `busybox`,
  `ghcr.io/ploy/mods-dev`) in local Docker; verify diff capture and SHIFT
  invocation via stub service.
- **CLI Golden**: status output for success/failure/inspection flows.
- **E2E**: follow-up once IPFS publisher lands to replay Mods plan end-to-end.

## Rollout Plan

1. Land manifest contracts + CLI compilation (draft mode).
2. Implement node step runner with local hydration and container lifecycle; gate
   behind feature flag `PLOY_USE_LOCAL_RUNTIME`.
3. Integrate SHIFT API and artifact publisher; enable default on workstation
   nodes.
4. Update documentation and CLI inspection commands; remove Grid dependencies
   from runner.

## Open Questions

- Should step manifests support multi-container steps (sidecars)? Potential
  follow-up.
- Do we need sandboxed runtimes beyond Docker (e.g., Podman rootless)?
  Investigate after initial implementation.
- How do we version manifest schemas for backward compatibility? Proposal: embed
  schema version in manifest metadata.

## Follow-Up Work (2025-10-21)

- [ ] Planned –
      [roadmap-mod-step-runtime-03 Artifact Publisher Integration](../../tasks/roadmap/03-ipfs-artifact-store.md)
      _(update task spec to include dependency)_
- [ ] Planned –
      [roadmap-mod-step-runtime-04 Job Observability Enhancements](../../tasks/roadmap/07-job-observability.md)

Status verification: task entries reviewed on 2025-10-21.

## References

- `../../v2/README.md`
- `../../v2/job.md`
- `../../v2/shift.md`
- `../../v2/mod.md`
