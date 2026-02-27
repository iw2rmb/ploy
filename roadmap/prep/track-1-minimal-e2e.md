# Prep Track 1: Minimal E2E

Scope: Implement the first end-to-end prep flow for newly registered repos: detect prep work, execute one non-interactive prep attempt, validate and persist profile output, and gate normal run execution on `PrepReady` while keeping the current jobs pipeline.

Documentation: `design/prep.md`; `design/prep-impl.md`; `design/prep-simple.md`; `design/prep-states.md`; `design/prep-prompt.md`; `docs/schemas/prep_profile.schema.json`; `internal/store/schema.sql`; `internal/store/migrations.go`; `internal/store/queries/mig_repos.sql`; `internal/store/queries/run_repos.sql`; `internal/server/handlers/migs_repos.go`; `internal/server/handlers/runs_submit.go`; `internal/server/handlers/runs_batch_http.go`; `internal/server/handlers/migs_runs.go`; `internal/store/batchscheduler/batch_scheduler.go`; `cmd/ployd/server.go`; `internal/server/config/types.go`; `internal/server/config/defaults.go`; `internal/server/handlers/nodes_claim.go`; `internal/nodeagent/manifest.go`; `internal/nodeagent/execution_orchestrator_gate.go`; `internal/workflow/contracts/build_gate_config.go`; `internal/workflow/contracts/mods_spec_parse.go`; `internal/workflow/step/gate_docker_stack_gate.go`; `docs/build-gate/README.md`; `docs/api/OpenAPI.yaml`.

Legend: [ ] todo, [x] done.

## Phase 1: Data Model and State Primitives
- [x] Add repo-level prep state and profile storage in the DB schema.
  - Repository: `ploy`
  - Component: `internal/store`
  - Scope:
    - Extend `mig_repos` in `internal/store/schema.sql` with prep lifecycle fields:
      - `prep_status` (`PrepPending|PrepRunning|PrepRetryScheduled|PrepReady|PrepFailed`)
      - `prep_attempts` (int), `prep_last_error`, `prep_failure_code`, `prep_updated_at`
      - `prep_profile` (`JSONB`) and `prep_artifacts` (JSONB refs or dedicated FK)
    - Add `prep_runs` table for attempt-level evidence (`repo_id`, `attempt`, `status`, `started_at`, `finished_at`, `result_json`, `logs_ref`).
    - Bump `SchemaVersion` in `internal/store/migrations.go`.
    - Regenerate sqlc outputs after query additions.
  - Snippets:
    ```sql
    -- Current mig_repos baseline (internal/store/schema.sql)
    CREATE TABLE IF NOT EXISTS mig_repos (
      id TEXT PRIMARY KEY,
      mig_id TEXT NOT NULL REFERENCES migs(id) ON DELETE CASCADE,
      repo_url TEXT NOT NULL,
      base_ref TEXT NOT NULL,
      target_ref TEXT NOT NULL,
      created_at TIMESTAMPTZ NOT NULL DEFAULT now()
    );
    ```
    ```go
    // internal/store/migrations.go
    const SchemaVersion int64 = 2026022601 // increment after schema.sql changes
    ```
  - Tests:
    - `go test ./internal/store -run 'TestMigrate|TestStore'`
    - `go test ./internal/store -run 'TestSQLCOverridesCompile'`

- [x] Add prep query surface for claim/transition/persistence.
  - Repository: `ploy`
  - Component: `internal/store/queries`
  - Scope:
    - Add queries to `internal/store/queries/mig_repos.sql` and new `internal/store/queries/prep_runs.sql`:
      - `ListReposByPrepStatus`
      - `ClaimNextPrepRepo` (atomic status `PrepPending -> PrepRunning`)
      - `UpdateMigRepoPrepState`
      - `SaveMigRepoPrepProfile`
      - `CreatePrepRun` / `FinishPrepRun`
    - Ensure `store.Store` interface gains these methods through sqlc generation.
  - Snippets:
    ```sql
    -- Existing style for deterministic queue reads (internal/store/queries/run_repos.sql)
    SELECT ... FROM run_repos
    WHERE run_id = $1 AND status = 'Queued'
    ORDER BY created_at ASC, repo_id ASC;
    ```
    ```sql
    -- Prep claim should follow the same deterministic pattern + SKIP LOCKED.
    ```
  - Tests:
    - `go test ./internal/store -run 'TestClaimJob|TestRunRepos|TestMigRepos'`

