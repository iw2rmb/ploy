# Remove backward compatibility code

Scope: Remove backward‑compatibility shims in CLI, control plane, nodeagent, workflow, and docs. Simplify contracts to a single canonical shape per surface (Mods API, endpoints, specs) assuming fresh redeploys only.

Documentation: AGENTS.md, docs/mods-lifecycle.md, docs/api/OpenAPI.yaml, cmd/ploy/README.md, internal/mods/api/types.go, internal/server/handlers/*, internal/nodeagent/*, internal/workflow/contracts/*, internal/cli/*.

Legend: [ ] todo, [x] done.

## Mods API wire contracts
- [x] Collapse Mods API responses to a single canonical type (no `ticket` wrapper, no legacy field names) — Ensure GET/POST /v1/mods return one consistent JSON schema.
  - Repository: ploy
  - Component: internal/mods/api, internal/server/handlers (mods_ticket.go), internal/cli/mods, cmd/ploy/mods_*.
  - Scope: Replace `RunSubmitResponse`, `RunStatusResponse`, and `Ticket` wrapper usage in internal/mods/api/types.go and handler responses in internal/server/handlers/mods_ticket.go. Update callers in internal/cli/mods and tests in cmd/ploy/* and internal/server/handlers/* to use the new canonical type. Remove comments and docs that reference “backward compatibility” for `ticket` / `stages` naming.
  - Snippets: Encode `modsapi.RunSummary` (or a new canonical struct) directly from handlers instead of `modsapi.RunStatusResponse{Ticket: summary}`.
  - Tests: go test ./internal/mods/... ./internal/server/handlers/... ./cmd/ploy/... — All existing Mods status/submit tests must pass with updated types and response shapes.

- [x] Decide and document the canonical Mods run status schema in OpenAPI/docs — Keep wire contracts discoverable and stable.
  - Repository: ploy
  - Component: docs/api, docs/mods-lifecycle.md.
  - Scope: Update docs/api/OpenAPI.yaml and docs/mods-lifecycle.md to describe the new response shape for Mods submit/status and SSE events (no `ticket` wrapper, clarified `stages` semantics or a new field name if changed). Remove notes about “retained for API backward compatibility”.
  - Snippets: N/A (documentation only).
  - Tests: go test ./docs/... (if any doc validation) + manual review; ensure docs/api/verify_openapi_test.go still passes.

## Mods submit flow (CLI ↔ server)
- [x] Remove dual response handling (201 vs 202) and simplified fallback payload from Mods submit CLI — Use a single canonical submit contract.
  - Repository: ploy
  - Component: internal/cli/mods (submit.go), cmd/ploy (mods commands).
  - Scope: In internal/cli/mods/submit.go, drop the 202 path that decodes modsapi.RunSubmitResponse and the second POST that sends simplified `{repo_url,base_ref,target_ref,spec}`. Keep only the canonical request+response path (e.g., 201 with a single summary type). Update cmd/ploy tests that assert 202 or dual behavior.
  - Snippets: Replace the switch on resp.StatusCode with a single case for the canonical status and error handling for all others.
  - Tests: go test ./internal/cli/mods/... ./cmd/ploy/... — Submit tests must pass with a single response path.

- [x] Align server Mods submit handler with the canonical CLI contract — Avoid supporting legacy response shapes.
  - Repository: ploy
  - Component: internal/server/handlers (mods_ticket.go and related).
  - Scope: Ensure POST /v1/mods always returns the chosen canonical status code and JSON shape. Remove any code that builds or accepts legacy submit responses tied to modsapi.RunSubmitResponse or other legacy envelopes.
  - Snippets: N/A (implementation detail is handler-specific).
  - Tests: go test ./internal/server/handlers/... — Mods submit handler tests must only reference the canonical contract.

## Node vs job completion / ack / logs
- [x] Remove node-based completion endpoint when job-level completion is canonical — Simplify node → server contract to /v1/jobs/{job_id}/complete only.
  - Repository: ploy
  - Component: internal/server/handlers (nodes_complete.go, jobs_complete.go), internal/nodeagent (statusuploader.go), router registration.
  - Scope: Verify that internal/nodeagent/statusuploader.go only uses /v1/jobs/{job_id}/complete. Remove completeRunHandler and its route from internal/server/handlers/nodes_complete.go and the router wiring if unused. Delete tests that depend on /v1/nodes/{id}/complete.
  - Snippets: N/A (mostly deletions and route changes).
  - Tests: go test ./internal/nodeagent/... ./internal/server/handlers/... — Nodeagent and completion tests must pass using only the job-level endpoint.

- [x] Remove ackRunStart backward-compatibility handler or narrow its role — Let claim/completion paths drive run status and events.
  - Repository: ploy
  - Component: internal/server/handlers/nodes_ack.go, internal/server/events, any nodeagent callers.
  - Scope: Confirm whether any nodeagent or CLI code still calls /v1/nodes/{id}/runs/ack. If not needed, remove ackRunStartHandler, its route, and tests. If SSE “run started” events are still required, emit them from claim logic or completion instead of a separate ack endpoint.
  - Snippets: N/A (handler deletion or simplification).
  - Tests: go test ./internal/server/handlers/... ./internal/server/events/... — Run lifecycle and SSE tests must pass without the ack endpoint.

- [ ] Make events service mandatory for logs; remove direct store write fallbacks — Use a single logging path.
  - Repository: ploy
  - Component: internal/server/handlers (nodes_logs.go, runs_logs.go), internal/server/events.
  - Scope: In createNodeLogsHandler and run logs handler, drop the `eventsService == nil` branches; require eventsService and always call CreateAndPublishLog. Update server wiring to always construct events.Service. Delete or adjust tests that exercised the direct st.CreateLog fallback.
  - Snippets: Replace conditional eventService usage with a single call to eventsService.CreateAndPublishLog.
  - Tests: go test ./internal/server/handlers/... ./internal/server/events/... — Log ingestion and SSE tests must pass with the events service required.

## Spec, healing, and image compatibility
- [ ] Tighten Mods spec parsing to a single canonical shape — Stop supporting legacy single-mod `mod` fallbacks.
  - Repository: ploy
  - Component: internal/nodeagent/claimer_spec.go, internal/nodeagent/claimer_spec_test.go, cmd/ploy/mod_run_spec_parsing_test.go.
  - Scope: In parseSpec, remove fallbacks that treat `mod` as a legacy single-mod spec when top-level fields are missing. Define and implement a canonical spec structure (e.g., multi-step `mods[]` plus structured healing config) and require it. Update tests that currently assert mixed legacy behavior.
  - Snippets: Simplify parseSpec to handle only the canonical schema; delete branches that copy from `mod.*` into top-level fields for BC.
  - Tests: go test ./internal/nodeagent/... ./cmd/ploy/... — Spec parsing tests must reflect only the canonical shape.

- [ ] Drop single-strategy healing fallback in maybeCreateHealingJobs — Require new healing configuration structure.
  - Repository: ploy
  - Component: internal/server/handlers/nodes_complete_healing.go.
  - Scope: Remove the block that converts top-level `mods[]` into a single unnamed healing strategy “for backward compatibility.” Require callers to provide healing strategies via the new `build_gate_healing` schema. Update tests that depend on the single-strategy fallback.
  - Snippets: Delete the `fallback to single-strategy form (mods[] at top level)` code path.
  - Tests: go test ./internal/server/handlers/... — Healing behavior tests must configure healing explicitly via the canonical schema.

- [ ] Re-evaluate ModImage dual-form handling (string vs map) — Optionally narrow accepted forms if desired.
  - Repository: ploy
  - Component: internal/workflow/contracts/mod_image.go, internal/nodeagent/manifest.go, internal/workflow/runtime/*.
  - Scope: Decide whether both universal string and stack-map forms remain supported. If narrowing, update ParseModImage and related code to accept only the chosen canonical form, and adjust manifest builders and tests accordingly. If keeping both, leave as-is (no-op step).
  - Snippets: N/A unless narrowing; then simplify ParseModImage to a single form.
  - Tests: go test ./internal/workflow/contracts/... ./internal/nodeagent/... — Image resolution tests must match the chosen contract.

## Worker lifecycle / status snapshots
- [ ] Remove map-based status accessors in lifecycle cache — Use typed NodeStatus everywhere.
  - Repository: ploy
  - Component: internal/worker/lifecycle/cache.go, internal/worker/lifecycle/types.go, status providers/consumers.
  - Scope: Find all callers of Cache.LatestStatusMap and migrate them to use Cache.LatestStatus and NodeStatus directly. Once all callers are updated, remove LatestStatusMap and any map-based SnapshotSource shims. Keep ToMap only if it is still used for JSON output; otherwise consider simplifying its shape.
  - Snippets: Replace usages of LatestStatusMap with typed accessors on NodeStatus.
  - Tests: go test ./internal/worker/... — Worker lifecycle and status reporting tests must pass without map-based helpers.

## Nodeagent options and manifest BC
- [ ] Remove raw options map round-trip when typed RunOptions is sufficient — Reduce duplicate state.
  - Repository: ploy
  - Component: internal/nodeagent/run_options.go, internal/nodeagent/run_options_test.go, internal/nodeagent/manifest.go.
  - Scope: Audit how RunOptions and the raw options map are used. If all consumers can operate on RunOptions, remove the need to preserve the raw map “for backward compatibility” and update tests that assert its presence. Simplify manifest and step builders to use only typed fields.
  - Snippets: Delete fields and methods that exist solely to mirror raw map[string]any into RunOptions.
  - Tests: go test ./internal/nodeagent/... — RunOptions and manifest tests must pass with typed-only options.

- [ ] Make manifest builders require explicit stack where appropriate — Avoid relying on “unknown” for BC.
  - Repository: ploy
  - Component: internal/nodeagent/manifest.go, internal/workflow/contracts/mod_image.go.
  - Scope: Collapse buildManifestFromRequest wrapper into buildManifestFromRequestWithStack where callers can provide a concrete stack. For callers that truly cannot know the stack, document and keep the “unknown” path explicitly rather than labeling it as backward compatibility. Update tests to call the stack-aware builder directly.
  - Snippets: Replace calls to buildManifestFromRequest with buildManifestFromRequestWithStack and explicit contracts.ModStack values.
  - Tests: go test ./internal/nodeagent/... ./internal/workflow/contracts/... — Manifest tests must pass with explicit stack handling.

- [ ] Tighten JobMeta JSON handling; treat legacy shapes as invalid if acceptable — Enforce structured metadata going forward.
  - Repository: ploy
  - Component: internal/workflow/contracts/job_meta.go.
  - Scope: In UnmarshalJobMeta, reconsider “backward compatibility” behavior for empty `{}`/`null` and missing `kind`. If acceptable, change logic to require non-empty kind and return an error for invalid payloads (or handle them via explicit migration). Update tests accordingly.
  - Snippets: Replace defaulting `m.Kind = JobKindMod` with validation and error handling.
  - Tests: go test ./internal/workflow/contracts/... — Job meta tests must reflect the stricter expectations.

## CLI surface and tests
- [ ] Remove deprecated CLI flags and BC mentions from docs — Simplify user-facing interface.
  - Repository: ploy
  - Component: cmd/ploy, cmd/ploy/README.md, relevant cobra command files.
  - Scope: Remove deprecated flags like `--retry-wait` where the README says “preserved for backward compatibility,” and update command help/usage strings. Adjust tests that rely on deprecated flags.
  - Snippets: Delete flag declarations and update usage examples in cmd/ploy/README.md.
  - Tests: go test ./cmd/ploy/... — CLI tests must pass without deprecated flags.

- [ ] Drop CLI type aliases kept solely for backward compatibility — Use canonical types directly.
  - Repository: ploy
  - Component: internal/cli/runs/follow.go, internal/cli/mods/logs.go.
  - Scope: Remove `type Format = logs.Format` re-exports when no longer needed as a public API. Update any external or internal callers to import and use logs.Format directly (if any remain).
  - Snippets: Replace usages of runs.Format/mods.Format with logs.Format where necessary.
  - Tests: go test ./internal/cli/... — Logs/follow tests must pass with direct use of logs.Format.

- [ ] Remove execute helper in main.go once tests are updated — Keep a single CLI entrypoint.
  - Repository: ploy
  - Component: cmd/ploy/main.go, cmd/ploy tests.
  - Scope: Update tests that depend on execute(args, stderr) to instead construct and execute newRootCmd directly. After tests are updated, delete execute and its comment about backward compatibility.
  - Snippets: Replace test calls to execute with rootCmd := newRootCmd(...); rootCmd.SetArgs(...); rootCmd.Execute().
  - Tests: go test ./cmd/ploy/... — All CLI tests must pass without execute.

