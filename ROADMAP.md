# Mods Batch Runs (`mod run` over multiple repos)

Scope: Allow a single Mods spec (`mod.yaml`) to be submitted once and applied to multiple repositories as a named batch, with per-repo and batch-level status, and repo-level operations (add/remove/restart/stop) while keeping the scenario (spec) independent from the repo set.

Documentation: docs/mods-lifecycle.md, cmd/ploy/README.md, docs/how-to/deploy-a-cluster.md, docs/how-to/update-a-cluster.md, docs/how-to/create-mr.md, docs/how-to/cancel-endpoint-rollout.md, docs/envs/README.md, docs/api/OpenAPI.yaml, docs/api/paths/mods_id_*.yaml.

Legend: [ ] todo, [x] done.

## Data model and types
- [x] Introduce RunRepo schema for batched runs — reuse `runs` as batch metadata and move per-repo execution state into a separate table.
  - Repository: ploy
  - Component: internal/store
  - Scope: Extend `internal/store/schema.sql` with:
    - New enum:
      - `CREATE TYPE run_repo_status AS ENUM ('pending', 'running', 'succeeded', 'failed', 'skipped', 'cancelled');` (mirrors `job_status` without `created`).
    - New table:
      - `run_repos` with `id UUID PRIMARY KEY DEFAULT gen_random_uuid(), run_id UUID NOT NULL REFERENCES runs(id) ON DELETE CASCADE, repo_url TEXT NOT NULL, base_ref TEXT NOT NULL, target_ref TEXT NOT NULL, status run_repo_status NOT NULL DEFAULT 'pending', attempt INTEGER NOT NULL DEFAULT 1 CHECK (attempt >= 1), last_error TEXT, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), started_at TIMESTAMPTZ, finished_at TIMESTAMPTZ`.
    - Indexes: `CREATE INDEX run_repos_run_idx ON run_repos(run_id);` and `CREATE INDEX run_repos_status_idx ON run_repos(status) WHERE status IN ('pending','running');`.
  - Snippets:
    - `CREATE TYPE run_repo_status AS ENUM ('pending','running','succeeded','failed','skipped','cancelled');`
    - `CREATE TABLE IF NOT EXISTS run_repos (... run_id UUID NOT NULL REFERENCES runs(id) ON DELETE CASCADE, repo_url TEXT NOT NULL, ...);`
  - Tests: `go test ./internal/store -run TestRunMigrations` — schema applies cleanly and includes the new enum/table; extend tests in `internal/store/store_test.go` to assert `run_repos` exists (simple SELECT) after migrations.

- [x] Introduce optional batch naming on runs — let callers name a batch while reusing `runs` as the canonical Mods run record.
  - Repository: ploy
  - Component: internal/store
  - Scope:
    - Extend `runs` table in `internal/store/schema.sql` with an optional `name` column:
      - `name TEXT` (nullable) to store a human-readable batch name when present.
    - Update `internal/store/models.go` `Run` struct to include `Name *string 'json:"name"'`.
    - Update `internal/store/queries/runs.sql` SELECT/INSERT statements to include the new column, then regenerate `internal/store/runs.sql.go` via sqlc so `CreateRun`, `GetRun`, and `ListRuns` read/write `name`.
  - Snippets:
    - `ALTER TABLE runs ADD COLUMN name TEXT;`
    - `type Run struct { ID pgtype.UUID; Name *string; RepoUrl string; Spec []byte; ... }`.
  - Tests: Extend `internal/store/store_test.go` to create runs with and without a name and verify round-trip; run `go test ./internal/store/...`.

- [x] Add RunRepo Go model and status type — ensure repo-level statuses are strongly typed while reusing `Run` and `RunStatus`.
  - Repository: ploy
  - Component: internal/store
  - Scope:
    - In `internal/store/models.go`, define:
      - `type RunRepoStatus string` with constants `RunRepoStatusPending`, `RunRepoStatusRunning`, `RunRepoStatusSucceeded`, `RunRepoStatusFailed`, `RunRepoStatusSkipped`, `RunRepoStatusCancelled`.
      - `type RunRepo struct { ID, RunID pgtype.UUID; RepoUrl, BaseRef, TargetRef string; Status RunRepoStatus; Attempt int32; LastError *string; CreatedAt, StartedAt, FinishedAt pgtype.Timestamptz }`.
    - Implement database integration for `RunRepoStatus` if needed (Scan/Value methods) mirroring `RunStatus`.
  - Snippets:
    - `type RunRepoStatus string` and `const ( RunRepoStatusPending RunRepoStatus = "pending" ... )`.
  - Tests: Add tests in a new `internal/store/run_repos_status_test.go` (or extend `status_conversion_test.go`) validating that only the defined constants are accepted when scanning from the database.

