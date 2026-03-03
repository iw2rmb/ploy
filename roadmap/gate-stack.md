# Gate Stack/Profile Refactor Rollout

Scope: Implement strict gate profile identity by `repo_sha + stack`, split global repository identity from mig membership, replace runtime gate image/profile source with `gates/stacks.yaml` + generated `gates/profiles/*.yaml`, and remove legacy `mig_repos.gate_profile*` persistence.

Documentation: `AGENTS.md`; `design/gate-stack.md`; `internal/store/schema.sql`; `internal/store/queries/*.sql`; `internal/server/handlers/nodes_claim.go`; `internal/server/handlers/jobs_complete.go`; `internal/server/handlers/migs_runs.go`; `internal/server/handlers/runs_batch_scheduler.go`; `internal/nodeagent/execution_orchestrator_gate.go`; `internal/workflow/contracts/build_gate_config.go`; `internal/workflow/step/build_gate_image_resolver.go`; `deploy/images/server/Dockerfile`; `deploy/images/node/Dockerfile`; `docs/build-gate/README.md`; `docs/migs-lifecycle.md`; `docs/api/components/schemas/controlplane.yaml`; `docs/api/paths/repos.yaml`.

Legend: [ ] todo, [x] done.

## Phase 1: Stack Catalog And Profile Sources
- [x] Replace legacy stack image map with `gates/stacks.yaml` and profile references — make one canonical source for stack image + default profile path.
  - Repository: ploy
  - Component: gate catalog assets; build/deploy packaging
  - Scope: add `gates/stacks.yaml` schema with fields `lang`, `release`, `tool`, `image`, `profile`; generate/update `gates/profiles/{lang}-{release}{-tool}.yaml`; keep file paths deterministic and stable for CI.
  - Snippets: `gates/stacks.yaml` entry: `{lang: java, release: "17", tool: maven, image: "...", profile: "gates/profiles/java-17-maven.yaml"}`
  - Tests: `go test ./internal/workflow/step -run BuildGateImageResolver` and asset validation tests — expect all catalog entries parse and referenced profile files exist.
- [x] Remove server dependence on `etc/ploy/gates/build-gate-images.yaml` — eliminate redundant server-local mapping file.
  - Repository: ploy
  - Component: server image packaging; resolver defaults
  - Scope: stop copying/reading legacy mapping file in server image/runtime; keep node/runtime behavior wired to new catalog-driven bootstrap data.
  - Snippets: remove server Docker `COPY ... build-gate-images.yaml ...`
  - Tests: image build + startup smoke in local deploy — expect server boot with no reads of `/etc/ploy/gates/build-gate-images.yaml`.

## Phase 2: Storage Schema Refactor
- [x] Add new canonical tables: `repos`, `stacks`, `gate_profiles`, `gates` — establish normalized identities and execution links.
  - Repository: ploy
  - Component: store schema and constraints
  - Scope: update `internal/store/schema.sql` with new tables/FKs/uniques; enforce `gate_profiles(repo_id, repo_sha, stack_id)` uniqueness; enforce `gates(job_id)` uniqueness.
  - Snippets: `UNIQUE (repo_id, repo_sha, stack_id)`, `FOREIGN KEY (profile_id) REFERENCES gate_profiles(id)`
  - Tests: schema constraint tests under `internal/store/*constraints*` — expect uniqueness/FK violations on invalid inserts.
- [x] Rewrite repo foreign keys across execution tables — move from mig-scoped repo IDs to global repo IDs.
  - Repository: ploy
  - Component: execution persistence model
  - Scope: rewrite `mig_repos` (`repo_url` -> `repo_id`, drop `gate_profile*`), `run_repos.repo_id -> repos.id`, `jobs.repo_id -> repos.id`; keep membership consistency with `(mig_id, repo_id)` FK.
  - Snippets: `FOREIGN KEY (mig_id, repo_id) REFERENCES mig_repos(mig_id, repo_id)`
  - Tests: `go test ./internal/store -run MigRepo|RunRepo|Job` — expect creation/listing paths to preserve run/mig scoping.

