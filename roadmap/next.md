# Replace `step_index` Scheduling With `next_id` Chains and Rename Job Fields

Scope: Implement job graph execution using explicit `next_id -> jobs.id` links instead of sortable/fractional `step_index`, and rename `ModType`/`ModImage` to `Type` (`JobType`) / `Image` (`JobImage`) across store, server, nodeagent, API, CLI output, tests, and docs. This lands before `mod -> mig` naming migration.

Documentation: `AGENTS.md`; `docs/mods-lifecycle.md`; `docs/build-gate/README.md`; `docs/api/OpenAPI.yaml`; `docs/api/components/schemas/controlplane.yaml`; `docs/testing-workflow.md`.

Legend: [ ] todo, [x] done.

## Phase 0: Contract and RED Gates
- [x] Define the new job field contract and enum mapping in design docs before code edits.
  - Repository: `ploy`
  - Component: Domain contract, naming glossary
  - Scope: Document canonical rename mapping: `ModType` -> `Type` (`JobType`), `ModImage` -> `Image` (`JobImage`). Define job type values as step phases (`pre_build`, `step`, `post_build`, `heal`, `re_build`, `mr`) and remove old `pre_gate/mod/post_gate/re_gate` names from the new contract.
  - Snippets: `internal/nodeagent/job.go`, `internal/workflow/contracts/job_meta.go`, `docs/api/components/schemas/controlplane.yaml`
  - Tests: Add/adjust guard checks in `tests/guards` that fail on newly introduced `mod_type`, `mod_image`, and `step_index` in current interfaces after migration.

- [x] Add RED tests for linked-list orchestration semantics before implementation.
  - Repository: `ploy`
  - Component: Server orchestration tests, nodeagent flow tests
  - Scope: Add failing tests for: linear execution via `next_id`, healing insertion rewiring, and claim order driven by chain edges (not numeric sort).
  - Snippets: `internal/server/handlers/jobs_complete_orchestration_test.go`, `internal/server/handlers/server_runs_claim_test.go`, `internal/nodeagent/execution_orchestrator*_test.go`
  - Tests: RED expected until schema, claim, and completion flows are migrated.

## Phase 1: Store Schema and Query Layer
- [x] Replace `jobs.step_index` with `jobs.next_id` in schema and generated queries.
  - Repository: `ploy`
  - Component: `internal/store/schema.sql`, query SQL, sqlc outputs
  - Scope: Drop `step_index` and related ordering/index constraints; add nullable `next_id` with `FOREIGN KEY (next_id) REFERENCES jobs(id)`, plus indexes for chain traversal and promotion.
  - Snippets: `internal/store/schema.sql`, `internal/store/queries/jobs.sql`, `internal/store/jobs.sql.go`
  - Tests: Store query tests for create/claim/promote behavior and FK validity.

- [x] Rename persisted job fields from `mod_type`/`mod_image` to `job_type`/`job_image` (DB) and `Type`/`Image` (Go).
  - Repository: `ploy`
  - Component: Store models/sqlc, domain structs
  - Scope: Rename columns and generated params/rows; replace all callsites using `ModType` and `ModImage` in store APIs; remove old fields.
  - Snippets: `internal/store/models.go`, `internal/store/queries/jobs.sql`, `internal/store/runs.sql.go`
  - Tests: Compile + store integration tests with new column names only.

## Phase 2: Server Scheduling and Dynamic Rewiring
- [x] Build initial run job chains using `next_id` pointers.
  - Repository: `ploy`
  - Component: Run submission and ticket creation handlers
  - Scope: When creating jobs (pre-build, step N, post-build), store the chain in `next_id` order and queue only chain head as claimable.
  - Snippets: `internal/server/handlers/mods_ticket.go`, `internal/server/handlers/runs_submit.go`
  - Tests: Run creation tests assert exact `next_id` linkage and single queued head job.

- [x] Replace midpoint insertion logic with transactional `next_id` rewiring for healing.
  - Repository: `ploy`
  - Component: Job completion/orchestration handlers
  - Scope: On gate/build failure requiring healing: read current job `next_id` (`old_next`), create healing step(s), then set `failed.next_id = heal.id` and `last_inserted.next_id = old_next` in one transaction.
  - Snippets: `internal/server/handlers/jobs_complete.go`, `internal/server/handlers/jobs_complete_logic.go`
  - Tests: Orchestration tests for pre-build and post-build healing insertion and recovery path.