## Phase 2: Prep Orchestrator Runtime
- [x] Implement a prep task (`scheduler.Task`) with retry/state transitions.
  - Repository: `ploy`
  - Component: `internal/server` + `cmd/ployd`
  - Scope:
    - Add new prep task package (e.g. `internal/server/prep`), modeled after:
      - `internal/store/batchscheduler/batch_scheduler.go`
      - `internal/server/recovery/stale_job_recovery_task.go`
    - Wire task in `cmd/ployd/server.go` scheduler registration.
    - Add scheduler config knobs in:
      - `internal/server/config/types.go`
      - `internal/server/config/defaults.go`
      - validation/loading tests in `internal/server/config/*_test.go`
  - Snippets:
    ```go
    // Existing task wiring (cmd/ployd/server.go)
    sched := scheduler.New()
    if batchSched != nil { sched.AddTask(batchSched) }
    if staleRecoveryTask != nil { sched.AddTask(staleRecoveryTask) }
    ```
    ```go
    // Add prep task with explicit interval (0 disables)
    // if prepTask != nil { sched.AddTask(prepTask) }
    ```
  - Tests:
    - `go test ./internal/server/config`
    - `go test ./internal/server/prep -run 'TestNew|TestRunCycle'`
    - `go test ./cmd/ployd -run 'Test.*server'`

- [x] Implement non-interactive prep execution and schema validation.
  - Repository: `ploy`
  - Component: `internal/server/prep`
  - Scope:
    - Add a runner interface (command executor abstraction) and a concrete non-interactive runner that:
      - clones/uses repo workspace
      - executes prep prompt + tactics (`design/prep-prompt.md`)
      - captures attempts/log refs
    - Validate produced JSON against `docs/schemas/prep_profile.schema.json`.
    - Persist `prep_runs` attempt rows and repo-level profile atomically on success.
  - Snippets:
    ```json
    // Contract to validate
    {
      "schema_version": 1,
      "repo_id": "repo_123",
      "runner_mode": "simple",
      "targets": { "build": {"status":"passed","command":"...","env":{}} }
    }
    ```
    ```go
    // Keep store JSON validation pattern consistent (internal/store/store.go)
    // withJSONB("...", raw, fn)
    ```
  - Tests:
    - `go test ./internal/server/prep -run 'TestRunner|TestSchemaValidation|TestStateTransitions'`

## Phase 3: Trigger and Gate Lifecycle
- [x] Trigger prep automatically when a repo is registered.
  - Repository: `ploy`
  - Component: `internal/server/handlers`
  - Scope:
    - On repo creation paths, initialize prep state as pending:
      - `internal/server/handlers/migs_repos.go` (`addMigRepoHandler`)
      - `internal/server/handlers/runs_submit.go` (`createSingleRepoRunHandler`)
      - `internal/server/handlers/runs_batch_http.go` (`addRunRepoHandler`)
    - Ensure bulk repo upsert (`bulkUpsertMigReposHandler`) sets prep state for inserted repos and invalidates stale prep profile when refs/url change.
  - Snippets:
    ```go
    // Existing repo creation call site
    repo, err := st.CreateMigRepo(ctx, store.CreateMigRepoParams{ ... })
    ```
    ```go
    // Immediately follow with prep state initialization:
    // st.UpdateMigRepoPrepState(... PrepPending ...)
    ```
  - Tests:
    - `go test ./internal/server/handlers -run 'TestAddModRepoHandler|TestBulkUpsertMigReposHandler|TestCreateSingleRepoRun'`

