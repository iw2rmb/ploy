# Run ID field naming consistency (RunID vs ID on claim responses)

Scope: Standardize run identifier field names in nodeagent and claim-related server responses by renaming `ID` to `RunID` (while keeping the JSON wire format as `\"id\"`). This improves type clarity in the codebase, makes it explicit where a value is a run identifier, and aligns with the type system work around KSUID-backed IDs without changing the API contract.

Documentation: ../auto/ROADMAP.md, roadmap/ksuid.md, internal/nodeagent/claimer.go, internal/nodeagent/claimer_loop.go, internal/nodeagent/agent_claim_test.go, internal/server/handlers/nodes_claim.go, docs/api/components/schemas/controlplane.yaml (NodeClaimResponse), docs/api/OpenAPI.yaml, docs/api/paths/nodes_id_claim.yaml.

Legend: [ ] todo, [x] done.

## Nodeagent ClaimResponse field rename
- [x] Rename `ClaimResponse.ID` to `ClaimResponse.RunID` in nodeagent — Make the run identifier explicit at the type level without changing the JSON schema.
  - Repository: github.com/iw2rmb/ploy
  - Component: internal/nodeagent/claimer.go
  - Scope:
    - Change the struct definition in `internal/nodeagent/claimer.go` from:
      - `ID types.RunID \`json:\"id\"\` // Run ID`
      - to `RunID types.RunID \`json:\"id\"\` // Run ID`.
    - Ensure any comments above/beside the struct still clearly describe this field as the run ID for the claimed job.
  - Snippets:
    - `type ClaimResponse struct {`
    - `    RunID types.RunID \`json:\"id\"\` // Run ID`
    - `    JobID types.JobID \`json:\"job_id\"\` // Claimed job ID`
    - `    // ...`
    - `}`
  - Tests:
    - Run `go test ./internal/nodeagent/...` — All nodeagent tests compile and pass after the rename.

## Update nodeagent claim handling call sites
- [x] Replace uses of `claim.ID` with `claim.RunID` in the claim loop and related helpers — Keep behavior identical while improving readability.
  - Repository: github.com/iw2rmb/ploy
  - Component: internal/nodeagent/claimer_loop.go
  - Scope:
    - In the claim loop, update logging and function calls:
      - `slog.Info(\"claimed job\", \"run_id\", claim.ID, ...)` → `claim.RunID`.
      - `if err := c.ackRun(ctx, claim.ID.String(), claim.JobID.String());` → pass `claim.RunID.String()`.
    - In the derived target-ref defaulting logic, replace the `/mod/<run-id>` formatting input:
      - `targetRef = fmt.Sprintf(\"/mod/%s\", claim.ID)` → `fmt.Sprintf(\"/mod/%s\", claim.RunID)`.
    - In the `StartRunRequest` construction, assign:
      - `RunID: claim.RunID` instead of `RunID: claim.ID`.
  - Snippets:
    - `targetRef := strings.TrimSpace(claim.TargetRef)`
    - `if targetRef == \"\" {`
    - `    targetRef = fmt.Sprintf(\"/mod/%s\", claim.RunID)`
    - `}`
  - Tests:
    - Run `go test ./internal/nodeagent/...` — Verify `TestClaimLoop_MapsClaimToStartRunRequest` and related tests still pass and that logs/tests reference `RunID` consistently.

## Adjust nodeagent tests constructing ClaimResponse
- [x] Update ClaimResponse initializers in nodeagent tests to use `RunID` — Keep test data aligned with the new field name.
  - Repository: github.com/iw2rmb/ploy
  - Component: internal/nodeagent/agent_claim_test.go, internal/nodeagent/agent_test.go, internal/nodeagent/claimer_loop_test.go
  - Scope:
    - Replace struct literals that set `ID: types.RunID(\"...\")` with `RunID: types.RunID(\"...\")`.
    - Search for any direct references to `claim.ID` or `ClaimResponse{ID:` in tests and update them to `claim.RunID` / `ClaimResponse{RunID:`.
    - Keep JSON expectations unchanged (the serialized field name remains `\"id\"`), only adjust Go-side field names.
  - Snippets:
    - `claim := ClaimResponse{`
    - `    RunID: types.RunID(\"2NxO0FEXAMPLE4Rn\"),`
    - `    JobID: types.JobID(\"2NxO0FEXAMPLE4Jb\"),`
    - `    // ...`
    - `}`
  - Tests:
    - Run `go test ./internal/nodeagent/...` — Confirm no tests rely on the old `ID` identifier and that JSON round-trips still work as before.

