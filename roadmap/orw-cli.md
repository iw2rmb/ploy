# ORW CLI Replacement Rollout

Scope: Replace plugin-driven OpenRewrite execution (`orw-maven`, `orw-gradle`) with one isolated runtime image (`orw-cli`) that applies recipes without invoking Gradle/Maven project tasks, remove legacy ORW images and references, and align contracts/tests/docs to the new runtime.

Documentation: `AGENTS.md`; `design/orw-cli.md`; `deploy/images/migs/orw-gradle/orw-gradle.sh`; `deploy/images/migs/orw-maven/orw-maven.sh`; `deploy/images/build-and-push-migs.sh`; `deploy/images/garage.sh`; `internal/workflow/contracts/mod_image.go`; `internal/nodeagent/execution_orchestrator_jobs.go`; `docs/migs-lifecycle.md`; `docs/envs/README.md`; `docs/how-to/publish-migs.md`; `tests/integration/migs/orw_gradle_test.go`; `tests/integration/migs/orw_maven_test.go`; `tests/e2e/migs/README.md`.

Legend: [ ] todo, [x] done.

## Phase 1: Lock `orw-cli` Runtime Contract
- [x] Define and codify `orw-cli` input/output contract and failure taxonomy — make runtime behavior deterministic before implementation.
  - Repository: ploy
  - Component: workflow contracts; runtime docs
  - Scope: formalize required envs (`RECIPE_*`), repository resolution envs (`ORW_REPOS`, credentials), `report.json` schema (`success`, `error_kind`, `reason`, `message`), and unsupported attribution semantics in docs and contract structs/constants used by runtime consumers.
  - Snippets: `{"success":false,"error_kind":"unsupported","reason":"type-attribution-unavailable"}`
  - Tests: `go test ./internal/workflow/contracts -run 'Test.*Mod|Test.*Spec|Test.*Parse'` — expect strict parsing/validation for new runtime fields.

## Phase 2: Implement `orw-cli` Image And Entrypoint
- [x] Add standalone OpenRewrite CLI runner image — remove build-tool execution from ORW runtime path.
  - Repository: ploy
  - Component: image assets
  - Scope: add `deploy/images/mig/orw-cli/Dockerfile` and `deploy/images/mig/orw-cli/orw-cli.sh`; implement CA import, recipe/config resolution, CLI invocation, `transform.log` and `report.json` writing, self-test mode, and explicit non-use of `gradle/mvn` execution.
  - Snippets: `orw-cli --apply --dir /workspace --out /out`
  - Tests: new integration tests under `tests/integration/migs/orw_cli_test.go` — expect recipe application and report contract output with no build-tool task invocation.

## Phase 3: Wire Build/Publish Tooling To `orw-cli`
- [x] Update image build and local registry tooling to publish `orw-cli` only — make release pipeline consistent with new runtime.
  - Repository: ploy
  - Component: image build scripts
  - Scope: update `deploy/images/build-and-push-migs.sh` and `deploy/images/garage.sh` to add `orw-cli` mapping and remove `orw-maven`/`orw-gradle` mappings; adjust directory resolution logic for new image path.
  - Snippets: `orw-cli -> orw-cli` (registry repository name)
  - Tests: script smoke runs in local env (`build`, `tag`, `push` dry checks) — expect no references to removed ORW images.

## Phase 4: Switch Java Stack Image Mapping To `orw-cli`
- [x] Route both Java stacks (`java-maven`, `java-gradle`) to `orw-cli` — enforce one execution engine across Java repos.
  - Repository: ploy
  - Component: workflow config samples, stack-aware image tests
  - Scope: update stack-aware image examples and fixtures in `docs/schemas/mig.example.yaml`, `docs/migs-lifecycle.md`, `tests/e2e/migs/scenario-stack-aware-images/*.yaml`, and contract tests in `internal/workflow/contracts/mod_image_test.go` and nodeagent tests expecting legacy image names.
  - Snippets: `image: { java-maven: <registry>/orw-cli:latest, java-gradle: <registry>/orw-cli:latest }`
  - Tests: `go test ./internal/workflow/contracts ./internal/nodeagent -run 'Test.*Image|Test.*Stack'` — expect stack resolution unchanged, image names updated.

