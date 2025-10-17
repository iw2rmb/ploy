# Mods Grid Restoration Design Spec

- **Identifier**: `shift-mods-grid`
- **Status**: [x] Draft · [x] In progress · [ ] Completed — last updated 2025-10-07
- **Linked Tasks**:
  - [x] `shift-mods-grid-01` – `docs/tasks/mods-grid/01-red.md`
  - [x] `shift-mods-grid-02` – `docs/tasks/mods-grid/02-green.md`
  - [x] `shift-mods-grid-03` – `docs/tasks/mods-grid/03-red.md`
  - [x] `shift-mods-grid-04` – `docs/tasks/mods-grid/04-green.md`
- **Blocked by**:
  - None (follow-ups tracked below)
- **Unblocks**:
  - `docs/tasks/mods-grid/05-refactor.md` (future)
  - Grid catalog registration follow-up (future design)
- **Last Verification**: 2025-10-07 — Re-reviewed tests and noted upcoming Grid
  catalog registration as follow-up work:
  - `go test ./internal/workflow/runner -run TestRunSchedulesHealingPlanAfterBuildGateFailure -count=1`
  - `go test ./cmd/ploy -run TestHandleModRunConfiguresModsFlags -count=1`
  - `go test -tags e2e ./tests/e2e -count=1`
- **Upstream Dependencies**:
  - `../workflow-rpc-alignment/README.md`
  - `../mods/README.md`
  - `../../docs/envs/README.md`

## Intent

Restore Mods workflows on Grid so the workstation CLI can execute
OpenRewrite-based migrations end-to-end, including healing loops. The slice
reintroduces repository materialisation, Grid job specs for Mods stages, and the
build-gate feedback loop that drives LLM/human remediation.

## Parallelisation Snapshot

| Track | Ready When | Owner | Notes |
| --- | --- | --- | --- |
| Repo materialisation RED (`shift-mods-grid-01`) | Design approved | Mods squad | Produces RED tests + E2E guardrails |
| Materialisation GREEN (`shift-mods-grid-02`) | RED tests merged | Mods squad | Enables simple OpenRewrite scenario |
| Healing RED (`shift-mods-grid-03`) | Simple scenario reaches build gate | Mods squad | Adds retry + branching tests |
| Healing GREEN (`shift-mods-grid-04`) | RED failures in place | Mods + Build Gate | Implements planner/runner feedback |

## Shared Components & Unblocking Candidates

- Mods repository fixture tarball (Java 11→17) hosted in local cache for
  tests.
- Lane definitions for OpenRewrite / LLM execution (temporary placeholders in
  Ploy; dedicated Mods catalog repository will publish them via Grid).
- Knowledge base catalog seeded with build gate error signatures to drive planner branching.

## Context

Legacy Mods E2E suites were removed in commit `da348c89`, leaving the CLI with a
planner that emitted Mods stages but no way to run them on Grid. The
`shift-mods-grid` slice set out to close the following gaps (now addressed by
Steps 1–4):

- Workflow tickets lacked repo/branch metadata, preventing runner-side materialisation.
- Mods stages inherited placeholder job specs from general lanes, forcing Grid jobs to execute irrelevant commands.
- Build gate results never fed back into Mods, so healing scenarios stalled after the first failure.
- E2E scenarios in `tests/e2e` intentionally failed to document the missing behaviour.

## Goals

- [x] Enable the "simple OpenRewrite" scenario in `ploy mod run` by
  materialising Git repos and composing Mods-specific job specs against Grid
  lanes.
- [x] Reinstate build-gate healing so planner retries, knowledge-base guidance,
  and optional human gates branch in parallel when failures surface.
- [ ] Register the Mods lane catalog with Grid once workstation parity is
  stable (dedicated repo + namespace served at `/lanes/mods.tar.gz`).

## Non-Goals