- [x] Gate run job materialization on `PrepReady`.
  - Repository: `ploy`
  - Component: `internal/store` + `internal/server/handlers`
  - Scope:
    - Enforce prep gate in scheduling path first (single source):
      - update queued repo selectors in `internal/store/queries/run_repos.sql` (join `mig_repos.prep_status='PrepReady'`)
      - keep `BatchRepoStarter.StartPendingRepos` unchanged semantically, but now naturally skips non-ready repos.
    - Remove/bypass immediate job creation on run submission endpoints so prep gate is authoritative:
      - `runs_submit.go`, `runs_batch_http.go`, `migs_runs.go` (stop direct `createJobsFromSpec` calls; let scheduler handle start).
  - Snippets:
    ```go
    // Current immediate start path (runs_submit.go)
    if err := createJobsFromSpec(...); err != nil { ... }
    ```
    ```sql
    -- Current queued repo query (run_repos.sql)
    WHERE run_id = $1 AND status = 'Queued'
    -- extend with prep-ready join condition
    ```
  - Tests:
    - `go test ./internal/server/handlers -run 'TestCreateSingleRepoRun|TestCreateMigRun|TestAddRunRepoHandler|TestBatchRepoStarter'`
    - `go test ./internal/store/batchscheduler`

## Phase 4: Use Prep Profile in Build Gate
- [x] Support spec-defined gate prep overrides (`build_gate.<phase>.prep`).
  - Repository: `ploy`
  - Component: `internal/workflow/contracts` + `internal/nodeagent` + `internal/workflow/step`
  - Scope:
    - Extend gate config contract with optional prep override per phase (pre/post) in:
      - `internal/workflow/contracts/build_gate_config.go`
      - `internal/workflow/contracts/mods_spec_parse.go`
      - `internal/nodeagent/run_options.go`
      - `internal/nodeagent/execution_orchestrator_gate.go`
    - Apply override in gate planner before fallback `buildCommandForTool`:
      - `internal/workflow/step/gate_docker_stack_gate.go`
    - Keep fallback behavior unchanged when no prep override exists.
  - Snippets:
    ```go
    // Current command resolution
    cmd, err := buildCommandForTool(tool)
    ```
    ```go
    // Target resolution order
    // 1) prep profile command/env override
    // 2) existing buildCommandForTool(tool)
    ```
  - Tests:
    - `go test ./internal/workflow/contracts -run 'TestParseModsSpecJSON_BuildGate'`
    - `go test ./internal/nodeagent -run 'TestBuildGateManifest|TestExecuteGateJob'`
    - `go test ./internal/workflow/step -run 'TestGateDocker'`

