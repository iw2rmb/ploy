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
- [x] Switch `nodes.id` and related foreign keys from UUID to NanoID(6) strings — Use compact node identifiers across DB and APIs.
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

- [x] Update node agent, bootstrap, and server handlers to use NanoID(6) node identifiers end-to-end — Align `Config.NodeID`, `PLOYD_NODE_ID`, TLS CN, and URL paths with the new format.
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

## KSUID/NanoID docs, CLI, and OpenAPI alignment
- [x] Update CLI help and README text to treat IDs as opaque KSUID/NanoID strings — Remove UUID-specific wording from CLI surfaces.
  - Repository: github.com/iw2rmb/ploy
  - Component: cmd/ploy/upload_command.go, cmd/ploy/mod_run_repo.go, cmd/ploy/README.md
  - Scope:
    - Replace `UUID` wording in flag help and usage for `--run-id` / `--build-id` in `cmd/ploy/upload_command.go` with neutral descriptions (e.g., "Run identifier", "Build identifier") that do not specify UUID shape.
    - In `cmd/ploy/mod_run_repo.go`, change examples and flag descriptions from `--repo-id <repo-uuid>` / "Repo UUID" to `--repo-id <repo-id>` and describe repo IDs as short string identifiers (NanoID(8) semantics).
    - In `cmd/ploy/README.md`, update examples and narrative text:
      - Replace `ploy upload --run-id <uuid> [--build-id <uuid>] ...` with wording that uses `<run-id>` / `<build-id>` and explicitly notes these are KSUID-backed strings.
      - Replace `--repo-id <repo-uuid>` placeholders with `--repo-id <repo-id>` and mention that repo IDs are NanoID(8) strings.
  - Tests:
    - Run `go test ./cmd/ploy/...` and fix any assertions that expect `uuid` / `repo-uuid` strings in usage output.

- [x] Align OpenAPI component schemas with KSUID/NanoID and UUID split — Ensure schema types match the actual DB and handler behavior.
  - Repository: github.com/iw2rmb/ploy
  - Component: docs/api/components/schemas/controlplane.yaml
  - Scope:
    - Update `RunRepo` schema so:
      - `id` is documented as a NanoID(8) string (no `format: uuid`; describe length/character set instead).
      - `run_id` is documented as a KSUID-backed string (27 characters), consistent with `runs.id`.
    - Update `Repo` schema to treat `id` as an opaque string or remove the field if no concrete backing ID exists in the control plane (avoid `format: uuid` unless there is a UUID column).
    - For run/job/build IDs in component schemas (`Run`, `RunSummary`, `RunStatus`, `Stage`, `RunTiming`, `Log`, `NodeClaimResponse`, etc.):
      - Ensure `run_id` / `id` / `job_id` / `build_id` are plain `string` fields with descriptions like "Run ID (KSUID string)" / "Job ID (KSUID string)" / "Build ID (KSUID string)" and do not use `format: uuid`.
    - For `Diff` and `ArtifactBundle` schemas:
      - Keep `id` as `format: uuid` (diff/artifact IDs remain UUID).
      - Change `run_id`, `job_id`, `build_id` fields to plain strings with KSUID descriptions, matching `internal/store/schema.sql`.
    - For node-related schemas (`Node`, `NodeClaimResponse` and any others with `node_id`):
      - Document `node_id` as a NanoID(6) string (no `format: uuid`; describe length and URL-safe alphabet).
    - For Build Gate schemas (`BuildGateValidateResponse`, `BuildGateJobStatusResponse`, `NodeBuildGateClaimResponse`):
      - Align `job_id` documentation with actual behavior for `buildgate_jobs.id` (currently UUID-backed); either:
        - Document `job_id` as a UUID string, or
        - If `buildgate_jobs.id` is later migrated to KSUID, update the schema and handlers together and reflect that here.
  - Tests:
    - Run `go test ./docs/api/...` (notably `docs/api/verify_openapi_test.go`) and fix any failures caused by schema shape changes.