## Phase 5: Update Runtime Consumption Of ORW Reports
- [x] Consume richer `report.json` contract in node execution path — surface deterministic failure kinds from `orw-cli`.
  - Repository: ploy
  - Component: nodeagent execution and status upload
  - Scope: update ORW result parsing/upload logic in `internal/nodeagent/execution_orchestrator_jobs.go` and related types to propagate `error_kind/reason` from `report.json` into job metadata/stats when present.
  - Snippets: map `error_kind=unsupported` into terminal run metadata without fallback execution path.
  - Tests: `go test ./internal/nodeagent -run 'Test.*Execute.*|Test.*Status.*|Test.*OutDir.*'` — expect preserved artifact upload plus enriched failure metadata.

## Phase 6: Replace Integration And E2E ORW Coverage
- [x] Replace plugin-specific tests with `orw-cli` tests — validate isolation guarantees and new runtime contract.
  - Repository: ploy
  - Component: integration/e2e tests
  - Scope: replace `tests/integration/migs/orw_maven_test.go` and `tests/integration/migs/orw_gradle_test.go` with `orw_cli_test.go`; update `tests/e2e/migs/README.md` and scenarios to build/use `orw-cli` image and assert no plugin-task coupling assumptions.
  - Snippets: add assertion that logs do not include `rewriteRun`/`rewrite-maven-plugin:run` command paths.
  - Tests: `go test ./tests/integration/migs -run 'TestOrwCLI'` and selected e2e scenarios — expect recipe application and deterministic unsupported behavior where attribution is unavailable.

## Phase 7: Remove Legacy ORW Images And References
- [x] Delete old plugin-based ORW runtime assets and stale references — complete non-backward-compatible replacement.
  - Repository: ploy
  - Component: image sources, docs, tests
  - Scope: remove `deploy/images/migs/orw-maven/` and `deploy/images/migs/orw-gradle/`; remove references from docs/tests/scripts (`docs/how-to/publish-migs.md`, `docs/envs/README.md`, `docs/migs-lifecycle.md`, e2e fixtures) and ensure no runtime path depends on legacy names.
  - Snippets: `rg -n "migs-orw-(maven|gradle)|orw-(maven|gradle)"` should only match changelog/history after cleanup.
  - Tests: repository-wide grep guard + targeted test updates — expect zero active references to removed images.

## Phase 8: Documentation And API Contract Sync
- [ ] Sync user-facing docs and API behavior notes with `orw-cli` rollout — make operational guidance match implementation.
  - Repository: ploy
  - Component: docs/api/docs/envs
  - Scope: update `docs/envs/README.md` with `orw-cli` env contract; update `docs/migs-lifecycle.md` and `docs/how-to/publish-migs.md` examples; add/refresh failure taxonomy text where run/job reporting surfaces unsupported attribution.
  - Snippets: ORW section title becomes `ORW image (orw-cli)`.
  - Tests: docs guard + schema checks — expect no stale runtime names and consistent examples.

## Phase 9: End-To-End Validation And Cutover
- [ ] Run full validation matrix and ship one-cut replacement — enforce no fallback and no compatibility mode.
  - Repository: ploy
  - Component: CI and local validation workflow
  - Scope: run unit/integration/e2e checks for workflow, nodeagent, server handlers, contracts, and docs; verify `orw-cli` handles Java Maven and Java Gradle repos through same runtime and old images are absent from build/publish flow.
  - Snippets: local check sequence: `make test`, `make vet`, `make staticcheck`.
  - Tests: full matrix above — expect green checks and successful runs with `job_image=.../orw-cli:...` only.