- [ ] Wire persisted repo `prep_profile` into gate planning (simple mode completion).
  - Repository: `ploy`
  - Component: `internal/server/handlers` + `internal/workflow/contracts` + `internal/nodeagent` + `internal/workflow/step`
  - Scope:
    - Add typed prep-profile decoding and validation helper for server-side claim path:
      - `internal/workflow/contracts` (shared profile type/parser) or `internal/server/prep` (adapter wrapper).
    - Inject repo-level prep overrides during claim response build in:
      - `internal/server/handlers/nodes_claim.go`
      - merge `mig_repos.prep_profile` into claimed spec before node execution.
    - Define deterministic simple-profile target mapping for gate phases:
      - `pre_gate` -> `targets.build`
      - `post_gate` and `re_gate` -> `targets.unit`
      - only inject when mapped target status is `passed` and command is non-empty.
    - Preserve precedence in command/env resolution:
      - `build_gate.<phase>.prep` already present in submitted spec wins
      - otherwise use mapped repo `prep_profile` target
      - fallback remains `buildCommandForTool(tool)`.
    - Keep env precedence unchanged in gate execution:
      - base gate env from spec/env injection first
      - mapped prep env overrides conflicting keys.
    - Update docs to reflect runtime source precedence and phase mapping:
      - `docs/build-gate/README.md`
      - `docs/migs-lifecycle.md`
  - Snippets:
    ```go
    // Claim-time merge point (nodes_claim.go)
    // mergedSpec := mergeJobIDIntoSpec(spec, job.ID)
    // mergedSpec = mergeRepoPrepProfileIntoSpec(mergedSpec, modRepo.PrepProfile, job.JobType)
    ```
    ```go
    // Runtime command resolution order
    // 1) explicit spec build_gate.<phase>.prep
    // 2) repo prep_profile mapped target
    // 3) buildCommandForTool(tool)
    ```
  - Tests:
    - `go test ./internal/server/handlers -run 'TestClaimJob.*PrepProfile'`
    - `go test ./internal/workflow/contracts -run 'TestPrepProfile.*(Parse|MapToGate)'`
    - `go test ./internal/nodeagent -run 'TestApplyGatePhaseOverrides'`
    - `go test ./internal/workflow/step -run 'TestResolveGateCommand|TestGateDocker'`
    - `go test ./docs/api -run TestOpenAPICompleteness`

## Phase 5: API Surface, Observability, and Docs
- [x] Expose prep status and evidence in repo-facing APIs and OpenAPI.
  - Repository: `ploy`
  - Component: `internal/server/handlers` + `docs/api`
  - Scope:
    - Add prep fields to repo responses (`GET /v1/repos`, `GET /v1/repos/{repo_id}/runs`) and/or add dedicated endpoint (`GET /v1/repos/{repo_id}/prep`).
    - Register route in `internal/server/handlers/register.go`.
    - Update OpenAPI files:
      - `docs/api/OpenAPI.yaml`
      - `docs/api/paths/repos*.yaml` (or new prep path file)
      - `docs/api/verify_openapi_test.go` expected endpoints/schemas
    - Add operational docs for prep behavior:
      - `docs/migs-lifecycle.md`
      - `docs/build-gate/README.md` (profile override precedence)
  - Snippets:
    ```go
    // Existing repo summary shape (repos.go)
    type RepoSummary struct {
      RepoID domaintypes.MigRepoID `json:"repo_id"`
      RepoURL string               `json:"repo_url"`
    }
    ```
    ```go
    // Extend with prep metadata:
    // PrepStatus, PrepUpdatedAt, PrepFailureCode
    ```
  - Tests:
    - `go test ./internal/server/handlers -run 'TestListRepos|TestListRunsForRepo|TestRegisterRoutesMatchesOpenAPI'`
    - `go test ./docs/api -run TestOpenAPICompleteness`

## Phase 6: End-to-End Validation (Track 1 Exit Criteria)
- [x] Add E2E coverage for `PrepPending -> PrepRunning -> PrepReady` and failure path.
  - Repository: `ploy`
  - Component: `tests` + focused package tests
  - Scope:
    - Add one happy-path test with a synthetic simple profile (build+unit pass, all_tests optional fail).
    - Add one failure-path test asserting `PrepFailed` stores failure code + evidence.
    - Validate run gating: jobs are not created/executed before `PrepReady`.
  - Snippets:
    ```text
    Expected state flow:
    PrepPending -> PrepRunning -> PrepReady
    or
    PrepPending -> PrepRunning -> PrepFailed
    ```
  - Tests:
    - `go test ./internal/server/prep ./internal/server/handlers ./internal/store/...`
    - `make test`
    - `make vet`
    - `make staticcheck`

## Open Questions
- For Track 1, persist prep logs as dedicated `prep_runs.logs_ref` records (recommended) or reuse existing run/job artifact tables via synthetic run IDs.
- For `createSingleRepoRunHandler`, confirm product behavior while repo is not prep-ready:
  - create run immediately but hold scheduling, or
  - return conflict until prep completes.
