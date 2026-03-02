# Gate Stack/Profile Refactor

## Goal

Make gate profile state strict and non-redundant.

- Separate repository identity from mig membership.
- Make gate profile identity `repo_sha + stack`.
- Remove profile payload storage from `mig_repos`.
- Keep one explicit profile resolution path for every gate run.

## Current Implementation (As-Is)

### Data model

- `mig_repos` stores `repo_url`.
- `mig_repos` stores `gate_profile` (JSONB).
- `mig_repos` stores `gate_profile_artifacts` (JSONB).
- `mig_repos` stores `gate_profile_updated_at`.
- `run_repos.repo_id` references `mig_repos.id`.
- `jobs.repo_id` references `mig_repos.id`.
- There is no `repos` table.
- There is no `stacks` table.
- There is no `gate_profiles` table.
- There is no `gates` table.
- There is no persisted `repo_sha` on `run_repos` or `jobs`.

Code paths:

- Schema: `internal/store/schema.sql`
- Queries: `internal/store/queries/mig_repos.sql`, `internal/store/queries/run_repos.sql`, `internal/store/queries/jobs.sql`

### Gate profile lifecycle

- Claim-time injection maps persisted `mig_repos.gate_profile` into `build_gate.<phase>.gate_profile`.
- `pre_gate` auto-generates a profile when repo profile is missing and no explicit `build_gate.pre.gate_profile` exists.
- On `pre_gate` success, generated profile is persisted into `mig_repos.gate_profile` (only if NULL).
- On successful `re_gate`, valid infra candidate is persisted into `mig_repos.gate_profile`.

Code paths:

- Claim mutation: `internal/server/handlers/claim_spec_mutator.go`, `internal/server/handlers/nodes_claim.go`
- Auto-bootstrap generation: `internal/nodeagent/execution_orchestrator_gate.go`, `internal/workflow/step/gate_docker_stack_gate.go`
- Promotion: `internal/server/handlers/jobs_complete.go`

### Stack/image source

- Stack image mapping is runtime-only from `etc/ploy/gates/build-gate-images.yaml`.
- File is copied into images as `/etc/ploy/gates/build-gate-images.yaml`.
- Mappings are not persisted in DB.

Code paths:

- Resolver: `internal/workflow/step/build_gate_image_resolver.go`
- Runtime gate executor: `internal/workflow/step/gate_docker.go`
- Docker image copy: `deploy/images/server/Dockerfile`, `deploy/images/node/Dockerfile`

### Gaps vs requested flow

- Repo identity is mig-scoped (`mig_repos.id`), not global.
- Gate profiles are attached to mig repo rows, not to `(repo_sha, stack)`.
- No strict default profile rows per stack in DB.
- No gate execution linkage table (`job_id -> profile_id`).
- No `build_gate.<phase>.target` or `always`.
- No skip-on-prior-success logic by `repo_sha`.

## Target Model

### Tables

### `repos`

- `id`
- `url` canonical normalized URL
- `at` created timestamp
- Unique index on `url`

### `mig_repos` rewrite

- Keep mig membership and refs.
- Replace `repo_url` with `repo_id -> repos.id`.
- Drop `gate_profile*` fields.
- Add unique `(mig_id, repo_id)`.

### `run_repos` rewrite

- `repo_id -> repos.id`
- Keep `mig_id`, refs, status/attempt.
- Add `source_commit_sha` as immutable captured commit for this run-repo.
- Add `repo_sha0` as initial repo state SHA for this run-repo.
- Add FK `(mig_id, repo_id) -> mig_repos(mig_id, repo_id)` to keep membership consistency.

### `jobs` rewrite

- `repo_id -> repos.id`
- Add `repo_sha_in` and `repo_sha_out`.
- `repo_sha_in` is the state before job execution and includes all prior patches.
- `repo_sha_out` is the state after job execution.
- Add `repo_sha_in8` and `repo_sha_out8` as derived short forms.
- SHA format for `repo_sha*`: lowercase 40-hex Git commit-compatible hash string.

### `stacks`

- `id`
- `lang`
- `release`
- `tool`
- `image`
- Populate from `gates/stacks.yaml`.
- Unique `(lang, release, tool)`.

### Stacks catalog and profile files

- Replace `etc/ploy/gates/build-gate-images.yaml` with `gates/stacks.yaml`.
- Each stack entry in `gates/stacks.yaml` must include:
- stack selectors (`lang`, `release`, optional `tool`)
- runtime image
- `profile` path to canonical profile YAML
- Generate default profile files to:
- `gates/profiles/{lang}-{release}{-tool}.yaml`
- Generated profile path must be the value referenced by `profile` in `gates/stacks.yaml`.