- [x] Wire store queries for RunRepo — provide minimal CRUD and state-transition helpers around the existing `runs` table.
  - Repository: ploy
  - Component: internal/store
  - Scope:
    - Add `internal/store/queries/run_repos.sql` with sqlc queries:
      - `CreateRunRepo`, `ListRunReposByRun`, `UpdateRunRepoStatus`, and a helper to aggregate counts by status for a given `run_id`.
    - Run sqlc to generate `internal/store/run_repos.sql.go` matching the new `RunRepo` model.
    - Extend `internal/store/store.go` interface with methods exposing these operations and update the concrete store in `internal/store/store_test.go` to satisfy the interface.
  - Snippets:
    - Example query: `-- name: CreateRunRepo :one INSERT INTO run_repos (run_id,repo_url,base_ref,target_ref,status) VALUES ($1,$2,$3,$4,'pending') RETURNING *;`.
  - Tests: Add unit tests in `internal/store/store_test.go` that create a `Run`, attach a `RunRepo`, transition repo status from `pending` → `running` → `succeeded`, and assert aggregates match expectations.

## Server API for batched mod runs
- [x] Add Run HTTP handlers for batch lifecycle — list, inspect, and stop batched runs on top of `runs`.
  - Repository: ploy
  - Component: internal/server/handlers, internal/server/http
  - Scope:
    - Introduce `internal/server/handlers/handlers_runs_batch.go` with handlers:
      - `GET /v1/runs` — lists run summaries (including name, status, per-repo counts from `run_repos`).
      - `GET /v1/runs/{id}` — returns a detailed run summary (batch-level status plus per-repo aggregate counts).
      - `POST /v1/runs/{id}/stop` — marks the run as cancelling/cancelled (via existing run-status helpers) and cancels all `pending` `run_repos`.
    - Use `context.Context` from the request, reusing error handling style from `handlers_mods_ticket.go`.
    - Register routes in `internal/server/handlers/register.go` with `auth.RoleControlPlane`, reusing the existing `/v1/runs` prefix instead of introducing `/v1/mod_runs`.
  - Snippets:
    - Route registration: `s.HandleFunc("GET /v1/runs", listRunsHandler(st), auth.RoleControlPlane)`.
  - Tests: Create `internal/server/handlers/handlers_runs_batch_test.go` with table-driven tests validating 200 responses, validation errors for bad IDs, and proper status codes for unknown runs.

- [x] Add RunRepo HTTP handlers — manage repos within a batch (add/remove/restart/list) under `/v1/runs/{id}/repos`.
  - Repository: ploy
  - Component: internal/server/handlers
  - Scope:
    - In `handlers_runs_batch.go` (or a sibling file), implement:
      - `POST /v1/runs/{id}/repos` — body `{repo_url, base_ref, target_ref}` (optional `branch` or derived metadata later) and creates a `run_repos` row with `status=pending`.
      - `GET /v1/runs/{id}/repos` — returns the list of repos with fields: repo URL, base_ref, target_ref, attempt, status, last_error, started_at, finished_at.
      - `DELETE /v1/runs/{id}/repos/{repo_id}` — for `pending` repos, mark status as `skipped`; for `running`, mark as `cancelled` once in-flight execution stops.
      - `POST /v1/runs/{id}/repos/{repo_id}/restart` — resets the repo status to `pending`, increments `attempt`, and optionally updates branch/base/target refs from request body.
    - Reuse `internal/domain/types` (`types.RepoURL`, `types.GitRef`) for repo and ref validation, mirroring `submitTicketHandler`.
  - Snippets:
    - Request struct: `type runRepoRequest struct { RepoURL types.RepoURL 'json:"repo_url"'; BaseRef types.GitRef 'json:"base_ref"'; TargetRef types.GitRef 'json:"target_ref"' }`.
  - Tests: Add handler tests in `handlers_runs_batch_test.go` that:
    - Reject invalid repo URLs/refs with 400.
    - Return 404 for unknown run IDs.
    - Correctly move repo status from `pending` to `skipped` on DELETE and from terminal states back to `pending` on restart.