## Phase 3: SHA Chain Persistence
- [x] Add run-repo seed SHA fields and pinning model — make run input commit immutable.
  - Repository: ploy
  - Component: run creation and scheduling
  - Scope: add `run_repos.source_commit_sha`, `run_repos.repo_sha0`; set them at run start; fail repo queueing when seed SHA cannot be resolved.
  - Snippets: `repo_sha0 = source_commit_sha`
  - Tests: handler tests in `internal/server/handlers/migs_runs_test.go` and `runs_submit_test.go` — expect queue rejection when SHA seed resolution fails.
- [x] Add per-job SHA in/out fields and short forms — persist deterministic state transitions.
  - Repository: ploy
  - Component: jobs schema + completion handling
  - Scope: add `jobs.repo_sha_in`, `jobs.repo_sha_out`, `jobs.repo_sha_in8`, `jobs.repo_sha_out8`; enforce lowercase 40-hex format validation at boundary.
  - Snippets: regex validation `^[0-9a-f]{40}$`
  - Tests: completion payload tests in `internal/server/handlers/jobs_complete*_test.go` — expect invalid SHA format rejection.
- [x] Implement atomic propagation `repo_sha_out -> next.repo_sha_in` — keep chain consistent under concurrency.
  - Repository: ploy
  - Component: jobs completion transaction
  - Scope: update completion query/logic to write current `repo_sha_out` and successor `repo_sha_in` in one DB transaction keyed by `next_id`.
  - Snippets: CTE update pattern with `target_job` + `next_job`
  - Tests: transactional tests in handler/store integration — expect no partial updates on injected failures.

## Phase 4: sqlc And Domain Type Migration
- [x] Introduce global `RepoID` domain type and remap sqlc columns — remove ambiguity between mig membership and global repo identity.
  - Repository: ploy
  - Component: domain types; sqlc overrides; generated models
  - Scope: add `RepoID` type in `internal/domain/types/ids.go`; update `sqlc.yaml` overrides for `repos.id`, `jobs.repo_id`, `run_repos.repo_id`, `mig_repos.repo_id`; regenerate store code.
  - Snippets: `- column: "jobs.repo_id" -> types.RepoID`
  - Tests: `go test ./internal/domain/types ./internal/store` — expect compile-time type safety and query mapping correctness.

## Phase 5: Runtime SHA Contract (`repo_sha_v1`)
- [x] Implement deterministic node SHA computation contract — make `repo_sha_out` reproducible.
  - Repository: ploy
  - Component: nodeagent execution + git helpers
  - Scope: implement `repo_sha_v1` algorithm (snapshot tree + synthetic commit hash with fixed metadata, no ref mutation); report `repo_sha_out` in completion payload.
  - Snippets: unchanged-tree fast path returns `repo_sha_in`
  - Tests: contract fixtures in nodeagent tests — expect same workspace => same sha; unchanged workspace => `sha_out == sha_in`.
- [x] Define server-side SHA acceptance policy — trust but verify node-reported hashes.
  - Repository: ploy
  - Component: completion validation
  - Scope: validate format and chain consistency before persistence; reject missing `repo_sha_out` for jobs that must advance chain.
  - Snippets: reject `repo_sha_out` when job succeeded and `next_id` exists.
  - Tests: jobs completion negative tests — expect 4xx on malformed or missing SHA data.

## Phase 6: Gate Profile Resolution Service
- [x] Implement lookup/copy/insert profile resolution pipeline — always resolve to exact `(repo_id, repo_sha_in, stack_id)` row.
  - Repository: ploy
  - Component: gate claim-time orchestration
  - Scope: add service for 3-step lookup order (exact -> repo+stack -> default stack); copy garage object on fallback; insert exact row; return `profile_id`.
  - Snippets: fallback copy inserts new `gate_profiles` row with concrete `repo_id` + `repo_sha`.
  - Tests: service tests for all lookup branches and object-copy behavior.
