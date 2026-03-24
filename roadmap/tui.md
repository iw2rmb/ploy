# Ploy CLI TUI Implementation

Scope: deliver `ploy tui` with the six-screen navigation contract, Bubble Tea v2 stack, fixed list width/help rules, and required jobs data surface.

Documentation: `design/tui.md`; `docs/envs/README.md`; `docs/testing-workflow.md`; `cmd/ploy/README.md`.

- [x] 1.1 [determined] Add TUI Jobs Data API Contract
  - Repository: `ploy`
  - Component: `internal/store/queries/jobs.sql`; `internal/store/jobs.sql.go`; `internal/server/handlers/register.go`; `internal/server/handlers/jobs_list.go` (new); `docs/api/OpenAPI.yaml`
  - Implementation:
    1. Add store queries that list jobs ordered newest-to-oldest with optional `run_id` filtering and total counting.
    2. Add `GET /v1/jobs` handler that validates pagination/filter inputs and returns `job_id`, `name`, `mig_name`, `run_id`, `repo_id`, and `total`.
    3. Register the new jobs-list route in server route wiring.
    4. Document the endpoint contract in OpenAPI.
  - Verification:
    1. `go test ./internal/store -run 'Test.*Jobs.*List.*|Test.*Ordering.*'`
    2. `go test ./internal/server/handlers -run 'Test.*Jobs.*List.*'`
    3. `go test ./docs/api/...`
  - Reasoning: `high`

- [x] 1.2 [determined] Add TUI CLI Data Client Layer
  - Repository: `ploy`
  - Component: `internal/cli/tui/*.go` (new)
  - Implementation:
    1. Add client commands for migrations list, runs list, jobs list, migration repo totals, migration run totals, and run totals needed by screens.
    2. Reuse existing control-plane HTTP client validation/error wrapping conventions from `internal/cli/httpx`.
    3. Add table-driven client tests with `httptest.Server` for success and error paths.
  - Verification:
    1. `go test ./internal/cli/tui -run 'Test.*'`
    2. `go test ./internal/cli/... -run 'Test.*TUI.*|Test.*HTTP.*'`
  - Reasoning: `medium`

- [x] 1.3 [determined] Bootstrap Bubble Tea v2 TUI Shell
  - Repository: `ploy`
  - Component: `internal/tui/*.go` (new); `cmd/ploy/tui_command.go` (new); `go.mod`; `go.sum`
  - Implementation:
    1. Add Bubble Tea v2/Bubbles v2/Lip Gloss v2 module dependencies.
    2. Add the base TUI model/update/view wiring and command entrypoint for `ploy tui`.
    3. Configure shared list invariants: width `24` and help disabled for all lists.
  - Verification:
    1. `go test ./internal/tui -run 'Test.*Init.*|Test.*Model.*'`
    2. `go test ./cmd/ploy -run 'Test.*TUI.*'`
  - Reasoning: `medium`

- [x] 1.4 [determined] Implement State S1 Root (`PLOY`)
  - Repository: `ploy`
  - Component: `internal/tui/model_*.go`; `internal/tui/view_*.go`; `internal/tui/model_root_test.go` (new)
  - Implementation:
    1. Implement root list title `PLOY` with items `Migrations`, `Runs`, `Jobs` and required detail lines (`select migration|run|job`).
    2. Disable filtering/search for the root list.
    3. Implement `Enter` transitions from root to `S2`, `S4`, and `S6` according to selected item.
  - Verification:
    1. `go test ./internal/tui -run 'Test.*Root.*|Test.*S1.*'`
  - Reasoning: `low`