- [x] Connect RunRepo entries to execution runs — map each repo entry to the existing `runs` jobs pipeline.
  - Repository: ploy
  - Component: internal/server/handlers, internal/store
  - Scope:
    - Extend RunRepo handlers or a dedicated orchestration helper to:
      - For existing single-repo flows, continue to use `/v1/mods` and the existing `runs` row (with `runs.repo_url/base_ref/target_ref`) while gradually backfilling a `run_repos` row for compatibility.
      - For new batched flows, treat the top-level `runs` row as the shared spec/batch metadata and use `run_repos` entries to drive execution per repo (each repo may map to its own `runs` row if multi-run is preferred, or share a single `runs` row with per-repo tracking — decide here based on scheduler design).
      - Ensure `createJobsFromSpec` is invoked for each execution run, preserving existing job semantics (pre-gate, mod, post-gate).
      - When a run completes, update the corresponding `RunRepo` status based on `RunStatus` (`succeeded`, `failed`, `canceled` → `succeeded`, `failed`, `cancelled`).
    - Keep existing `/v1/mods` behavior unchanged for single-repo tickets (submitTicketHandler stays valid) while the batch surface builds on `/v1/runs`.
  - Snippets:
    - Pseudo-code: `runRepo, _ := st.CreateRunRepo(ctx, store.CreateRunRepoParams{RunID: run.ID, RepoUrl: repo.RepoUrl, BaseRef: repo.BaseRef, TargetRef: repo.TargetRef})`.
  - Tests: Extend `handlers_runs_batch_test.go` to assert that starting a batch run with two repos updates `run_repos` statuses correctly when underlying runs complete.

## Batch lifecycle and scheduler
- [x] Implement simple batch run scheduler — process repos within a batch, without cross-batch FIFO.
  - Repository: ploy
  - Component: internal/server/handlers, worker/scheduler (if present)
  - Scope:
    - Introduce a small scheduler loop or reuse existing scheduler hooks to:
      - For each run with at least one `run_repos.status = 'pending'`, schedule execution according to the chosen mapping (reuse existing `runs` row or spawn per-repo `runs` entries) when the batch is started.
      - Update run status from `queued/assigned` → `running` when any repo is running; mark as `succeeded/failed/canceled` when all repos are in terminal states (`succeeded|failed|skipped|cancelled`).
    - For v1, ignore global FIFO between multiple runs and focus on correct per-batch behavior.
  - Snippets:
    - Loop sketch: `runs, _ := st.ListActiveRunsWithRepos(ctx); for each run { repos := st.ListPendingRunRepos(ctx, run.ID); ... }`.
  - Tests: Add unit tests around the scheduler helper (in a new package or file) to validate state transitions; run `go test ./internal/server/...`.

- [x] Derive batch-level status aggregates — expose repo counts and terminal state in Runs APIs.
  - Repository: ploy
  - Component: internal/server/handlers, internal/store
  - Scope:
    - Add a store helper (e.g., `ListRunRepoCountsByStatus`) that returns counts of repos by `RunRepoStatus` for a given run ID.
    - Update `GET /v1/runs` and `GET /v1/runs/{id}` handlers to include:
      - `total`, `pending`, `running`, `succeeded`, `failed`, `cancelled`, `skipped` counts.
      - A derived batch run state:
        - `running` if any repo is running.
        - `failed` if none running and at least one repo failed (and batch not cancelled).
        - `completed` if all repos are succeeded/skipped and batch not cancelled.
        - `cancelled` if the batch was explicitly stopped.
  - Snippets:
    - `type RunBatchSummary struct { ID string; Name string; Status store.RunStatus; Counts RunRepoCounts }`.
  - Tests: Extend `handlers_runs_batch_test.go` to cover combinations of repo statuses and verify the summary fields and derived batch run status.

