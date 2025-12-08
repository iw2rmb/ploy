# Ticket ID → Run ID migration

Scope: Replace remaining `TicketId`/`TicketID`/ticket terminology with `RunId`/`RunID`/run across the Ploy Mods stack (domain IDs, workflow contracts, control-plane handlers, CLI, docs) while keeping wire formats (`run_id`) and behavior stable.

Documentation: `ROADMAP.md`, `docs/mods-lifecycle.md`, `docs/api/OpenAPI.yaml`, `docs/api/components/schemas/controlplane.yaml`, `docs/envs/README.md`.

Legend: [ ] todo, [x] done.

## Domain identifiers
- [x] Collapse duplicate run identifiers — consolidate on `RunID` and remove `TicketID` in `internal/domain/types/ids.go` — avoids parallel ID types for the same concept.
  - Repository: ploy
  - Component: `internal/domain/types`
  - Scope: Replace `TicketID` type and methods with `RunID` usage; update tests in `internal/domain/types/ids_test.go` and `internal/domain/types/adapters_test.go` to cover `RunID`.
  - Snippets: `type RunID string`, `func (v RunID) IsZero() bool { return IsEmpty(string(v)) }`.
  - Tests: `go test ./internal/domain/types` — all tests pass with `RunID`-only API.

## Workflow runtime and node agent
- [ ] Thread `RunID` through container execution API instead of `TicketID` — ensures labels and telemetry consistently use run identifiers.
  - Repository: ploy
  - Component: `internal/workflow/runtime/step`, `internal/nodeagent`
  - Scope: Change `step.Request` field from `TicketID types.TicketID` to `RunID types.RunID`; update `buildContainerSpec` to accept `RunID`; switch node agent calls in `internal/nodeagent/execution_orchestrator.go` to pass `RunID` directly.
  - Snippets: `req := step.Request{RunID: req.RunID, Manifest: manifest, Workspace: workspace, OutDir: outDir, InDir: ""}`.
  - Tests: `go test ./internal/workflow/runtime/step ./internal/nodeagent/...` — label tests (`labels_test.go`) still assert `LabelRunID` is set from `RunID`.

## Mods API types and events
- [ ] Rename Mods API fields from TicketID to RunID — align type names with `run_id` JSON fields.
  - Repository: ploy
  - Component: `internal/mods/api`, `internal/server/events`, `internal/stream`
  - Scope: In `internal/mods/api/types.go`, change `RunSummary.TicketID domaintypes.TicketID` to `RunID domaintypes.RunID`; update `RunSubmitRequest` to use `RunID` or drop the field when unused; rename `PublishTicket` to `PublishRun` in `internal/stream/hub.go` and `internal/server/events/service.go`, updating all callers.
  - Snippets: `type RunSummary struct { RunID domaintypes.RunID \`json:"run_id"\` ... }`.
  - Tests: `go test ./internal/mods/api ./internal/stream ./internal/server/events` — fuzz tests and stream tests still pass with renamed methods and fields.

## Mods control plane handlers
- [ ] Rename Mods HTTP handlers and summaries to run-centric naming — remove ticket language from server-facing API while preserving routes.
  - Repository: ploy
  - Component: `internal/server/handlers`
  - Scope: In `mods_ticket.go`, rename `submitTicketHandler` → `submitRunHandler`, `getTicketStatusHandler` → `getRunStatusHandler`; construct `modsapi.RunSummary` with `RunID` instead of `TicketID`. In `mods_cancel.go` and `mods_resume.go`, populate `RunSummary.RunID` and update log messages/comments to refer to runs.
  - Snippets: `summary := modsapi.RunSummary{RunID: domaintypes.RunID(run.ID), ...}`.
  - Tests: `go test ./internal/server/handlers` — all Mods handler tests updated to expect `RunID` fields and run-centric wording.

## CLI and user-facing terminology
- [ ] Normalise CLI commands and helpers on Run ID terminology — make user-facing text speak about runs, not tickets, without breaking flags.
  - Repository: ploy
  - Component: `cmd/ploy`, `internal/cli/mods`, `internal/cli/runs`
  - Scope: 
    - Rename helpers in `cmd/ploy/mod_run_exec.go`: `buildTicketRequest` → `buildRunRequest`, `submitTicket` → `submitRun`, `followTicketEvents` → `followRunEvents`, `downloadTicketArtifacts` → `downloadRunArtifacts`; keep JSON `run_id` field name stable.
    - Switch output messages from `"Mods ticket %s submitted"` to `"Mods run %s submitted"` in `cmd/ploy/mod_run.go` and related tests.
    - In `internal/cli/mods/{submit,events,logs,artifacts,inspect,diffs,cancel,resume}.go`, rename struct fields from `Ticket` to `RunID` and update error messages to say “run id required” (only where this does not change CLI flag names).
    - Update `internal/cli/runs/inspect.go` to print `Run %s: ...` instead of `Ticket %s: ...`.
  - Snippets: `_, _ = fmt.Fprintf(stderr, "Mods run %s submitted (state: %s)\n", summary.RunID, summary.State)`.
  - Tests: `go test ./cmd/ploy ./internal/cli/...` — golden outputs and error message assertions adjusted to run-centric wording.

## Workflow contracts and subjects
- [ ] Align workflow contracts with Run ID naming — remove `ticket_id` field names and helper names where they encode runs.
  - Repository: ploy
  - Component: `internal/workflow/contracts`
  - Scope: 
    - In `workflow_ticket.go`, change JSON tag `RunID types.RunID "json:\"ticket_id\""` to `"run_id"` and error messages from “ticket_id is required” to “run_id is required`.
    - In `contracts.go`, consider renaming `SubjectsForTicket` → `SubjectsForRun` and updating call sites and tests.
    - In `stub.go`, rename `EnqueueTicket`/`ClaimTicket` to run-centric counterparts while keeping behavior identical.
  - Snippets: `func SubjectsForRun(runID string) SubjectSet { trimmed := strings.TrimSpace(runID); ... }`.
  - Tests: `go test ./internal/workflow/contracts` — JSON compatibility and subject tests updated to `run_id` / run naming.

## Documentation and OpenAPI
- [ ] Update docs and OpenAPI to describe runs instead of tickets — eliminate ticket terminology from public docs while keeping endpoints stable.
  - Repository: ploy
  - Component: `docs`, `docs/api`
  - Scope: 
    - `docs/api/OpenAPI.yaml`: replace “ticket submission” with “run submission”; drop `TicketID/RunID` phrasing in type semantics; keep `run_id` fields unchanged.
    - `docs/api/components/schemas/controlplane.yaml`: rename “Ticket submission (simplified Mods facade)” to “Run submission” and call `RunStatus.metadata` “run metadata”.
    - `docs/mods-lifecycle.md`: switch core lifecycle descriptions and handler names to run-centric language (`submitRunHandler`, `getRunStatusHandler`, `PublishRun`).
    - `docs/envs/README.md`, `docs/build-gate/README.md`: update examples from `<ticket-id>` to `<run-id>`; clarify `PLOY_E2E_TICKET_PREFIX` semantics or rename if in scope.
  - Snippets: `ploy mod inspect <run-id>` in examples; narrative refers to “run status” and “run events”.
  - Tests: `go test ./docs/api` — OpenAPI verification passes with updated schemas and descriptions.