## Server claim response struct symmetry (optional)
- [x] Rename the inline `ID` field to `RunID` in the server claim response struct — Align server-side naming with nodeagent while preserving the JSON schema.
  - Repository: github.com/iw2rmb/ploy
  - Component: internal/server/handlers/nodes_claim.go
  - Scope:
    - In `buildAndSendJobClaimResponse`, change the response struct field:
      - From `ID string \`json:\"id\"\` // Run ID`
      - To `RunID string \`json:\"id\"\` // Run ID`.
    - Adjust struct initialization to set `RunID: run.ID` instead of `ID: run.ID`.
    - Ensure logging or other call sites that deserialize the claim use the existing `NodeClaimResponse` schema; no changes to JSON field names are required.
  - Snippets:
    - `resp := struct {`
    - `    RunID string \`json:\"id\"\` // Run ID`
    - `    JobID string \`json:\"job_id\"\``
    - `    // ...`
    - `}{`
    - `    RunID: run.ID,`
    - `    JobID: job.ID,`
    - `    // ...`
    - `}`
  - Tests:
    - Run `go test ./internal/server/handlers -run Claim` — Verify claim handler tests still pass and response JSON remains shaped as `{\"id\": ..., \"job_id\": ...}`.

## Docs and schema validation check
- [x] Re-verify that docs and OpenAPI already describe `id` as Run ID — Confirm no further changes are needed after internal renames.
  - Repository: github.com/iw2rmb/ploy
  - Component: docs/api/components/schemas/controlplane.yaml, docs/api/OpenAPI.yaml, docs/api/paths/nodes_id_claim.yaml, docs/build-gate/README.md, docs/mods-lifecycle.md, cmd/ploy/README.md
  - Scope:
    - Confirm `NodeClaimResponse.id` is documented as “Run ID (parent run of the claimed job, KSUID string)” and does not need renaming, since the JSON field name stays `id`.
    - Scan docs for references to `claim.id` vs `claim.run_id` in narrative examples; update prose to use `run_id` terminology where appropriate while keeping JSON examples stable.
    - Ensure there is no mismatch between the Go field name (`RunID`) and the documented JSON key (`id`) in examples and path descriptions.
  - Snippets:
    - `NodeClaimResponse:` block in `docs/api/components/schemas/controlplane.yaml` showing `id` as the run identifier.
  - Tests:
    - Run `go test ./docs/api/...` (including `docs/api/verify_openapi_test.go`) — Expect no schema changes, only verification that documentation remains consistent with the existing wire format.

## Replace primitives with domaintypes IDs in control-plane and CLI
- [ ] Migrate control-plane and CLI structs from `string` IDs to `domaintypes` newtypes — Use `RunID`, `JobID`, `NodeID`, `ClusterID`, and `RunRepoID` instead of raw `string` where values are stable identifiers.
  - Repository: github.com/iw2rmb/ploy
  - Components: `internal/server`, `internal/cli`, `internal/deploy`, `internal/nodeagent`, `internal/worker`, `cmd/ploy`
  - Scope:
    - For run identifiers:
      - Replace `RunID string` fields in control-plane handlers and CLI types with `RunID domaintypes.RunID` where the field represents a Mods run identifier and the JSON tag is already `json:"run_id"` or equivalent.
      - Focus first on handwritten structs (handlers and CLI request/response types), not sqlc-generated store models.
      - Representative hotspots:
        - `internal/server/handlers/diffs.go` (`diffGetResponse.RunID`).
        - `internal/server/handlers/artifacts_download.go` response structs using `run_id`.
        - `internal/server/handlers/nodes_logs.go`, `internal/server/handlers/nodes_events.go` structs with `RunID string`.
        - `internal/cli/mods/{diffs.go,artifacts.go,resume.go,cancel.go,inspect.go,logs.go,events.go}` where request/response types expose `run_id`.
        - `cmd/ploy/mod_run_exec.go`, `cmd/ploy/mod_run_repo.go`, `cmd/ploy/mod_run_batch_test.go` helper structs that wrap run identifiers in `string`.
      - Keep wire format unchanged by preserving existing JSON tags:
        - Example: `RunID domaintypes.RunID \`json:"run_id"\`` in place of `RunID string \`json:"run_id"\``.
    - For job identifiers:
      - Replace `JobID string` fields in control-plane and CLI types with `JobID domaintypes.JobID` where the field identifies a job and the JSON tag is `json:"job_id"`.
      - Focus areas:
        - `internal/server/handlers/diffs.go` (`diffItem.JobID`).
        - `internal/server/events/service.go` payload structs carrying job IDs.
        - `internal/server/handlers/nodes_claim.go` claim response and request shapes with `job_id`.
        - `internal/cli/runs/{inspect.go,follow.go}`, `internal/cli/mods/diffs.go`, `internal/cli/logs/printer.go`, and `internal/cli/transfer/client.go` where IDs are treated as opaque job identifiers.
      - Preserve JSON tags and semantics; only change the Go field type.
    - For cluster and node identifiers:
      - Replace `ClusterID string` with `ClusterID domaintypes.ClusterID` for fields representing cluster descriptors, keeping JSON/YAML tags stable:
        - `internal/deploy/{detect.go,workstation_config.go,ca_rotation_types.go,bootstrap_types.go}`.
        - `internal/nodeagent/config.go` (`ClusterID string \`yaml:"cluster_id"\`` → `domaintypes.ClusterID`).
        - `cmd/ploy/node_command.go` and `internal/server/config/types.go` where `cluster_id` identifies a cluster.
      - Replace `NodeID string` with `NodeID domaintypes.NodeID` in:
        - `internal/nodeagent/config.go`, `internal/nodeagent/heartbeat.go`, and `internal/server/handlers/bootstrap.go`.
        - `internal/worker/lifecycle/{collector.go,types.go}`, `internal/server/events/service.go`, and `internal/stream/hub.go`.
        - CLI types that carry node IDs, such as `internal/cli/logs/printer.go`, `internal/cli/transfer/client.go`, and `cmd/ploy/node_command.go`.
      - Maintain JSON/YAML tags (e.g., `json:"node_id"`, `yaml:"node_id"`); only update types.
    - For batched run repositories:
      - Introduce `domaintypes.RunRepoID` in handler- and CLI-level code that references per-repo IDs in batched runs while keeping store/sqlc models as `string`:
        - `internal/server/handlers/runs_batch_types.go` and `internal/server/handlers/runs_batch_http.go` when dealing with repo IDs returned from `store.RunRepo`.
        - CLI side structs for batch status where repo IDs are currently plain `string`.
      - Convert between `RunRepoID` and `string` at boundaries:
        - `id domaintypes.RunRepoID` ↔ `string(id)` when calling store methods.
  - Tests:
    - For each package touched, run focused tests before and after type changes:
      - `go test ./internal/server/handlers -run '(Diffs|Artifacts|Nodes|RunsBatch)'`
      - `go test ./internal/server/events ./internal/stream`
      - `go test ./internal/cli/... ./cmd/ploy`
      - `go test ./internal/deploy/... ./internal/nodeagent/... ./internal/worker/...`
    - Confirm that there are no JSON schema or OpenAPI changes (wire types remain strings) by running `go test ./docs/api`.