## CLI surfaces for batched runs
- [x] Extend `mod run` CLI grammar to route `mod run repo` subcommands — support `repo add/remove/restart/status`.
  - Repository: ploy
  - Component: cmd/ploy
  - Scope:
    - Update `cmd/ploy/mod_command.go` and `cmd/ploy/mod_run.go` to:
      - Preserve existing `ploy mod run` behavior (single-repo path).
      - Add a new router in `handleModRun` for `args[0] == "repo"` that delegates to `handleModRunRepo(args[1:], stderr)`.
    - Add `cmd/ploy/mod_run_repo.go` with a `handleModRunRepo` function that dispatches:
      - `repo add`, `repo remove`, `repo restart`, `repo status`.
    - Keep naming consistent with the requirement: use `mod run repo <action>` instead of `mod run <action>-repo`.
    - Update `cmd/ploy/testdata/help_mod.txt` and `printModRunFlagsSummary` in `cmd/ploy/mod_command.go` to mention the new subcommands without breaking existing golden tests.
  - Snippets:
    - `case "run": if len(args) > 1 && args[1] == "repo" { return handleModRunRepo(args[2:], stderr) }`.
  - Tests: Add `cmd/ploy/mod_run_repo_test.go` to cover argument parsing and error messages (unknown action, missing batch ID, missing repo flags).

- [ ] Implement CLI client for batch run lifecycle — create/list/stop/status using control-plane APIs.
  - Repository: ploy
  - Component: internal/cli/mods, cmd/ploy
  - Scope:
    - Add `internal/cli/mods/batch.go` (or similar) with functions:
      - `CreateBatch(ctx, baseURL, httpClient, name, specPath, createdBy) (BatchSummary, error)`.
      - `ListBatches(ctx, baseURL, httpClient) ([]BatchSummary, error)`.
      - `GetBatchStatus(ctx, baseURL, httpClient, id) (BatchStatus, error)`.
      - `StopBatch(ctx, baseURL, httpClient, id) error`.
    - Reuse control-plane resolution helpers from `cmd/ploy/mod_run_exec.go` (`resolveControlPlaneHTTP`) and spec loading from `cmd/ploy/mod_run_spec.go`.
    - Wire `handleModRun` (for batch creation) and `handleModRunRepo` (for repo operations) to these helpers.
  - Snippets:
    - `type BatchSummary struct { ID, Name, State string; Counts RunRepoCounts }`.
  - Tests: Add `internal/cli/mods/batch_test.go` using `httptest.Server` to verify JSON request/response, error mapping, and that `BatchSummary` fields are set correctly.

- [ ] Implement `mod run repo` subcommands — add/remove/restart repos and display per-batch status.
  - Repository: ploy
  - Component: cmd/ploy, internal/cli/mods
  - Scope:
    - In `cmd/ploy/mod_run_repo.go`, define four subcommands:
      - `repo add <batch-id> --repo-url <url> --repo-base-ref <ref> --repo-target-ref <ref>` (plus optional flags like `--name` or `--attempt` later).
      - `repo remove <batch-id> --repo-url <url>` (or `--repo-id` once exposed by API).
      - `repo restart <batch-id> --repo-url <url> [--branch <ref>]`.
      - `repo status <batch-id>` — prints a table with columns: repo, base_ref, target_ref, attempt, status, last_error.
    - Parse flags using `flag.FlagSet` per subcommand, consistent with `cmd/ploy/mod_run_flags.go`.
    - Delegate HTTP calls to `internal/cli/mods` batch client functions.
  - Snippets:
    - Output line: `fmt.Fprintf(w, "%-40s %-16s %-16s %-4d %-10s %s\n", repo.RepoURL, repo.BaseRef, repo.TargetRef, repo.Attempt, repo.Status, repo.LastError)`.
  - Tests: Extend `cmd/ploy/mod_run_repo_test.go` to:
    - Validate flag parsing and required arguments.
    - Use a fake `internal/cli/mods` client (or httptest.Server) to assert correct endpoints are called and output formatting matches expectations.