- [x] 1.5 [determined] Implement State S2 Migrations List (`PLOY | MIGRATIONS`)
  - Repository: `ploy`
  - Component: `internal/tui/model_*.go`; `internal/tui/view_*.go`; `internal/tui/model_migrations_test.go` (new)
  - Implementation:
    1. Render side-by-side `PLOY` and `MIGRATIONS` lists and populate migration rows with name and mig id.
    2. Enforce newest-to-oldest migration ordering in rendered items.
    3. Implement `Enter` transition to `S3` for selected migration and `Esc` transition back to `S1`.
  - Verification:
    1. `go test ./internal/tui -run 'Test.*Migrations.*|Test.*S2.*'`
  - Reasoning: `medium`

- [x] 1.6 [determined] Implement State S3 Migration Details (`MIGRATION <...>`)
  - Repository: `ploy`
  - Component: `internal/tui/model_*.go`; `internal/tui/view_*.go`; `internal/tui/model_migration_details_test.go` (new)
  - Implementation:
    1. Render migration details list with items `repositories` and `runs` and totals sourced from client data.
    2. Keep the heading title in migration context (`MIGRATION <name or id>`).
    3. Implement `Esc` transition back to `S2`.
  - Verification:
    1. `go test ./internal/tui -run 'Test.*Migration.*Details.*|Test.*S3.*'`
  - Reasoning: `low`

- [x] 1.7 [determined] Implement State S4 Runs List (`PLOY | RUNS`)
  - Repository: `ploy`
  - Component: `internal/tui/model_*.go`; `internal/tui/view_*.go`; `internal/tui/model_runs_test.go` (new)
  - Implementation:
    1. Render side-by-side `PLOY` and `RUNS` lists with run rows containing run label/id, migration name, and `DD MM HH:mm` timestamp.
    2. Enforce newest-to-oldest run ordering in rendered items.
    3. Implement `Enter` transition to `S5` and `Esc` transition back to `S1`.
  - Verification:
    1. `go test ./internal/tui -run 'Test.*Runs.*|Test.*S4.*'`
  - Reasoning: `medium`

- [ ] 1.8 [determined] Implement State S5 Run Details (`RUN`)
  - Repository: `ploy`
  - Component: `internal/tui/model_*.go`; `internal/tui/view_*.go`; `internal/tui/model_run_details_test.go` (new)
  - Implementation:
    1. Render run details list with `Repositories` and `Jobs` totals from run-scoped data.
    2. Keep list output constrained to default components with no custom styling.
    3. Implement `Esc` transition back to `S4`.
  - Verification:
    1. `go test ./internal/tui -run 'Test.*Run.*Details.*|Test.*S5.*'`
  - Reasoning: `low`

- [ ] 1.9 [determined] Implement State S6 Jobs List (`PLOY | JOBS`)
  - Repository: `ploy`
  - Component: `internal/tui/model_*.go`; `internal/tui/view_*.go`; `internal/tui/model_jobs_test.go` (new)
  - Implementation:
    1. Render side-by-side `PLOY` and `JOBS` lists with rows showing `job`, `mig name`, `run id`, and `repo id`.
    2. Bind rows to the new jobs list API client and keep ordering deterministic.
    3. Implement `Esc` transition back to `S1`.
  - Verification:
    1. `go test ./internal/tui -run 'Test.*Jobs.*|Test.*S6.*'`
  - Reasoning: `medium`

- [ ] 1.10 [determined] Wire Command Surface, Docs, And Full Validation
  - Repository: `ploy`
  - Component: `cmd/ploy/root.go`; `cmd/ploy/main.go`; `cmd/ploy/commands_test.go`; `cmd/ploy/README.md`; `docs/envs/README.md`
  - Implementation:
    1. Add `tui` to top-level command wiring and usage/help output.
    2. Update CLI docs to describe `ploy tui` behavior and screen navigation.
    3. Confirm env-var documentation remains accurate with no new TUI-specific env vars.
    4. Run full unit and hygiene validation for touched packages.
  - Verification:
    1. `make test`
    2. `make vet`
    3. `make staticcheck`
    4. `/Users/vk/@iw2rmb/amata/scripts/check_docs_links.sh`
  - Reasoning: `medium`