## Remove TicketID alias and migrate remaining call sites to RunID
- [ ] Eliminate `TicketID` alias from `internal/domain/types` and replace residual Ticket-based terminology with `RunID` usage — finish the migration so `RunID` is the only domain type for Mods run identifiers.
  - Repository: github.com/iw2rmb/ploy
  - Component: `internal/domain/types`, `internal/cli/mods`, `internal/server`, docs
  - Scope:
    - In `internal/domain/types/ids.go`:
      - Remove `type TicketID = RunID` and all Ticket-specific comments.
      - Update any references in comments to state that `RunID` is the canonical run identifier, with no alias.
    - In `internal/domain/types/ids_test.go` and `internal/domain/types/adapters_test.go`:
      - Delete the `ticket_alias_compatibility` subtest and any tests that mention `TicketID` explicitly.
      - Ensure all remaining tests refer only to `RunID` for run identifier behavior.
    - In CLI Mods helpers:
      - Replace internal structs that still speak about `TicketID` with `RunID`:
        - `internal/cli/mods/submit.go`: anonymous responses that expose `TicketID string \`json:"run_id"\`` should be renamed to `RunID string \`json:"run_id"\``; mapping to `modsapi.RunSummary` should use `domaintypes.RunID(resp.RunID)`.
        - `internal/cli/mods/batch.go`: change helper response fields and uses of `srvResp.TicketID` to `srvResp.RunID`.
        - `internal/cli/mods/events.go`: remove `TicketID` field aliases and operate on `RunID` naming only.
      - Keep JSON `run_id` field names unchanged for all wire types.
    - In control-plane tests and handlers:
      - Search for `TicketID` in `internal/server`:
        - Update any comments like “formerly TicketID” to historical notes that can be removed once alias is gone, or drop them if no longer needed.
        - Ensure events and completion handlers refer only to `RunID` in comments and variables.
    - Documentation cleanup:
      - In `roadmap/ticket-id.md`, mark the alias removal step as complete or note that the alias no longer exists once this work lands.
      - Scan `docs/mods-lifecycle.md`, `docs/api/OpenAPI.yaml`, and `docs/api/components/schemas/controlplane.yaml` for remaining “TicketID” references and replace them with “RunID”/“run id” terminology where they describe type names (not wire fields).
  - Tests:
    - `go test ./internal/domain/types`
    - `go test ./internal/cli/mods/... ./cmd/ploy`
    - `go test ./internal/server/...`
    - `go test ./docs/api`
