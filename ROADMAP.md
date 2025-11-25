# Multi-node Mods Execution with Snapshot+Diff Rehydration

Scope: Enable Mods runs to execute gates and mods across multiple nodes and parallel “theories” by rehydrating each step into an isolated workspace from a shared git base state plus an ordered chain of per-step diffs instead of a single mutable workspace on one node.

Documentation: docs/schemas/mod.example.yaml, docs/how-to/deploy-a-cluster.md, tests/e2e/mods/README.md, internal/nodeagent/*.go, internal/server/handlers/*.go, internal/workflow/runtime/step/*.go, internal/worker/hydration/*.go.

Legend: [ ] todo, [x] done.

## Spec & Ticket Model
- [x] Extend Mod spec for multi-step mods (global build_gate/build_gate_healing, mods[]) — Define how multiple mods share one repo and global gate/heal policy
  - Component: ploy (CLI, docs)
  - Scope: docs/schemas/mod.example.yaml, tests/e2e/mods/scenario-multi-step/*; describe mods[] semantics (sequential steps, shared repo, global build_gate/build_gate_healing policy)
  - Test: go test ./cmd/ploy/... — Spec parsing tests cover mods[] and env_from_file; bash tests/e2e/mods/scenario-multi-step/run.sh — multi-step spec submission works end-to-end
- [x] Clarify ticket model for multi-step Mods runs — Ensure control plane treats a run as an ordered sequence of steps
  - Component: ploy (server, store)
  - Scope: internal/mods/api/types.go, internal/server/handlers/handlers_mods_ticket.go, internal/store/migrations/* (if step metadata needed), CHECKPOINT_MODS.md
  - Test: go test ./internal/server/... ./internal/store/... — New tests assert run creation and stage metadata support multi-step runs

## Diff Lifecycle & Storage
- [x] Introduce per-step diff lifecycle for Mods runs — Capture a diff after each gate+mod step instead of only once at the end
  - Component: ploy (nodeagent, server, store)
  - Scope: internal/nodeagent/execution_orchestrator.go (call uploadDiff per step), internal/nodeagent/execution_healing.go (decide where a “step” ends), internal/server/handlers/nodes_stage_diff.go, internal/store/queries/diffs.sql
  - Test: go test ./internal/nodeagent/... ./internal/server/... ./internal/store/... — Multiple diffs per run are created, ordered by created_at
- [x] Attach step identity to stored diffs — Allow rehydration to select “all diffs before step k”
  - Component: ploy (server, store)
  - Scope: internal/store/migrations/* (optional step_index/phase in diffs), internal/server/handlers/nodes_stage_diff.go, internal/server/handlers/handlers_diffs.go
  - Test: go test ./internal/server/... ./internal/store/... — ListRunDiffs exposes step metadata; ordering by step_index matches created_at

## Workspace Hydration & Rehydration
- [x] Define base hydration strategy using shallow clones — Use a shallow git clone of base_ref (or default branch) and optional commit_sha as the logical "base snapshot" on each node
  - Component: ploy (worker hydration, nodeagent)
  - Scope: internal/worker/hydration/git_fetcher.go, internal/nodeagent/execution.go, GOLANG.md (document shallow clone behaviour)
  - Test: go test ./internal/worker/hydration/... ./internal/nodeagent/... — Hydration shallow-clones base_ref and pins commit_sha when present
- [x] Add per-node base clone caching under PLOYD_CACHE_HOME — Avoid repeated full clones for the same run/repo on one node
  - Component: ploy (nodeagent, hydration)
  - Scope: internal/nodeagent/workspace.go, internal/nodeagent/execution.go, internal/nodeagent/buildgate_executor.go, internal/worker/hydration/git_fetcher.go (optional cache dir support), docs/envs/README.md (PLOYD_CACHE_HOME expectations)
  - Test: go test ./internal/nodeagent/... ./internal/worker/hydration/... — Second hydration for same repo/ref under PLOYD_CACHE_HOME reuses cache; cache failures fall back to fresh clone
- [x] Implement “rehydrate from base+diffs” helper for Mods steps — Build a fresh workspace by copying base clone and applying ordered diffs
  - Component: ploy (nodeagent, workflow runtime)
  - Scope: internal/nodeagent/execution.go (new rehydration helper), internal/workflow/runtime/step/hydrator_test.go, internal/workflow/runtime/step/stub.go (hydration path selection)
  - Test: go test ./internal/nodeagent/... ./internal/workflow/runtime/step/... — Given a base clone and N diffs, rehydrated workspace matches incremental edits

## Multi-step Execution & Multi-node Scheduling
- [x] Refactor Mods run execution into explicit steps (gates + mods) — Represent each gate+mod pair as a logical step with an index
  - Component: ploy (nodeagent)
  - Scope: internal/nodeagent/run_options.go (typed multi-step mods[] as Steps), internal/nodeagent/claimer_spec.go (preserve mods[] for nodeagent), internal/nodeagent/manifest.go (step-specific image/command/env), internal/nodeagent/execution_orchestrator.go (loop over steps and per-step logging)
  - Test: go test ./internal/nodeagent/... — Multi-step runs execute sequential steps with per-step indices; single-step runs still pass
- [x] Make gate/mod steps rehydratable on any node — Use rehydration helper instead of long-lived workspaces per run
  - Component: ploy (nodeagent)
  - Scope: internal/nodeagent/execution_orchestrator.go (create a fresh workspace per step), internal/nodeagent/execution_healing.go (healing uses rehydrated workspace), internal/nodeagent/workspace.go
  - Test: go test ./internal/nodeagent/... — Steps can be executed in isolation; parallel tests use different workspaces without interference
- [x] Allow scheduler to assign steps across nodes (same run) — Enable multiple nodes to execute distinct steps of one run using rehydration
  - Component: ploy (server, nodeagent)
  - Scope: internal/store/migrations/008_run_steps.sql (run_steps table for per-step status), internal/store/queries/run_steps.sql (ClaimRunStep and step status helpers), internal/server/handlers/nodes_claim.go (step-level claims before whole-run claims), internal/nodeagent/diffuploader.go (step_index tagging for diffs), internal/nodeagent/handlers.go and internal/nodeagent/claimer.go/claimer_loop.go (thread step_index into StartRunRequest), internal/nodeagent/execution_orchestrator.go (execute only claimed step and upload per-step diffs)
  - Test: go test ./internal/nodeagent/... ./internal/server/handlers/... — Nodes can claim specific steps of a multi-step run, execute only the claimed step with rehydrated workspace, and upload diffs tagged with step_index
  - Note: run_step_status transitions (AckRunStepStart / UpdateRunStepCompletion) are defined but not yet wired into status handlers; multi-node scheduling remains behind a feature flag until those updates and integration tests are added

## Scheduler Status Wiring & Run Semantics
- [x] Materialize run_steps rows for multi-step runs — Create one step record per mods[] entry when a run is queued
  - Component: ploy (server, store)
  - Scope: internal/server/handlers/handlers_mods_ticket.go (helper called from submitTicketHandler to invoke CreateRunStep once per mods[] index), internal/store/queries/run_steps.sql (CreateRunStep)
  - Test: go test ./internal/server/handlers/... — New tests assert CreateRunStep is invoked for multi-step specs (mods[] present) and skipped for single-step specs
- [ ] Restrict ClaimRun to runs without run_steps — Ensure multi-step runs are claimed via ClaimRunStep only
  - Component: ploy (store)
  - Scope: internal/store/queries/runs.sql (ClaimRun WHERE clause gains NOT EXISTS (SELECT 1 FROM run_steps WHERE run_id = runs.id)), regenerate internal/store/runs.sql.go via sqlc
  - Test: go test ./internal/store/... — New tests seed runs with and without run_steps rows and assert ClaimRun only returns runs that have no run_steps
- [ ] Wire AckRunStepStart through node ack endpoint — Transition run_steps.status from assigned→running for step claims
  - Component: ploy (server, nodeagent)
  - Scope: internal/server/handlers/nodes_ack.go (extend request payload with optional step_index and, when present, call GetRunStepByIndex and AckRunStepStart after validating node_id and run_id), internal/nodeagent/claimer_loop.go (include ClaimResponse.StepIndex in POST /v1/nodes/{id}/ack payload)
  - Test: go test ./internal/server/handlers/... ./internal/nodeagent/... — New tests assert AckRunStepStart is invoked for multi-step claims and that ack payloads include the expected step_index
- [ ] Wire UpdateRunStepCompletion through completion endpoint — Transition run_steps.status to terminal state on step completion
  - Component: ploy (server, nodeagent, store)
  - Scope: internal/server/handlers/nodes_complete.go (accept optional step_index, load run_step via GetRunStepByIndex, and call UpdateRunStepCompletion with mapped RunStepStatus and reason), internal/nodeagent/statusuploader.go and internal/nodeagent/execution_upload.go (thread optional step_index from executeRun into StatusUploader payload)
  - Test: go test ./internal/server/handlers/... ./internal/nodeagent/... ./internal/store/... — New tests assert run_step_status flows queued→assigned→running→succeeded/failed/canceled for multi-step runs
- [ ] Ensure per-step diffs are incremental and rehydration-safe — Make diff[0..k-1] replayable in order to reconstruct workspace[step_k]
  - Component: ploy (nodeagent, workflow runtime)
  - Scope: internal/nodeagent/execution_orchestrator.go (after rehydration for stepIndex>0, create a baseline git commit in the workspace before execution), internal/nodeagent/execution.go (helper that uses internal/nodeagent/git.EnsureCommit to write the baseline commit), internal/workflow/runtime/step/stub.go (continue to use git diff HEAD in filesystemDiffGenerator)
  - Test: go test ./internal/nodeagent/... ./internal/workflow/runtime/step/... — New tests in internal/nodeagent/execution_rehydrate_test.go verify that applying stored per-step diffs for steps 0..k-1 to a fresh base clone yields the expected workspace contents for step k
- [ ] Refine run-level start semantics for multi-step runs — Mark run as running when the first step starts, even when claimed via run_steps
  - Component: ploy (server, store)
  - Scope: internal/server/handlers/nodes_ack.go (when step_index is present and run has run_steps rows, relax the status precondition to allow queued→running transition, and call AckRunStart or a dedicated helper to set runs.status=running), internal/store/queries/runs.sql (reuse AckRunStart)
  - Test: go test ./internal/server/handlers/... ./internal/store/... — New tests assert that the first step ack moves run.status to running for multi-step runs, while subsequent step acks leave run.status unchanged
- [ ] Refine run-level completion semantics for multi-step runs — Derive terminal run status from run_steps instead of trusting caller status
  - Component: ploy (server, store)
  - Scope: internal/server/handlers/nodes_complete.go (for runs that have run_steps entries, compute the effective run terminal state using CountRunSteps and CountRunStepsByStatus instead of blindly trusting the status field in the request), internal/store/queries/run_steps.sql (CountRunSteps, CountRunStepsByStatus already present)
  - Test: go test ./internal/server/handlers/... ./internal/store/... — New tests assert that runs are marked succeeded only when all steps succeeded, failed when any step failed, and that inconsistent combinations of requested run status and run_steps state are rejected or normalized
- [ ] Add explicit tests for step-level claiming and single-step execution — Demonstrate end-to-end that nodes execute only the claimed step and upload step-indexed diffs
  - Component: ploy (server, nodeagent)
  - Scope: internal/server/handlers/nodes_claim.go (unit tests that simulate multi-step runs with run_steps rows and assert ClaimRunStep is used and step_index is present in claim responses), internal/nodeagent/agent_claim_test.go or equivalent (tests that ClaimManager maps ClaimResponse.StepIndex into StartRunRequest.StepIndex and executes a single step in executeRun when StepIndex is non-nil)
  - Test: go test ./internal/server/handlers/... ./internal/nodeagent/... — New tests show nodes can claim distinct steps of the same run, execute only the claimed step per node, and still support legacy single-step runs claimed via ClaimRun

## Diff Download & Apply Pipeline
- [x] Provide node-facing API to list and fetch run diffs — Let nodes pull gzipped patches and metadata per run
  - Component: ploy (server, nodeagent)
  - Scope: internal/server/handlers/handlers_diffs.go (reuse GET /v1/mods/{id}/diffs and GET /v1/diffs/{id}?download=true), internal/nodeagent/diffuploader.go (document symmetry), internal/nodeagent/difffetcher.go (node diff client helper)
  - Test: go test ./internal/server/... ./internal/nodeagent/... — Node can fetch and gunzip patches uploaded earlier by any node
- [x] Implement patch application in nodeagent using git/patch — Apply ordered run diffs onto a fresh base clone when rehydrating
  - Component: ploy (nodeagent)
  - Scope: internal/nodeagent/execution.go (apply patch chain), internal/nodeagent/difffetcher.go (fetch patches), internal/nodeagent/execution_orchestrator.go (wire into step hydration)
  - Test: go test ./internal/nodeagent/... ./internal/workflow/runtime/step/... — Given stored patches, workspace contents match expected code after each step

## CLI, Docs & E2E Coverage
- [x] Update CLI spec handling to preserve mods[] and new step metadata — Ensure buildSpecPayload and parseSpec handle multi-step fields without breaking single-mod flows
  - Component: ploy (CLI, nodeagent)
  - Scope: cmd/ploy/mod_run_spec.go, cmd/ploy/mod_run_spec_parsing_test.go, internal/nodeagent/claimer_spec.go
  - Test: go test ./cmd/ploy/... ./internal/nodeagent/... — Spec round-trips mods[] and step metadata; legacy single-mod specs still pass
- [x] Document multi-node Mods architecture and rehydration model — Explain base clone + diff chain semantics and scheduler behaviour
  - Component: ploy (docs)
  - Scope: docs/how-to/deploy-a-cluster.md, docs/how-to/publish-mods.md, CHECKPOINT_MODS.md, ROADMAP_NEXT.md (link to this roadmap)
  - Test: make lint-docs or manual review — Docs describe the new flow consistently with implementation
- [x] Add E2E scenarios for multi-step, multi-node Mods runs — Validate rehydration and MR content end-to-end
  - Component: ploy (tests)
  - Scope: tests/e2e/mods/scenario-multi-node-rehydration/* (new spec and run.sh), tests/e2e/mods/README.md (usage)
  - Test: bash tests/e2e/mods/scenario-multi-node-rehydration/run.sh — Scenario validates multi-step execution with rehydration, works on both single-node and multi-node clusters
