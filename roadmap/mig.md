# Replace `mod` With `mig` Across Project

Scope: Rename the workflow concept from `mod`/`mods` to `mig`/`migs` across code, API, CLI, docs, tests, and container images. No backward compatibility layer is kept: old names are removed, old routes/commands are deleted, and docs/examples are rewritten to the new naming only.

Precondition: Execute `roadmap/next.md` first. This roadmap does not include migration of `ModType`/`ModImage` fields, because those are replaced earlier by `Type` (`JobType`) and `Image` (`JobImage`).

Documentation: `AGENTS.md`; `README.md`; `docs/migs-lifecycle.md`; `docs/api/OpenAPI.yaml`; `docs/how-to/publish-migs.md`; `deploy/images/migs/README.md`; `tests/e2e/migs/README.md`; `docs/testing-workflow.md`.

Legend: [ ] todo, [x] done.

## Rename Matrix and Exclusions

Canonical mapping for this migration:

| Legacy | Canonical |
| --- | --- |
| `mod` | `mig` |
| `mods` | `migs` |
| `mod-` | `mig-` |
| `mods-` | `migs-` |
| `/v1/mods` | `/v1/migs` |
| `ploy mod` | `ploy mig` |

Exclusions (do not rename these in bulk scans):

- Go module/tooling terms and files: `go.mod`, `go.sum`, `go mod ...`
- Generic English words that contain `mod` without workflow meaning (for example `modify`, `mode`)
- Job field names already migrated by `roadmap/next.md`: `ModType`/`mod_type` -> `Type`/`job_type`, `ModImage`/`mod_image` -> `Image`/`job_image`

## Phase 0: Naming Contract and Guardrails (RED)
- [x] Define exact rename matrix and exclusions before touching implementation — prevents accidental renames of unrelated terms (for example `go.mod` and generic "modify").
  - Repository: `ploy`
  - Component: Architecture glossary, contributor rules
  - Scope: Add a short mapping section in this roadmap and reference it from `design/` docs used by contributors; mapping must include `mod`->`mig`, `mods`->`migs`, `mod-`->`mig-`, `mods-`->`migs-`, `/v1/mods`->`/v1/migs`, `ploy mod`->`ploy mig`; also document exclusions, including already-migrated `Type`/`Image` job fields from `roadmap/next.md`.
  - Snippets: `rg -n '\\bmods?\\b|/v1/mods|ploy mod|mod-' cmd internal deploy docs tests`
  - Tests: Add guard tests/scripts that fail if new `mod` naming is introduced outside approved exclusions.

- [x] Add RED checks for forbidden legacy naming in key surfaces — enforces "no compatibility" policy automatically.
  - Repository: `ploy`
  - Component: Test guardrails, CI hygiene
  - Scope: Add or extend guard tests under `tests/guards/` to block `/v1/mods`, `internal/mods`, `ploy mod`, `deploy/images/mods`, and `mods-` image names once the rename lands.
  - Snippets: `go test ./tests/guards/...`
  - Tests: RED expected first; checks pass only after all rename slices are complete.

## Phase 1: Domain, Store, and Server Rename
- [x] Rename core domain/store identifiers from mod(s) to mig(s) — keeps server internals consistent with the new canonical term.
  - Repository: `ploy`
  - Component: `internal/domain`, `internal/store`, SQL query layer
  - Scope: Rename files/types/params such as `mods.go`, `modref_*`, `mods.sql`, `mod_repos.sql`, related generated sqlc files, and table/entity references to `migs` naming; remove old symbols. Update DB schema names in `internal/store/schema.sql` as part of the same slice (tables, columns, foreign keys, indexes, and constraints that contain `mod`/`mods` naming), then regenerate and update store/query artifacts accordingly.
  - Snippets: `internal/store/queries/mods.sql` -> `internal/store/queries/migs.sql`; `internal/domain/types/mods.go` -> `internal/domain/types/migs.go`
  - Tests: `make test` with focus on store/domain packages; run coverage to ensure critical runner/store packages stay within targets.

- [x] Rename HTTP handlers, routes, and event contracts to mig(s) — aligns control-plane API surface to one vocabulary.
  - Repository: `ploy`
  - Component: `internal/server/handlers`, router wiring, stream/event contracts
  - Scope: Replace handler files and route registrations named `mods_*` with `migs_*`; rename endpoint paths from `/v1/mods/...` to `/v1/migs/...`; remove legacy route registration.
  - Snippets: `internal/server/handlers/mods_*.go`, `internal/server/events/*`
  - Tests: Handler unit tests updated to `/v1/migs/...`; integration tests for run creation, spec/repo operations, and pull resolution.

## Phase 2: CLI and Client Surface Rename
- [x] Replace CLI command group `mod` with `mig` end-to-end — users should have only one command vocabulary.
  - Repository: `ploy`
  - Component: `cmd/ploy`, `internal/cli/*`
  - Scope: Rename command files (`mod_*.go` -> `mig_*.go`), command registration/help text (`ploy mod` -> `ploy mig`), and all flags/messages/docs strings (`--mod-image` style flags to `--job-image`, etc.) without aliases.
  - Snippets: `cmd/ploy/mod_command.go`, `cmd/ploy/mod_run_flags.go`, `internal/cli/mods/*`
  - Tests: CLI command tests + smoke checks updated to `ploy mig`; rebuild help golden files.