- [x] Persist job-to-profile mapping in `gates` — make gate/profile relation auditable.
  - Repository: ploy
  - Component: claim and completion persistence
  - Scope: insert `gates(job_id, profile_id)` at gate job claim/start; enforce one row per gate job.
  - Snippets: idempotent insert on retries/reclaims.
  - Tests: claim handler tests — expect `gates` row creation for pre/post/re-gate.

## Phase 7: Build Gate Contract Changes
- [x] Add `build_gate.[pre|post].target` and `always` fields — expose strict skip/target behavior in spec.
  - Repository: ploy
  - Component: workflow contracts parser/validator
  - Scope: extend `BuildGatePhaseConfig`; parse/validate enums and booleans; wire through typed options and claim mutation.
  - Snippets: `target: all_tests|unit|build`, `always: true`
  - Tests: `go test ./internal/workflow/contracts -run BuildGate` — expect strict validation errors for invalid target values.
- [x] Implement gate skip logic and infra target locking — enforce deterministic healing behavior.
  - Repository: ploy
  - Component: claim mutation + healing orchestration
  - Scope: skip gate when prior exact profile qualifies and `always` is not set; during infra healing, prohibit target hopping when target is pinned; return/record `unsupported` and stop run when lock cannot be healed.
  - Snippets: skip result metadata includes source profile id and matched target.
  - Tests: handler tests for skip and unsupported paths; healing tests for target-lock enforcement.

## Phase 8: Remove Legacy Auto-Bootstrap Profile Persistence
- [x] Delete pre-gate runtime auto-generator fallback path — defaults must come from catalog-generated profiles and DB bootstrap.
  - Repository: ploy
  - Component: nodeagent gate execution; server promotion logic
  - Scope: remove auto-generated pre-gate profile creation/promotion (`generated_gate_profile` bootstrap flow); keep infra candidate artifact flow for healing only.
  - Snippets: remove `AutoBootstrapRepoGateProfile` wiring and promotion query usage.
  - Tests: update gate tests to assert no auto-bootstrap persistence behavior remains.

## Phase 9: Deploy Seeding Pipeline
- [x] Seed `stacks` and default `gate_profiles` from `gates/stacks.yaml` on deploy/startup — make defaults explicit and reproducible.
  - Repository: ploy
  - Component: deploy bootstrap and server startup tasks
  - Scope: parse catalog, verify profile file presence, upload profile objects to garage, upsert DB rows for stacks/default profiles.
  - Snippets: startup idempotency key `(lang, release, tool)` for stacks and `(NULL repo_id, NULL repo_sha, stack_id)` for defaults.
  - Tests: local deploy smoke + idempotent reseed tests — expect no duplicate rows and stable object paths.

## Phase 10: API, Docs, And Regression Matrix
- [ ] Update API surfaces and docs to global repos model and SHA chain fields — align contracts with new persistence model.
  - Repository: ploy
  - Component: handlers + OpenAPI + docs
  - Scope: update repo-centric endpoints/types (`RepoID`), gate execution status payloads, build-gate docs, lifecycle docs, and schema docs.
  - Snippets: controlplane schema includes `repo_sha_in`, `repo_sha_out` where relevant.
  - Tests: `go test ./docs/api` and handler contract tests — expect schema and API behavior consistency.
- [ ] Run full validation matrix and clean legacy references — ensure no runtime path still depends on `etc/ploy/gates/build-gate-images.yaml`.
  - Repository: ploy
  - Component: CI/hygiene/test suites
  - Scope: run unit, vet, staticcheck, integration slices; remove stale docs/tests referring to legacy mapping file.
  - Snippets: `rg -n "build-gate-images.yaml" internal docs deploy` should only match migration/history references after rollout.
  - Tests: `make test`, `make vet`, `make staticcheck` — expect green across store/server/node/workflow modules.