### `gate_profiles`

- `id`
- nullable `repo_id -> repos.id`
- nullable `repo_sha`
- nullable `repo_sha8`
- `url` (garage object key/path)
- `stack_id -> stacks.id`
- `created_at`
- `updated_at`
- Unique `(repo_id, repo_sha, stack_id)`.
- Rows with `repo_id IS NULL` and `repo_sha IS NULL` are stack defaults.

### `gates`

- `job_id -> jobs.id`
- `profile_id -> gate_profiles.id`
- Unique `job_id`.

## Runtime Behavior

### SHA lifecycle

- On run start, server resolves remote HEAD/ref to `source_commit_sha` and pins it on `run_repos`.
- `repo_sha0 = source_commit_sha` and is the chain seed.
- All jobs in this run-repo must clone/checkout `source_commit_sha` (not moving branch refs).
- `pre_gate.repo_sha_in = run_repos.repo_sha0`.
- Node computes deterministic `repo_sha_out` with `repo_sha_v1` algorithm:
- Create temporary Git index and stage workspace snapshot (`git add -A`) with policy excludes.
- Compute snapshot tree object.
- If tree equals `repo_sha_in^{tree}`, then `repo_sha_out = repo_sha_in`.
- Else compute synthetic commit hash from `(parent=repo_sha_in, tree=snapshot_tree, fixed author/committer/date/message)`.
- Synthetic hash is calculated without moving refs.
- On job completion, node sends `repo_sha_out`.
- Server updates SHA chain in one DB transaction:
- current `jobs.repo_sha_out = reported sha`
- `next_job.repo_sha_in = current_job.repo_sha_out` when `next_id` exists

Strictness:

- Gate profile lookup uses `job.repo_sha_in`.
- If `repo_sha0` is unavailable, do not queue `pre_gate` for that repo.

### Build gate schema extensions

Add fields to `build_gate.pre` and `build_gate.post`:

- `target: all_tests|unit|build`
- `always: true`

Semantics:

- If `target` is set, infra healing must keep this target. No target hopping.
- If healing cannot support target, produce `unsupported` and stop run.
- If `always` is not set, existing successful profile can skip this gate.

### Profile lookup algorithm

Inputs:

- `repo_id`
- `repo_sha` from `job.repo_sha_in`
- `stack_id`
- phase config (`target`, `always`)

Lookup order:

1. Exact: `gate_profiles` by `(repo_id, repo_sha, stack_id)`.
2. Fallback: latest `(repo_id, stack_id)` profile.
3. Fallback: default profile `(NULL repo_id, NULL repo_sha, stack_id)`.

Resolution rules:

- If exact exists and phase does not set `always: true`, skip when required target succeeded.
- If exact exists and phase does not set `always: true`, skip when any other target succeeded.
- For fallback steps 2 and 3:
- Copy referenced garage profile object to a new exact object for `(repo_id, repo_sha, stack_id)`.
- Insert new exact `gate_profiles` row.
- Continue with resolved exact profile id.

Result:

- Every gate run resolves to one exact profile row for `(repo_id, repo_sha, stack_id)`.
- `gates` row links `job_id` and `profile_id`.

### Profile updates

- On gate success, update exact `gate_profiles` row (payload in garage + `updated_at`).
- During infra healing, candidate changes are stored as healing artifacts only.
- Infra healing does not update `gate_profiles`.
- Candidate is applied only through successful follow-up gate.

### Default profiles bootstrap

- Remove pre-gate auto-generator as runtime fallback.
- Generate default profiles from `gates/stacks.yaml` to `gates/profiles/` at deploy/build time.
- On deploy, upload defaults to garage.
- On deploy, upsert `stacks` and default `gate_profiles` rows.
- Server stops using `etc/ploy/gates/build-gate-images.yaml`.
- Server image does not need this file anymore.

## Required Code Changes

### Store and schema

- `internal/store/schema.sql`
- `internal/store/queries/mig_repos.sql`
- `internal/store/queries/run_repos.sql`
- `internal/store/queries/jobs.sql`
- New queries for `repos`, `stacks`, `gate_profiles`, `gates`.
- `sqlc.yaml` type overrides for new IDs and updated `repo_id` mappings.
- `internal/store/models.go` and generated sqlc outputs.

### Domain types

- Introduce `RepoID` (global repo identity), separate from `MigRepoID`.
- Update all APIs and structs that currently use `MigRepoID` where global repo identity is intended.

Key files:

- `internal/domain/types/ids.go`
- `internal/nodeagent/handlers.go`
- `internal/server/handlers/*.go` using `repo_id` path params/payloads

### Claim/scheduler/runtime flow