- Registering the Mods catalog with Grid (tracked separately under `docs/tasks/mods-grid/05-refactor.md`).
- Reintroducing Nomad/controller-specific scripts removed during legacy teardown.
- Full Grid/VPS integration tests (remains future REFACTOR scope).

## Current State

- `ploy mod run` now accepts `--repo-*` flags, populates `WorkflowTicket.Repo`, and
  routes tickets through a repo-aware events client so workstation runs retain
  repo metadata end-to-end.
- The CLI provisions a `gitWorkspacePreparer` that clones repositories into
  `workspace/` before Mods stages execute, honouring optional workspace hints for
  language-specific layouts.
- Mods lane specs (bundled under `configs/lanes` as `mods-plan.toml`,
  `mods-java.toml`, `mods-llm.toml`, `mods-human.toml`) feed the job composer so
  Grid runs boot the correct containers instead of fallback lanes.
- Build-gate failures dispatch through `handleHealing`, which extracts
  knowledge-base findings from checkpoint metadata, replans Mods stages once,
  and appends `#healN` branches that continue through build/static-check/test
  lanes.
- Stage metadata mirrors planner output
  (`stage_metadata.mods.plan`/`recommendations`), enabling CLI summaries and
  downstream tooling to understand selected recipes and branch justifications.
- Mods E2E harness now executes against the in-memory Grid stub (`go test -tags
  e2e ./tests/e2e`); Grid catalog registration follow-up will point these
  scenarios at the published namespace once available.

## Proposed Architecture

### Delivered (Steps 1–4)

- CLI: `ploy mod run` now shells repository inputs (`--repo-url`, `--repo-base-ref`,
  `--repo-target-ref`, `--repo-workspace-hint`) into `contracts.WorkflowTicket.Repo`
  and decorates the events client so claimed tickets inherit workstation
  overrides (`cmd/ploy/mod_run.go`, `cmd/ploy/workspace_preparer.go`).
- Workspace: a `gitWorkspacePreparer` clones the repo into `<run>/workspace/`,
  checks out the target ref, and scaffolds optional hints that lane scripts
  expect. Runs without repo metadata skip cloning entirely.
- Lanes & job specs: Mods-specific TOML lanes (see `configs/lanes` —
  `mods-plan.toml`, `mods-java.toml`, `mods-llm.toml`, `mods-human.toml`) flow
  through the job
  composer so staged Mods work pulls the correct container images and commands.
- Healing: `handleHealing` inspects build-gate outcomes, converts
  static-check/log findings into `mods.AdviceSignals`, and invokes the planner
  with `PlanInput{Ticket, Signals}`. Newly planned stages gain a `#healN` suffix
  and dependencies are renamed so the appended branch executes in order before
  returning to build/static-check/test.

### Interfaces & Contracts

- `WorkflowTicket` now serialises the `repo` object:

  ```json
  "repo": {
    "url": "https://gitlab.example/acme/demo.git",
    "base_ref": "main",
    "target_ref": "feature/mods-grid",
    "workspace_hint": "mods/java"
  }
  ```

- CLI flags/env: `--repo-url`, `--repo-base-ref`, `--repo-target-ref`, and
  `--repo-workspace-hint` surface workstation overrides;
  `PLOY_E2E_REPO_OVERRIDE` keeps tests hermetic.
- Planner API: `mods.NewPlanner(opts).Plan(ctx, mods.PlanInput{Ticket, Signals})`
  emits stage metadata with selected recipes, recommended playbooks, and
  max-parallel hints; healing passes the `Signals` map built from build-gate
  metadata.
- Stage metadata: `stage_metadata.mods.plan`, `.recommendations`, and `.human`
  are propagated to checkpoints and CLI summaries so operators can see branch
  provenance.

### Data Model & Persistence

- No persistent datastore additions. Workspace materialisation uses ephemeral
  directories under the CLI run root.
- Knowledge Base catalog remains file-backed; healing depends on the new
  ESLint/Error Prone incident entries merged in previous slices.

### Failure Modes & Recovery