- [x] Update claim logic to follow queue+link semantics instead of `step_index` ordering.
  - Repository: `ploy`
  - Component: Claim endpoints and scheduler
  - Scope: Claim from queued jobs only; when a job reaches terminal success state, promote `next_id` job to queued if present. Ensure cancellation/failure semantics still stop chain progression as defined.
  - Snippets: `internal/server/handlers/server_runs_claim_test.go`, `internal/server/handlers/runs_batch_scheduler.go`, `internal/store/queries/jobs.sql`
  - Tests: Claim ordering, cancellation, retries, and multi-repo fairness tests.

## Phase 3: Nodeagent and Workflow Contracts
- [x] Remove `StepIndex` dependence from execution and rehydration paths.
  - Repository: `ploy`
  - Component: Nodeagent orchestrator and diff fetch ordering
  - Scope: Stop deriving behavior from float indices; use claimed job identity and chain progression. Update rehydration ordering to use chain traversal / creation order contract as defined by server API.
  - Snippets: `internal/nodeagent/execution_orchestrator.go`, `internal/nodeagent/execution_orchestrator_rehydrate.go`, `internal/nodeagent/difffetcher.go`
  - Tests: Nodeagent orchestration tests and rehydration tests with inserted healing steps.

- [x] Rename runtime metadata fields from mod-prefixed to job-prefixed names.
  - Repository: `ploy`
  - Component: Contracts, SSE payloads, log record types
  - Scope: Replace `mod_type`/`mod_image` fields in job metadata, events, and log records with `job_type`/`job_image` (or `type`/`image` where schema requires). Remove old fields and converters.
  - Snippets: `internal/workflow/contracts/job_meta.go`, `internal/mods/api/types.go`, `internal/server/events/service.go`, `internal/cli/follow/engine.go`
  - Tests: Event parsing/stream tests and CLI follow/status tests.

## Phase 4: API, CLI Output, and Docs
- [x] Update OpenAPI and handler contracts to the new field names and chain model.
  - Repository: `ploy`
  - Component: API spec and handler responses
  - Scope: Replace `mod_type`, `mod_image`, and `step_index` in schemas/examples with new job fields and `next_id`/predecessor-successor semantics. Remove all step-index ordering language.
  - Snippets: `docs/api/OpenAPI.yaml`, `docs/api/components/schemas/controlplane.yaml`, `docs/api/verify_openapi_test.go`
  - Tests: `go test ./docs/api/...` and handler response assertions.

- [x] Rewrite lifecycle/build-gate docs to chain-based execution semantics.
  - Repository: `ploy`
  - Component: Runtime docs
  - Scope: Replace diagrams/text that describe fractional insertion with `next_id` rewiring examples, including the requested case: failed pre-build job gets `next_id` updated to healing step, and healing points to former successor.
  - Snippets: `docs/mods-lifecycle.md`, `docs/build-gate/README.md`
  - Tests: Link checks and doc-tested snippets if present.

## Phase 5: GREEN and REFACTOR
- [ ] Run full validation on the new model and remove transitional code.
  - Repository: `ploy`
  - Component: Whole repository
  - Scope: Execute `make test`, `make coverage`, `make vet`, `make staticcheck`, `make build`; delete temporary adapters, comments, or fallback paths created during migration.
  - Snippets: `make test && make coverage && make vet && make staticcheck && make build`
  - Tests: All suites green with no `step_index`/`mod_type`/`mod_image` usage in active interfaces.

- [ ] Run residue scans and enforce final invariants.
  - Repository: `ploy`
  - Component: Hygiene checks
  - Scope: Verify `next_id` is the only orchestration ordering primitive; verify all job metadata surfaces use new names.
  - Snippets: `rg -n 'step_index|mod_type|mod_image|ModType|ModImage' internal cmd docs tests`
  - Tests: Scan output must be empty or limited to explicitly approved historical notes/tests.