- Replace `GetMigRepo(job.RepoID)` assumptions in claim path.
- Resolve repo URL via `(run.mig_id, repo_id)` from `mig_repos + repos`.
- Resolve and persist `run_repos.repo_sha0` before job chain starts.
- Set `pre_gate.repo_sha_in = run_repos.repo_sha0`.
- Add profile resolution service that executes lookup/copy/insert rules.
- Write `gates` link on gate job claim/start.
- Accept `repo_sha_out` on job completion payload.
- Propagate `repo_sha_out -> next_job.repo_sha_in` transactionally.
- Add gate-skip outcome path and chain progression.

Key files:

- `internal/server/handlers/nodes_claim.go`
- `internal/server/handlers/claim_spec_mutator.go`
- `internal/server/handlers/jobs_complete.go`
- `internal/server/handlers/nodes_complete_healing*.go`
- `internal/server/handlers/migs_ticket.go`
- `internal/server/handlers/runs_batch_scheduler.go`

### Build gate contracts

- Extend `BuildGatePhaseConfig` with `target` and `always`.
- Add parsing and validation.
- Enforce target locking in infra healing path.
- Define `repo_sha_v1` contract and validation for node-reported `repo_sha_out`.
- Replace image/profile source contract from `etc/ploy/gates/build-gate-images.yaml` to `gates/stacks.yaml`.

Key files:

- `internal/workflow/contracts/build_gate_config.go`
- `internal/workflow/contracts/mods_spec.go`
- `internal/workflow/contracts/mods_spec_parse.go`
- `internal/workflow/contracts/gate_profile.go`

### Deploy/bootstrap

- Add stacks/default-profiles seeding on deploy startup.
- Add profile generation source under `gates/stacks.yaml` and `gates/profiles/`.
- Stop shipping/reading `etc/ploy/gates/build-gate-images.yaml` on server.

Key files:

- `deploy/images/server/Dockerfile`
- `deploy/images/node/Dockerfile`
- deploy/bootstrap flow under `internal/deploy/*` and local deploy scripts

### API/docs

- Update repo-centric endpoints and schema docs to global `repos.id`.
- Document new gate skip result and gate/profile linkage behavior.

Key files:

- `docs/api/components/schemas/controlplane.yaml`
- `docs/api/paths/repos.yaml`
- `docs/build-gate/README.md`
- `docs/migs-lifecycle.md`
- `design/gate-profile*.md` (or consolidate into this design)

## Implementation Sequence

1. Add schema tables and new FKs (`repos`, `stacks`, `gate_profiles`, `gates`), rewrite `mig_repos/run_repos/jobs`.
2. Introduce global `RepoID` types and update sqlc mappings.
3. Implement `repo_sha0` resolution and persist it on `run_repos`.
4. Add `repo_sha_in/repo_sha_out` completion propagation in job chain.
5. Implement stack/default-profile seeding from `gates/stacks.yaml` + `gates/profiles/`.
6. Implement profile resolution service and `gates` linkage.
7. Add skip logic with `always`/`target`.
8. Remove pre-gate auto-bootstrap fallback.
9. Enforce infra healing target locking.
10. Update API handlers, OpenAPI docs, and tests.

## Validation Plan

Unit/integration coverage to add:

- Store tests for new FK/uniqueness constraints and lookup ordering.
- Store tests for `repo_sha0`, `repo_sha_in`, and `repo_sha_out` propagation.
- Contract tests for `repo_sha_v1` fixtures (same input state -> same sha, unchanged state -> sha_out=sha_in).
- Resolver tests for exact/fallback/default lookup and copy-on-resolve behavior.
- Gate skip test: required target success -> skip.
- Gate skip test: any target success -> skip.
- Gate skip test: `always: true` -> no skip.
- Healing tests for `target` lock and `unsupported` termination.
- Claim tests for `gates.job_id -> profile_id` linkage.
- Completion tests for atomic `repo_sha_out` write + `next.repo_sha_in` update.
- Deploy seeding tests from `gates/stacks.yaml` and referenced profile files.

Commands:

- `make test`
- `make vet`
- `make staticcheck`

## Decisions

- Use GitLab API HEAD/ref SHA as primary `run_repos.source_commit_sha` source.
- Use VCS `ls-remote` as fallback for non-GitLab or API-unavailable repos.
- Pin `source_commit_sha` for the whole run-repo lifecycle.
- Use `repo_sha_v1` deterministic synthetic commit hashing for `jobs.repo_sha_out`.
- Use server-side transactional propagation to set successor `jobs.repo_sha_in`.
- Use `jobs.repo_sha_in` as the canonical gate profile lookup SHA.
