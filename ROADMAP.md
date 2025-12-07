# KSUID/NanoID identifiers for runs, builds, jobs, repos, and nodes

Scope: Replace UUID-based identifiers for runs, jobs, builds, run-repos, and nodes with KSUID/NanoID-backed string IDs. Keep `events.id` unchanged. Update DB schema, store layer, server handlers, nodeagent, CLI, OpenAPI, and docs so IDs are generated via helpers and treated as opaque strings on the wire. Per AGENTS.md (“Backward Compatibility and Deprecation Policy”), do not add fallbacks, do not keep UUID-based code paths, and do not design or document runtime data migration flows; assume fresh deploys in the lab environment when IDs change.

Documentation: ../auto/ROADMAP.md, internal/domain/types/ids.go, internal/domain/types/uuid.go, internal/store/schema.sql, internal/store/models.go, internal/store/*.sql.go, internal/server/handlers/*.go, internal/server/events/service.go, internal/nodeagent/heartbeat.go, internal/nodeagent/logstreamer.go, internal/deploy/bootstrap_helpers.go, internal/deploy/bootstrap.go, internal/deploy/bootstrap_script.go, docs/api/OpenAPI.yaml, docs/api/components/schemas/controlplane.yaml, docs/envs/README.md, cmd/ploy/README.md.

Legend: [ ] todo, [x] done.

## Identifier helpers and libraries
- [x] Add KSUID/NanoID dependencies and central ID helpers — Provide explicit constructors for KSUID- and NanoID-based IDs so call sites do not embed library calls.
  - Repository: github.com/iw2rmb/ploy
  - Component: go.mod, go.sum, internal/domain/types
  - Scope:
    - Add `github.com/segmentio/ksuid` and a NanoID library (e.g. `github.com/matoous/go-nanoid/v2`) to `go.mod`.
    - Introduce a new file `internal/domain/types/idgen.go` with helpers:
      - `func NewRunID() RunID` returning `RunID(ksuid.New().String())`.
      - `func NewJobID() JobID` returning `JobID(ksuid.New().String())`.
      - `func NewBuildID() string` returning a KSUID string for `builds.id` until a dedicated `BuildID` type exists.
      - `func NewRunRepoID() RunRepoID` returning an 8-character NanoID using a fixed URL-safe alphabet.
      - `func NewNodeKey() string` returning a 6-character NanoID for node identifiers (used for `nodes.id` and node agent config).
    - Keep existing `RunID`, `JobID`, `RunRepoID`, and `NodeID` types in `internal/domain/types/ids.go` as simple string newtypes; rely on helper functions for ID generation rather than embedding KSUID/NanoID parsing in the types.
  - Snippets:
    - `func NewRunID() RunID { return RunID(ksuid.New().String()) }`
    - `func NewRunRepoID() RunRepoID { id, _ := gonanoid.Generate(alphabet, 8); return RunRepoID(id) }`
  - Tests:
    - Extend `internal/domain/types/ids_test.go` to verify the new helpers:
      - Non-empty output and expected length for `NewRunID`, `NewRunRepoID`, and `NewNodeKey`.
      - Multiple calls produce different values over a small sample (probabilistic uniqueness sanity check).

## KSUID for run-id / job-id / build-id — schema and store
- [x] Migrate run, job, and build IDs (and their FKs) from UUID to text KSUIDs in the database and store layer — Make IDs KSUID-backed strings end-to-end instead of Postgres UUIDs.
  - Repository: github.com/iw2rmb/ploy
  - Component: internal/store/schema.sql, internal/store/queries/*.sql, internal/store/*.sql.go, internal/store/models.go, sqlc.yaml
  - Scope:
    - In `internal/store/schema.sql`, change primary keys:
      - `runs.id`, `jobs.id`, `builds.id` from `UUID PRIMARY KEY DEFAULT gen_random_uuid()` to `TEXT PRIMARY KEY` (or `VARCHAR(27)` if you want stricter length checks).
    - Update all `run_id`, `job_id`, and `build_id` foreign keys that reference these tables (`jobs.run_id`, `events.run_id`, `events.job_id`, `builds.run_id`, `builds.job_id`, `diffs.run_id`, `diffs.job_id`, `logs.run_id`, `logs.job_id`, `logs.build_id`, `artifact_bundles.run_id`, `artifact_bundles.job_id`, `artifact_bundles.build_id`, `run_repos.execution_run_id`) from `UUID` to `TEXT` with the same nullability and referential constraints.
    - Keep `events.id`, `logs.id`, `artifact_bundles.id`, and other unrelated primary keys unchanged.
    - Update `internal/store/queries/*.sql` to remove `::uuid` casts for run/job/build IDs; treat them as text parameters.
    - Regenerate store code (via `sqlc`) so Go structs in `internal/store/*.sql.go` use `string` (or `pgtype.Text`) for run/job/build IDs and their foreign keys instead of `pgtype.UUID`.
    - Adjust `internal/store/models.go` types for `Run`, `Job`, `Build`, and related models to use `string` for ID fields matching the new schema.
  - Snippets:
    - Example schema change for runs in `internal/store/schema.sql`:
      - `id UUID PRIMARY KEY DEFAULT gen_random_uuid()` → `id TEXT PRIMARY KEY`
  - Tests:
    - Run `go test ./internal/store/...`:
      - Fix compilation errors from type changes (e.g., code that expects `pgtype.UUID`).
      - Update tests in `internal/store/store_test.go`, `internal/store/claims_state_test.go`, and `internal/store/batchscheduler/*.go` that construct `pgtype.UUID` run/job/build IDs to use string IDs instead.

## KSUID for run-id / job-id / build-id — server, nodeagent, CLI, OpenAPI
- [x] Treat run, job, and build IDs as KSUID-backed strings in application code — Remove UUID parsing from request paths and payloads; align OpenAPI formats.
  - Repository: github.com/iw2rmb/ploy
  - Component: internal/server/handlers, internal/server/events, internal/nodeagent, cmd/ploy, docs/api
  - Scope:
    - Server handlers:
      - Replace `uuid.Parse` / `domaintypes.ToPGUUID` for run/job/build IDs with direct string usage and optional KSUID validation:
        - Examples include `internal/server/handlers/events.go` (mods events SSE), `internal/server/handlers/jobs_complete.go`, `internal/server/handlers/runs_logs.go`, `internal/server/handlers/runs_artifacts.go`, `internal/server/handlers/nodes_logs.go`, and batch run handlers in `internal/server/handlers/runs_batch_http.go`.
      - Use `types.NewRunID()` and `types.NewJobID()` when creating new runs and jobs, and pass these strings into store insert params.
      - For `builds.id`, call `types.NewBuildID()` when creating build rows.
    - Events service:
      - In `internal/server/events/service.go`, simplify or remove `uuidToString`; use the string run ID from store models directly as the SSE stream key.
    - Nodeagent:
      - Ensure `internal/nodeagent/logstreamer.go` and `internal/nodeagent/execution_*` use string run/job IDs without calling `uuid.Parse` for these fields. Compose URLs using the string IDs directly.
    - CLI:
      - Update `cmd/ploy` flags and usage messages that hard-code `UUID` wording:
        - Example: `cmd/ploy/upload_command.go` (`--run-id` and `--build-id`) and `cmd/ploy/README.md` examples (`ploy upload --run-id <uuid> ...`).
      - Replace client-side UUID validation with len/charset checks for KSUID where validation is needed, or treat IDs as opaque strings.
    - OpenAPI:
      - In `docs/api/components/schemas/controlplane.yaml`, change `format: uuid` to a custom `format: ksuid` or omit `format` for all `run_id`, `job_id`, and `build_id` fields (e.g., `RunSummary`, `RunStatus`, log/event payloads).
      - Regenerate or update `docs/api/OpenAPI.yaml` to keep component and path references consistent.
  - Snippets:
    - Path parsing pattern for run IDs:
      - `runID := strings.TrimSpace(r.PathValue("id")); if runID == "" { http.Error(w, "id path parameter is required", http.StatusBadRequest); return }`
  - Tests:
    - Update tests under `internal/server/handlers` and `internal/server/events` that assert error messages containing `"invalid uuid"` to match new validation messages.
    - Run `go test ./internal/server/... ./internal/nodeagent/... ./cmd/ploy/...` and fix failing assertions about ID formats or JSON output.
    - Run `go test ./docs/api/...` to ensure `docs/api/verify_openapi_test.go` passes with updated formats.

## NanoID(8) for repo-id — run_repos.id
- [x] Convert `run_repos.id` and its uses to NanoID(8)-backed `RunRepoID` strings — Make per-repo IDs compact and human-friendly.
  - Repository: github.com/iw2rmb/ploy
  - Component: internal/store/schema.sql, internal/store/run_repos.sql.go, internal/store/models.go, internal/server/handlers/runs_batch_http.go, internal/server/handlers/runs_batch_types.go
  - Scope:
    - Schema:
      - In `internal/store/schema.sql`, change `run_repos.id` from `UUID PRIMARY KEY DEFAULT gen_random_uuid()` to `TEXT PRIMARY KEY` (or `VARCHAR(8)`), with no default. Application code will supply the NanoID via `NewRunRepoID()`.
    - Store:
      - Update `internal/store/run_repos.sql.go` so `RunRepo.ID` is a string (or `pgtype.Text`) matching the schema.
      - Ensure queries like `GetRunRepo`, `UpdateRunRepoStatus`, `IncrementRunRepoAttempt`, and `UpdateRunRepoRefs` accept string IDs.
    - Server handlers:
      - In `internal/server/handlers/runs_batch_http.go`:
        - For `deleteRunRepoHandler` and `restartRunRepoHandler`, replace UUID parsing of `repo_id` path parameters with raw string parsing: trim whitespace, reject empty, and pass the string into store calls.
        - When verifying that a repo belongs to a run, compare string `runRepo.RunID` against the string run ID (`runIDStr`) instead of converting via `uuid.UUID(...)`.
      - In `internal/server/handlers/runs_batch_types.go`, ensure `RunRepoResponse.ID` uses `domaintypes.RunRepoID` built from the string ID (no `uuid.UUID(rr.ID.Bytes)` conversions).
    - ID generation:
      - At `run_repos` creation sites (batch run setup in `runs_batch_http.go`), call `types.NewRunRepoID()` and pass the returned string into store insert params.
  - Snippets:
    - Path parsing pattern for repo IDs:
      - `repoID := strings.TrimSpace(r.PathValue("repo_id")); if repoID == "" { http.Error(w, "repo_id path parameter is required", http.StatusBadRequest); return }`
  - Tests:
    - Update `internal/server/handlers/runs_batch_http_test.go` to stop constructing `uuid.New()` for `sampleRepoID` and instead use deterministic short IDs (e.g. `"abcd1234"`) that match NanoID(8) length.
    - Run `go test ./internal/server/handlers/... ./internal/store/...` and fix any compilation errors or failing assertions related to repo IDs.

## NanoID(6) for node-id — DB nodes.id and node agent identifiers
- [ ] Switch `nodes.id` and related foreign keys from UUID to NanoID(6) strings — Use compact node identifiers across DB and APIs.
  - Repository: github.com/iw2rmb/ploy
  - Component: internal/store/schema.sql, internal/store/nodes.sql.go, internal/store/buildgate_jobs.sql.go, internal/store/jobs.sql.go, internal/store/runs.sql.go, internal/store/tokens.sql.go, internal/store/node_metrics.sql.go, internal/store/models.go
  - Scope:
    - Schema:
      - In `internal/store/schema.sql`, change `nodes.id` from `UUID PRIMARY KEY DEFAULT gen_random_uuid()` to `TEXT PRIMARY KEY` (or `VARCHAR(6)`).
      - Update foreign keys referencing nodes:
        - `runs.node_id`, `jobs.node_id`, `node_metrics.node_id`, `tokens.node_id`, and `buildgate_jobs.node_id` from `UUID` to `TEXT` with the same nullability and `REFERENCES nodes(id)` constraints.
      - Keep uniqueness constraints on `nodes.name` and `nodes.ip_address` unchanged.
    - Store:
      - Regenerate store code so `Node.ID` and all `NodeID` fields in Go structs are strings matching the schema.
  - Snippets:
    - Example schema change for nodes in `internal/store/schema.sql`:
      - `id UUID PRIMARY KEY DEFAULT gen_random_uuid()` → `id TEXT PRIMARY KEY`
  - Tests:
    - Run `go test ./internal/store/...` and fix any tests (`internal/store/claims_state_test.go`, `internal/store/ttlworker/...`) that constructed node IDs as `pgtype.UUID`.

- [ ] Update node agent, bootstrap, and server handlers to use NanoID(6) node identifiers end-to-end — Align `Config.NodeID`, `PLOYD_NODE_ID`, TLS CN, and URL paths with the new format.
  - Repository: github.com/iw2rmb/ploy
  - Component: internal/deploy/bootstrap_helpers.go, internal/deploy/bootstrap_helpers_test.go, internal/deploy/bootstrap.go, internal/deploy/bootstrap_script.go, internal/deploy/ca_crypto.go, internal/deploy/provision_test.go, internal/nodeagent/heartbeat.go, internal/nodeagent/logstreamer.go, internal/server/handlers/nodes_ack.go, internal/server/handlers/nodes_logs.go, internal/server/handlers/server_runs_complete.go, docs
  - Scope:
    - Bootstrap helpers:
      - Change `GenerateNodeID` in `internal/deploy/bootstrap_helpers.go` to use `NewNodeKey()` (NanoID(6)) and return the NanoID-based node identifier (with or without a `node-` prefix; choose and document).
      - Update `internal/deploy/bootstrap_helpers_test.go` expectations:
        - New total length and prefix rules.
        - Character set matches the chosen NanoID alphabet.
    - Bootstrap script and env:
      - Ensure `internal/deploy/bootstrap.go` passes the NanoID-based `nodeID` both as `--node-id` and `PLOYD_NODE_ID` (sanitized).
      - In `internal/deploy/bootstrap_script.go`, keep templating `node_id: ${NODE_ID:-}` but make sure docs and examples describe the new NanoID format.
      - Update any OpenSSL CN examples in docs (e.g. `docs/how-to/bearer-token-troubleshooting.md`) from UUID-style `node:$NODE_ID` to the new shorter ID.
    - Node agent:
      - In `internal/nodeagent/heartbeat.go`, continue using `cfg.NodeID` as a string; no UUID parsing is required.
      - In `internal/nodeagent/logstreamer.go`, remove the `uuid.Parse(ls.cfg.NodeID)` call in `sendChunk` and use `ls.cfg.NodeID` directly when constructing `/v1/nodes/{node_id}/logs` URLs.
    - Server handlers:
      - In `internal/server/handlers/nodes_ack.go`, `internal/server/handlers/nodes_logs.go`, and other node-centric handlers, replace `domaintypes.ToPGUUID(nodeIDStr)` with direct string validation (trim, non-empty) and string-based store calls.
      - Ensure store methods such as `GetNode`, `UpdateNodeHeartbeat`, and any job/node association helpers accept string node IDs.
    - OpenAPI and docs:
      - In `docs/api/components/schemas/controlplane.yaml`, change `node_id` fields from `format: uuid` to a custom `format: nanoid` or drop the `format` and describe the length (6) in the field description.
      - Update `docs/envs/README.md` to describe `PLOYD_NODE_ID` and `NODE_ID` as short NanoID strings rather than UUIDs.
  - Snippets:
    - HTTP path parsing for node IDs:
      - `nodeID := strings.TrimSpace(r.PathValue("id")); if nodeID == "" { http.Error(w, "id path parameter is required", http.StatusBadRequest); return }`
  - Tests:
    - Update tests that assume UUID-shaped node IDs:
      - `internal/deploy/provision_and_workstation_test.go`, `internal/worker/lifecycle/cache_test.go`, `internal/server/handlers/worker_logs_test.go`, `internal/stream/hub_test.go`, and any others asserting full UUID node IDs.
    - Run `go test ./internal/deploy/... ./internal/nodeagent/... ./internal/server/handlers/...` and fix failing assertions and URL expectations.