## Documentation and OpenAPI
- [ ] Extend OpenAPI spec for batch run endpoints — document control-plane surfaces for runs and repos.
  - Repository: ploy
  - Component: docs/api
  - Scope:
    - Update `docs/api/OpenAPI.yaml` to add:
      - Paths for `/v1/runs` (list), `/v1/runs/{id}` (summary), `/v1/runs/{id}/stop`, `/v1/runs/{id}/repos`, `/v1/runs/{id}/repos/{repo_id}`, `/v1/runs/{id}/repos/{repo_id}/restart`.
    - Add a new path file (e.g., `docs/api/paths/runs_batch.yaml`) with detailed request/response schemas referencing new components, reusing existing run-related components where possible.
    - Introduce schema components for:
      - `RunBatchSummary`, `RunRepo`, `RunRepoStatus`, and `RunRepoCounts`.
    - Ensure reused types (repo_url, base_ref, target_ref) match existing definitions used by `/v1/mods`.
  - Snippets:
    - Example YAML: `RunBatchSummary: { type: object, properties: { id: {type: string, format: uuid}, name: {type: string}, status: { $ref: '#/components/schemas/RunStatus' }, counts: { $ref: '#/components/schemas/RunRepoCounts' } } }`.
  - Tests: Run `go test ./docs/api/...` (including `docs/api/verify_openapi_test.go`) to validate schema references and path registration.

- [ ] Add repo-centric API endpoints — list repos and show runs for a given repo.
  - Repository: ploy
  - Component: internal/server/handlers, internal/store, docs/api
  - Scope:
    - Implement `GET /v1/repos` to list known repositories:
      - Backed by `run_repos` (and optionally `runs.repo_url` for legacy rows) using `SELECT DISTINCT repo_url` with optional filters via query params (e.g. `?contains=org/`).
      - Response shape: array of `{repo_url, last_run_at?, last_status?}` to help users see active repos.
      - Add a small store helper (e.g., `ListRepos(ctx, filter)` in `internal/store`) or inline SQL in a handler-specific helper.
    - Implement `GET /v1/repos/{repo_id}/runs` to list runs for a given repo:
      - Use `repo_id` as URL-encoded `types.RepoURL.String()` (decode path segment and validate via `types.RepoURL`).
      - Query `run_repos` joined with `runs` to return `{run_id, name, status, base_ref, target_ref, attempt, started_at, finished_at}` ordered by time.
    - Register new routes in `internal/server/handlers/register.go` with `auth.RoleControlPlane`.
    - Document these endpoints in `docs/api/OpenAPI.yaml` and a new path file (e.g., `docs/api/paths/repos.yaml`) with schemas for `RepoSummary` and `RepoRunsResponse`.
  - Snippets:
    - Handler signature: `func listReposHandler(st store.Store) http.HandlerFunc { ... }`.
    - Path example: `GET /v1/repos/https%3A%2F%2Fgithub.com%2Forg%2Frepo.git/runs`.
  - Tests: Add `internal/server/handlers/handlers_repos_test.go` with table-driven tests covering:
    - Happy paths for listing repos and runs for a repo.
    - Validation failures for invalid `repo_id` (bad URL decode or invalid scheme).
    - Empty responses when no runs exist for a repo.

- [ ] Update mods lifecycle docs to cover batched runs — clarify relationship between runs, run_repos, and jobs.
  - Repository: ploy
  - Component: docs
  - Scope:
    - Extend `docs/mods-lifecycle.md`:
      - Add a new subsection (e.g., "Batched Mods runs (`runs` + `run_repos`)") describing:
        - How a run row stores spec and metadata, independent of repositories.
        - How run_repos entries attach repos and branches to a run.
        - How each run (or execution mapped from run_repos) has its own jobs pipeline.
        - The repo-level and batch-level state machines.
      - Cross-link from existing sections that describe `/v1/mods` and single-mod runs, indicating that a single-repo batch is a degenerate case with one run_repo.
    - Ensure diagrams or sequence descriptions (if any) mention `mod run repo add/remove/restart`.
  - Snippets:
    - Text example: "In a batch, `ploy mod run` submits the spec once, then `ploy mod run repo add` attaches multiple repositories under the same run via `run_repos`."
  - Tests: Manual review; ensure `docs/mods-lifecycle.md` still builds and is referenced from any docs index if present.

