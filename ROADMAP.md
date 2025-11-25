# Multi-node Mods Execution with Snapshot+Diff Rehydration

Scope: Enable Mods runs to execute gates and mods across multiple nodes and parallel “theories” by rehydrating each step into an isolated workspace from a shared git base state plus an ordered chain of per-step diffs instead of a single mutable workspace on one node.

Documentation: docs/schemas/mod.example.yaml, docs/how-to/deploy-a-cluster.md, tests/e2e/mods/README.md, internal/nodeagent/*.go, internal/server/handlers/*.go, internal/workflow/runtime/step/*.go, internal/worker/hydration/*.go.

Legend: [ ] todo, [x] done.

## Spec & Ticket Model
- [ ] Extend Mod spec for multi-step mods (global build_gate/build_gate_healing, mods[]) — Define how multiple mods share one repo and global gate/heal policy
  - Component: ploy (CLI, docs)
  - Scope: docs/schemas/mod.example.yaml, tests/e2e/mods/scenario-*/mod.yaml, docs/how-to/publish-mods.md; describe mods[] semantics (sequential steps, global gate, shared repo)
  - Test: go test ./cmd/ploy/... ./tests/e2e/... — Spec parsing tests cover mods[], docs reference new fields
- [ ] Clarify ticket model for multi-step Mods runs — Ensure control plane treats a run as an ordered sequence of steps
  - Component: ploy (server, store)
  - Scope: internal/mods/api/types.go, internal/server/handlers/handlers_mods_ticket.go, internal/store/migrations/* (if step metadata needed), CHECKPOINT_MODS.md
  - Test: go test ./internal/server/... ./internal/store/... — New tests assert run creation and stage metadata support multi-step runs

## Diff Lifecycle & Storage
- [ ] Introduce per-step diff lifecycle for Mods runs — Capture a diff after each gate+mod step instead of only once at the end
  - Component: ploy (nodeagent, server, store)
  - Scope: internal/nodeagent/execution_orchestrator.go (call uploadDiff per step), internal/nodeagent/execution_healing.go (decide where a “step” ends), internal/server/handlers/nodes_stage_diff.go, internal/store/queries/diffs.sql
  - Test: go test ./internal/nodeagent/... ./internal/server/... ./internal/store/... — Multiple diffs per run are created, ordered by created_at
- [ ] Attach step identity to stored diffs — Allow rehydration to select “all diffs before step k”
  - Component: ploy (server, store)
  - Scope: internal/store/migrations/* (optional step_index/phase in diffs), internal/server/handlers/nodes_stage_diff.go, internal/server/handlers/handlers_diffs.go
  - Test: go test ./internal/server/... ./internal/store/... — ListRunDiffs exposes step metadata; ordering by step_index matches created_at

## Workspace Hydration & Rehydration
- [ ] Define base hydration strategy using shallow clones — Use git clone of base_ref/commit_sha as the logical “base snapshot” on each node
  - Component: ploy (worker hydration, nodeagent)
  - Scope: internal/worker/hydration/git_fetcher.go, internal/nodeagent/execution.go, GOLANG.md (document shallow clone behaviour)
  - Test: go test ./internal/worker/hydration/... ./internal/nodeagent/... — Hydration always clones the expected ref; respects commit_sha when present
- [ ] Add per-node base clone caching under PLOYD_CACHE_HOME — Avoid repeated full clones for the same run/repo on one node
  - Component: ploy (nodeagent, hydration)
  - Scope: internal/nodeagent/workspace.go, internal/worker/hydration/git_fetcher.go (optional cache dir support), docs/envs/README.md (PLOYD_CACHE_HOME expectations)
  - Test: go test ./internal/nodeagent/... ./internal/worker/hydration/... — Second hydration for same repo/ref under PLOYD_CACHE_HOME reuses cache
- [ ] Implement “rehydrate from base+diffs” helper for Mods steps — Build a fresh workspace by copying base clone and applying ordered diffs
  - Component: ploy (nodeagent, workflow runtime)
  - Scope: internal/nodeagent/execution.go (new rehydration helper), internal/workflow/runtime/step/hydrator_test.go, internal/workflow/runtime/step/stub.go (hydration path selection)
  - Test: go test ./internal/nodeagent/... ./internal/workflow/runtime/step/... — Given a base clone and N diffs, rehydrated workspace matches incremental edits

## Multi-step Execution & Multi-node Scheduling
- [ ] Refactor Mods run execution into explicit steps (gates + mods) — Represent each gate+mod pair as a logical step with an index
  - Component: ploy (server, nodeagent)
  - Scope: CHECKPOINT_MODS.md, internal/server/handlers/handlers_mods_ticket.go (step metadata), internal/nodeagent/execution_orchestrator.go (loop over steps instead of single manifest)
  - Test: go test ./internal/server/... ./internal/nodeagent/... — Execution logs and stats show per-step boundaries and indices
- [ ] Make gate/mod steps rehydratable on any node — Use rehydration helper instead of long-lived workspaces per run
  - Component: ploy (nodeagent)
  - Scope: internal/nodeagent/execution_orchestrator.go (create a fresh workspace per step), internal/nodeagent/execution_healing.go (healing uses rehydrated workspace), internal/nodeagent/workspace.go
  - Test: go test ./internal/nodeagent/... — Steps can be executed in isolation; parallel tests use different workspaces without interference
- [ ] Allow scheduler to assign steps across nodes (same run) — Enable multiple nodes to execute distinct steps of one run using rehydration
  - Component: ploy (server, nodeagent)
  - Scope: internal/server/handlers/nodes_claim.go (step-level claims), internal/nodeagent/claimer_loop.go (claim “step work” not just whole runs), internal/store/queries/runs.sql and stages.sql (if additional step rows needed)
  - Test: integration tests ./tests/integration/... — Two nodes claim different steps of the same run; both succeed and final MR includes all changes

## Diff Download & Apply Pipeline
- [ ] Provide node-facing API to list and fetch run diffs — Let nodes pull gzipped patches and metadata per run
  - Component: ploy (server, nodeagent)
  - Scope: internal/server/handlers/handlers_diffs.go (reuse GET /v1/mods/{id}/diffs and GET /v1/diffs/{id}?download=true), internal/nodeagent/diffuploader.go (document symmetry), internal/nodeagent/new diff client helper
  - Test: go test ./internal/server/... ./internal/nodeagent/... — Node can fetch and gunzip patches uploaded earlier by any node
- [ ] Implement patch application in nodeagent using git/patch — Apply ordered run diffs onto a fresh base clone when rehydrating
  - Component: ploy (nodeagent)
  - Scope: internal/nodeagent/execution.go (apply patch chain), internal/nodeagent/git/* (helper for git apply), internal/nodeagent/execution_orchestrator.go (wire into step hydration)
  - Test: go test ./internal/nodeagent/... — Given stored patches, workspace contents match expected code after each step

## CLI, Docs & E2E Coverage
- [ ] Update CLI spec handling to preserve mods[] and new step metadata — Ensure buildSpecPayload and parseSpec handle multi-step fields without breaking single-mod flows
  - Component: ploy (CLI, nodeagent)
  - Scope: cmd/ploy/mod_run_spec.go, cmd/ploy/mod_run_spec_parsing_test.go, internal/nodeagent/claimer_spec.go
  - Test: go test ./cmd/ploy/... ./internal/nodeagent/... — Spec round-trips mods[] and step metadata; legacy single-mod specs still pass
- [ ] Document multi-node Mods architecture and rehydration model — Explain base clone + diff chain semantics and scheduler behaviour
  - Component: ploy (docs)
  - Scope: docs/how-to/deploy-a-cluster.md, docs/how-to/publish-mods.md, CHECKPOINT_MODS.md, ROADMAP_NEXT.md (link to this roadmap)
  - Test: make lint-docs or manual review — Docs describe the new flow consistently with implementation
- [ ] Add E2E scenarios for multi-step, multi-node Mods runs — Validate rehydration and MR content end-to-end
  - Component: ploy (tests)
  - Scope: tests/e2e/mods/* (new multi-step specs and run.sh), tests/README.md, tests/e2e/mods/README.md (usage)
  - Test: bash tests/e2e/mods/<new-scenario>/run.sh — Scenario passes with steps on one node and with steps split across nodes (when lab is available)