- Git clone failures bubble up immediately with `git <cmd>` output; runs
  terminate before scheduling Mods stages so operators can fix credentials
  locally.
- Healing is single-shot per build-gate failure (`MaxStageRetries` guard);
  planners returning no stages or reusing blank lanes short-circuit without
  enqueueing empty branches.
- Non-retryable build-gate outcomes skip healing entirely, preserving the
  legacy behaviour for static analysis failures flagged as terminal.

## Dependencies & Interactions

- Requires lane registry updates (temporary in Ploy) and knowledge base catalog
  entries.
- Aligns with Workflow RPC job submission; no Grid server change required.
- Downstream Grid catalog registration depends on these lanes being stabilised
  in Ploy first.

## Risks & Mitigations

| Risk | Impact | Mitigation |
| --- | --- | --- |
| Git materialisation timeouts | Workflow delays | Use shallow clone with configurable timeout and surface metrics |
| Planner branching explosion | Scheduler overload | Limit branch count (e.g. ≤3) and collapse redundant options |
| Knowledge base drift | Healing ineffective | Add validation tests for tracked build gate codes |

## Observability & Telemetry

- Checkpoint payloads now include `stage_metadata.mods.*` for both initial and
  healing branches, exposing selected recipes, summaries, and knowledge-base
  recommendations.
- Healing stages are suffixed (`#heal1`) and recorded through the existing
  `StageInvocation` reporter so CLI summaries and downstream tooling can
  correlate retries.
- Repo metadata is echoed in claimed ticket checkpoints (URL/base/target),
  enabling audit logs without storing credentials.
- Follow-up: add structured metrics for workspace preparation time and healing
  fan-out once Grid telemetry ingestion resumes.

## Test Strategy

- Unit: `internal/workflow/runner/runner_mods_test.go` now exercises healing
  (`TestRunSchedulesHealingPlanAfterBuildGateFailure`) and ensures branch stages
  enqueue with suffixed names; planner and metadata conversion tests cover
  recipe propagation.
- CLI: `cmd/ploy/mod_run_mods_test.go` asserts repo flags populate tickets,
  workspace preparer execution, and Mods lane wiring.
- Integration: `cmd/ploy/test_support_test.go` keeps the in-memory Grid stub
  verifying job specs; additional workspace error cases are covered via stubbed
  git commands.
- E2E: `tests/e2e` now runs workstation smoke tests against the in-memory Grid
  stub; upgrading to real Grid fixtures is tracked in
  `docs/tasks/mods-grid/05-refactor.md`.

## Rollout Plan

1. [x] Land RED tasks and document failing tests using `ploy mod run` as the
   entry point (`docs/tasks/mods-grid/01-red.md`, `02-green.md`).
2. [x] Implement materialisation + Mods job specs; verify simple scenario via
   unit tests and CLI harness (`TestModRunWorkspaceMaterialization`).
3. [x] Add healing coverage and implement planner/runner feedback loop
   (`TestRunSchedulesHealingPlanAfterBuildGateFailure`).
4. [ ] Update docs/env references for workstation consumers, register the Mods
   catalog with Grid (publishing `/lanes/mods.tar.gz`), and replace RED e2e
   guards with runnable smoke tests (`docs/tasks/mods-grid/05-refactor.md`).

## Open Questions

- Should workspace hints allow selecting non-default lanes (e.g. Gradle vs Maven)? (TBD)
- How to expose branch selection metrics to Grid dashboards? (future)

## Follow-Up Work (2025-10-07)

- [ ] Planned –
  [shift-mods-grid-05 Grid Catalog Registration](../../tasks/mods-grid/05-refactor.md)
  *(placeholder to be created once GREEN completes)*
- [ ] Planned – Publish Mods catalog repository and update consumers to
  reference the Grid namespace.

## References

- `tests/e2e/scenarios.go`
- `internal/workflow/runner/runner_mods_test.go`
- `docs/design/workflow-rpc-alignment/README.md`
- `docs/design/mods/README.md`