- [ ] Update CLI and how-to docs — show `mod run` vs `mod run repo` usage and batch workflows.
  - Repository: ploy
  - Component: cmd/ploy/README.md, docs/how-to
  - Scope:
    - In `cmd/ploy/README.md`, add a subsection "Batched mod runs" with examples:
      - Single repo: `ploy mod run --spec mod.yaml --repo-url https://github.com/example/repo.git --repo-base-ref main --repo-target-ref feature --follow`.
      - Batch: `ploy mod run --spec mod.yaml --name my-batch` followed by `ploy mod run repo add my-batch --repo-url ...` for multiple repos.
    - Update how-to guides that reference `ploy mod run`:
      - `docs/how-to/deploy-a-cluster.md`, `docs/how-to/update-a-cluster.md`, `docs/how-to/create-mr.md`, `docs/how-to/cancel-endpoint-rollout.md`, and `docs/envs/README.md` to:
        - Mention that `mod run` can operate as a batch over multiple repos.
        - Show at least one end-to-end example where a batch is created and a repo is restarted with a different branch.
  - Snippets:
    - Example: `ploy mod run repo restart my-batch --repo-url https://github.com/example/repo.git --branch hotfix`.
  - Tests: None automated; validate examples by running them against a dev cluster once implementation is in place.

## Type-safety and tests
- [ ] Tighten type system for batch run and RunRepo inputs/outputs — avoid raw strings for critical fields.
  - Repository: ploy
  - Component: internal/server/handlers, internal/domain/types
  - Scope:
    - Reuse existing domain types in run batch handlers:
      - `types.RepoURL` for `repo_url`.
      - `types.GitRef` for `base_ref` and `target_ref`.
      - `types.CommitSHA` for optional `commit_sha` if extended in future.
    - Introduce IDs or name types if helpful:
      - e.g., `type RunID string` or reusing `types.TicketID` semantics in `internal/domain/types/ids.go` for clearer logging and tracing.
    - Ensure JSON decoding uses these types directly so validation happens at boundary layers (similar to `submitTicketHandler` in `handlers_mods_ticket.go`).
  - Snippets:
    - `type runBatchCreateRequest struct { Name string 'json:"name"'; Spec *json.RawMessage 'json:"spec"'; CreatedBy *string 'json:"created_by,omitempty"' }`.
  - Tests: Add/extend tests in `internal/domain/types/vcs_test.go` or a new test file to validate that RepoURL/GitRef validation still passes existing cases and rejects invalid inputs in RunRepo handlers.

- [ ] Add focused tests for batch run workflow across server, CLI, and E2E — keep regression surface small.
  - Repository: ploy
  - Component: internal/server/handlers, cmd/ploy, tests/e2e/mods
  - Scope:
    - Server:
      - Extend `internal/server/handlers/handlers_runs_batch_test.go` to cover:
        - Happy path: list runs with and without `run_repos`, add two repos, start batch, mark underlying runs as succeeded, batch completes with expected summary.
        - Error paths: invalid repo URL, unknown run ID, restart on non-terminal repo.
    - CLI:
      - Add tests in `cmd/ploy/mod_run_repo_test.go` verifying:
        - Each subcommand (`add/remove/restart/status`) validates arguments.
        - The CLI calls the expected HTTP paths with correct payloads (using a test HTTP server).
    - E2E:
      - Add `tests/e2e/mods/scenario-batch-run/` with a `run.sh` that:
        - Uses `dist/ploy mod run --spec mod.yaml --name batch-1` to create a batch run.
        - Adds at least two repos with `mod run repo add`.
        - Restarts one repo with a different branch.
        - Stops the batch and asserts that final statuses are visible via `mod run repo status`.
  - Snippets:
    - Example E2E invocation: `dist/ploy mod run repo status "$BATCH_ID" --watch` (once watch support is added).
  - Tests: Run `make test` and `tests/e2e/mods/scenario-batch-run/run.sh` locally as part of RED→GREEN cycles; ensure coverage thresholds in `GOLANG.md` remain satisfied.
