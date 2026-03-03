# SBOM Artifact Persistence And Compatibility Lookup

Scope: implement SBOM flow from gate `/out/*` artifacts through persistence and compatibility lookup for `deps` healing, as defined in `design/sboms.md`.

Documentation: `AGENTS.md`; `design/sboms.md`; `design/bumps.md`; `docs/build-gate/README.md`; `docs/migs-lifecycle.md`; `docs/envs/README.md`; `internal/store/schema.sql`; `internal/store/queries/*.sql`; `internal/nodeagent/execution_orchestrator_gate.go`; `internal/server/handlers/jobs_complete.go`; `internal/server/handlers/nodes_claim.go`

Legend: [ ] todo, [x] done.

## Phase 1: Preconditions And Contract Lock
- [x] Lock runtime/test preconditions and API contract before implementation.
  - Repository: `ploy`
  - Component: docs + handler contract boundaries
  - Scope: confirm required envs for local tests are documented in `docs/envs/README.md`; if any are missing, add TODO markers; freeze `/v1/sboms/compat` request/response contract and successful-gate-only ingestion rule in docs.
  - Snippets: `GET /v1/sboms/compat?lang=<lang>&release=<release>&tool=<tool>&libs=<name>:<ver>,<name>`
  - Tests: `go test ./docs/...` and docs guard tests — expect docs consistency and no missing env documentation references.

## Phase 2: Gate `/out/*` Artifact Persistence
- [x] Persist all gate `/out/*` files as first-class artifacts with deterministic `out/` archive paths.
  - Repository: `ploy`
  - Component: `internal/nodeagent`
  - Scope: update gate artifact upload path handling to archive every `/out/*` file (not only tool-specific paths), preserving relative path fidelity for downstream SBOM discovery.
  - Snippets: archive key prefix `out/` for uploaded gate artifacts.
  - Tests: `go test ./internal/nodeagent -run 'Test.*Gate.*Artifact|Test.*Out.*Upload'` — expect complete `/out/*` upload and stable archive paths.

## Phase 3: SBOM Discovery And Parsing Pipeline
- [ ] Add SBOM detector/parser pipeline over uploaded gate artifacts.
  - Repository: `ploy`
  - Component: `internal/nodeagent` + `internal/server/handlers` (or shared parser package)
  - Scope: detect supported SBOM files in gate artifacts, parse formats, flatten package entries into normalized `(lib, ver)` tuples, and attach provenance (`job_id`, `repo_id`).
  - Snippets: flattened row shape `{job_id, repo_id, lib, ver}`.
  - Tests: `go test ./internal/nodeagent -run 'Test.*SBOM.*Detect|Test.*SBOM.*Parse'` and parser unit tests — expect normalized rows and graceful handling of unsupported files.

## Phase 4: SBOM Storage Model And Write Path
- [ ] Introduce `sboms(job_id, repo_id, lib, ver)` persistence and write queries.
  - Repository: `ploy`
  - Component: `internal/store`
  - Scope: extend `internal/store/schema.sql` with `sboms` table, add sqlc queries for insert/select operations, and ensure stack/time are derived via joins (no duplicated stack/time columns in `sboms`).
  - Snippets: join path `sboms.job_id -> gates.job_id -> gate_profiles.stack_id -> stacks`.
  - Tests: `go test ./internal/store -run 'Test.*SBOM.*(Insert|Query|Constraint)'` — expect valid writes, stable reads, and correct FK/uniqueness behavior.

## Phase 5: Successful-Gate-Only Ingestion Wiring
- [ ] Persist SBOM rows only for successful `pre_gate`, `post_gate`, `re_gate` jobs.
  - Repository: `ploy`
  - Component: `internal/server/handlers`
  - Scope: wire ingestion into gate completion flow; guard by job type/status so failed/canceled gates do not write SBOM rows.
  - Snippets: allowlist `job_type in (pre_gate, post_gate, re_gate)` and `jobs.status='Success'`.
  - Tests: `go test ./internal/server/handlers -run 'Test.*Gate.*SBOM.*Success|Test.*Gate.*SBOM.*NoPersistOnFailure'` — expect writes only on successful gate completions.

## Phase 6: Compatibility API (`/v1/sboms/compat`)
- [ ] Implement compatibility lookup endpoint backed by successful SBOM evidence.
  - Repository: `ploy`
  - Component: `internal/server/handlers` + `internal/store/queries`
  - Scope: add handler and queries for stack-filtered lookup by `lang/release/tool`; for each requested lib return minimum successful version, or minimum successful version `>=` requested floor when `name:ver` is provided; return `null` when no stack evidence exists.
  - Snippets: response payload `{ "<lib>": "<ver>", ... } | null`.
  - Tests: `go test ./internal/server/handlers -run 'Test.*SBOM.*Compat'` and store query tests — expect correct per-lib results, floor filtering, and null response semantics.

## Phase 7: Ecosystem-Aware Version Ordering
- [ ] Implement non-lexical version ordering per ecosystem for compatibility floor queries.
  - Repository: `ploy`
  - Component: version comparator package + compat query integration
  - Scope: introduce ecosystem-aware version comparator(s) used by `/v1/sboms/compat` so `>=` filtering and minimum selection are semantically correct for each supported stack ecosystem.
  - Snippets: comparator interface keyed by stack ecosystem metadata.
  - Tests: `go test ./internal/... -run 'Test.*Version.*Compare|Test.*Compat.*Floor'` — expect correct ordering for semantic versions and non-semver edge cases per supported ecosystems.

## Phase 8: `deps` Healing Integration And Documentation Sync
- [ ] Wire compatibility endpoint exposure to `deps` healing claims and synchronize docs.
  - Repository: `ploy`
  - Component: `internal/server/handlers/nodes_claim.go`; nodeagent recovery input hydration; docs
  - Scope: include prefilled compatibility endpoint in `deps` recovery context, ensure healing receives prior `deps_bumps` from metadata, and update docs to reflect final SBOM + compatibility behavior.
  - Snippets: recovery context field `deps_compat_endpoint`.
  - Tests: `go test ./internal/server/handlers -run 'Test.*Claim.*Deps.*Compat'` and `go test ./internal/nodeagent -run 'Test.*Deps.*Compat.*Hydration'` — expect claim payload and hydrated `/in` files to match contract.

## Phase 9: Validation Matrix
- [ ] Run full validation for changed modules and hygiene checks.
  - Repository: `ploy`
  - Component: nodeagent + handlers + store + docs
  - Scope: execute unit tests for touched packages and project hygiene commands.
  - Snippets: `make test`, `make vet`, `make staticcheck`.
  - Tests: all commands above complete successfully.