- [ ] Align OpenAPI path parameter formats and error descriptions with KSUID/NanoID identifiers — Remove UUID-specific validation language from paths that now accept opaque string IDs.
  - Repository: github.com/iw2rmb/ploy
  - Component: docs/api/paths/*.yaml
  - Scope:
    - Mods endpoints (`mods.yaml`, `mods_id.yaml`, `mods_id_cancel.yaml`, `mods_id_graph.yaml`, `mods_id_events.yaml`, `mods_id_logs.yaml`, `mods_id_diffs.yaml`, `mods_id_artifact_bundles.yaml`):
      - Change path parameter `id` from `format: uuid` to plain `string` and update descriptions to "Run ID (KSUID string)".
      - Update 400 error descriptions from "invalid UUID" to wording that matches current validation (e.g., "missing or empty run id").
    - Runs/batch endpoints (`runs_id.yaml`, `runs_id_stop.yaml`, `runs_id_repos.yaml`, `runs_id_repos_repo_id.yaml`, `runs_id_repos_repo_id_restart.yaml`):
      - For `id` path parameters, drop `format: uuid`, describe as KSUID string.
      - For `repo_id` path parameters, drop `format: uuid`, describe as "Repo ID (NanoID 8-character string)".
    - Node endpoints (`nodes_id_heartbeat.yaml`, `nodes_id_logs.yaml`, `nodes_id_events.yaml`, `nodes_id_complete.yaml`, `nodes_id_ack.yaml`, `nodes_id_buildgate_claim.yaml`, `nodes_id_buildgate_jobid_ack.yaml`, `nodes_id_buildgate_jobid_complete.yaml`):
      - Change path parameter `id` from `format: uuid` to plain `string` described as NanoID(6).
      - Update error descriptions that mention "invalid run/job UUID" or "invalid node UUID" to neutral messages (e.g., "node not found", "id path parameter is required").
    - Job-scoped artifact/diff endpoints (`runs_run_id_jobs_job_id_artifact.yaml`, `runs_run_id_jobs_job_id_diff.yaml`):
      - Treat `run_id` / `job_id` path parameters as plain strings described as KSUID strings (no `format: uuid`).
      - Keep only the diff/artifact IDs as `format: uuid` in response schemas.
    - Artifact and Diff fetch endpoints (`artifacts_id.yaml`, `diffs_id.yaml`):
      - Keep path param `id` as `format: uuid` (artifact/diff UUID).
      - Update response fields `run_id`, `job_id`, `build_id` to plain `string` with KSUID descriptions, and adjust any "Run ID (UUID)" wording accordingly.
    - Job-completion endpoint (`jobs_job_id_complete.yaml`):
      - Change `job_id` path parameter from `format: uuid` to plain `string` described as "Job ID (KSUID string)".
  - Tests:
    - Re-run `go test ./docs/api/...` to ensure all path references and schemas are still valid and `TestOpenAPICompleteness` continues to pass.

- [ ] Update markdown docs to describe KSUID/NanoID IDs instead of UUIDs — Ensure narrative docs match the new ID model.
  - Repository: github.com/iw2rmb/ploy
  - Component: docs/mods-lifecycle.md, docs/envs/README.md, docs/how-to/*
  - Scope:
    - In `docs/mods-lifecycle.md`:
      - Replace references to "run UUID" / "job UUID" with "run ID (KSUID string)" / "job ID (KSUID string)".
      - Update descriptions of `TicketSummary.stages` to say map keys are job IDs (KSUID strings), not UUIDs.
      - Update examples for `mod run repo` to use `--repo-id <repo-id>` and mention NanoID(8) semantics.
    - In `docs/envs/README.md` and `docs/how-to/create-mr.md`:
      - Replace "database run UUID" wording in branch derivation with "run ID (KSUID string)" and explain that `/mod/<run-id>` uses the KSUID run identifier.
    - In `docs/how-to/update-a-cluster.md` and any other how-to docs that show `--repo-id <repo-uuid>`:
      - Change placeholders to `--repo-id <repo-id>` and, where helpful, note that repo IDs are short NanoID(8) strings.
  - Tests:
    - Run `rg "UUID" docs` and ensure remaining UUID mentions are limited to:
      - IDs that are intentionally still UUIDs (`diffs.id`, `artifact_bundles.id`, `api_tokens.id`, `bootstrap_tokens.id`, `buildgate_jobs.id`), or
      - Historical notes that explicitly call out the prior UUID-based design.

- [ ] Clean up in-code comments that still describe run/job/repo IDs as UUIDs — Keep comments consistent with the KSUID/NanoID implementation.
  - Repository: github.com/iw2rmb/ploy
  - Component: internal/mods/api/types.go, docs/api/paths/*.yaml (inline descriptions), any handlers mentioning "job UUID"/"run UUID" in comments.
  - Scope:
    - In `internal/mods/api/types.go`, update comments for `RunSummary` and `StageStatus`:
      - Replace "ticket run_id — run UUID" and "Stages is keyed by job UUID" with descriptions using "run ID (KSUID string)" and "job ID (KSUID string)".
    - In server handlers and docs where comments still say "job UUID"/"run UUID":
      - Normalize wording to "job ID"/"run ID" and, where helpful, note KSUID/NanoID where IDs are generated by helpers.
  - Tests:
    - Run `go test ./internal/mods/... ./internal/server/...` to ensure no behavior changes were introduced and comments remain in sync with the code.