- [x] Rename internal package/module paths from `mods` to `migs` — removes mixed imports and inconsistent package names.
  - Repository: `ploy`
  - Component: `internal/mods`, `internal/cli/mods`, imports across repository
  - Scope: Move package directories and update import paths (`internal/mods/api` -> `internal/migs/api`, `internal/cli/mods` -> `internal/cli/migs`), then update all call sites.
  - Snippets: `rg -n 'internal/mods|internal/cli/mods' cmd internal tests docs`
  - Tests: `make test`; `make build`; verify no stale import path remains.

## Phase 3: Runtime Image and Artifact Rename
- [x] Rename image directories, scripts, and tags from mods to migs — keeps build/publish/runtime naming consistent.
  - Repository: `ploy`
  - Component: `deploy/images`, image build scripts, e2e image references
  - Scope: Rename `deploy/images/mods` to `deploy/images/migs`; update `build-and-push-mods.sh` to mig naming and tags (`mods-*` -> `migs-*`); rename image folder names where prefixed by `mod-`.
  - Snippets: `deploy/images/build-and-push-migs.sh`, `deploy/images/migs/*`, references in `tests/e2e/*` and docs
  - Tests: Build each renamed image locally; run representative e2e scenarios that pull/use renamed images.

- [x] Rename runtime artifact and temp naming where user-facing — avoids mixed terminology in output bundles/logs.
  - Repository: `ploy`
  - Component: Nodeagent execution artifacts, CLI artifact fetch/output labels
  - Scope: Replace user-facing names like `mod-out`, `ploy-mod-in-*`, and related log labels with `mig-*`; keep binary behavior otherwise unchanged.
  - Snippets: `internal/nodeagent/execution_orchestrator_jobs.go`, `internal/nodeagent/execution_healing_io.go`, related tests
  - Tests: Nodeagent unit tests for artifact names + e2e artifact extraction scripts.

## Phase 4: Test Tree and Fixtures Rename
- [x] Rename test directories, scenarios, and fixture names under `tests/*/mods` — keeps test paths aligned with production vocabulary.
  - Repository: `ploy`
  - Component: `tests/e2e`, `tests/integration`, `tests/unit`, smoke scripts
  - Scope: Move `tests/e2e/mods` and `tests/integration/mods` to `migs`, rename scripts/spec files with `mod` prefixes where they represent the feature, and update all invocations.
  - Snippets: `tests/e2e/mods/scenario-*.sh`, `tests/integration/mods/*`, `tests/smoke_tests.sh`
  - Tests: Run renamed e2e selftest and core integration suites from new paths.

- [x] Update golden outputs and static fixtures for CLI/API naming changes — prevents flaky diffs after rename.
  - Repository: `ploy`
  - Component: `cmd/ploy/testdata`, API verification fixtures, docs examples in tests
  - Scope: Regenerate or rewrite help text fixtures and API path assertions from `mod`/`mods` to `mig`/`migs`.
  - Snippets: `cmd/ploy/testdata/help_mod.txt`, `docs/api/verify_openapi_test.go`
  - Tests: Targeted fixture tests + full `make test`.

## Phase 5: Documentation and OpenAPI Rewrite
- [x] Rewrite docs to mig vocabulary and new paths only — published behavior must match implementation exactly.
  - Repository: `ploy`
  - Component: `docs/`, `README.md`, API docs
  - Scope: Rename and rewrite docs like `docs/migs-lifecycle.md`, `docs/how-to/publish-migs.md`, `docs/schemas/mig.example.yaml`, and cross-links to new names; remove old doc filenames/content.
  - Snippets: `docs/api/OpenAPI.yaml` paths `/v1/migs/...`; schema examples and CLI snippets using `ploy mig ...`
  - Tests: `go test ./docs/api/...`; validate all markdown links and command snippets referenced by tests/scripts.

- [x] Keep `docs/` synchronized with each implementation slice — enforces project doc policy at commit time.
  - Repository: `ploy`
  - Component: Documentation governance
  - Scope: For each commit in this migration, update corresponding docs and cross-references in the same diff; avoid trailing stale `mod` docs.
  - Snippets: per-slice checklist in PR description
  - Tests: PR gate requires docs diff for code/API/CLI surface changes.

## Phase 6: GREEN + REFACTOR Completion Gate
- [ ] Execute full validation suite on final mig-only tree — proves rename completeness.
  - Repository: `ploy`
  - Component: Build/test quality gate
  - Scope: Run `make test`, `make coverage`, `make vet`, `make staticcheck`, `make build`, plus selected local-cluster e2e scenarios from renamed `tests/e2e/migs`.
  - Snippets: `PLOY_CONFIG_HOME=$PWD/deploy/local/cli make test`
  - Tests: All checks green; coverage thresholds maintained (`>=60%` overall and `>=90%` critical runner packages).

- [ ] Run final residue scan and cleanup — ensures no accidental legacy naming remains.
  - Repository: `ploy`
  - Component: Whole tree hygiene
  - Scope: Execute repository-wide scans for legacy names; remove temporary debug hooks and transitional comments introduced during migration.
  - Snippets: `rg -n '\\bmod(s)?\\b|/v1/mods|ploy mod|internal/mods|deploy/images/mods|mods-'`
  - Tests: Scan output limited to explicitly approved exclusions only.
