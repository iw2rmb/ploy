# TmpDir Bundle Upload And Extraction

Scope: replace inline `tmp_dir` file payloads with bundle references by archiving user-provided files/directories, uploading one bundle per block, and downloading/unpacking on node execution.

Documentation: `AGENTS.md`; `docs/envs/README.md`; `docs/schemas/mig.example.yaml`; `cmd/ploy/README.md`; `docs/migs-lifecycle.md`; `internal/workflow/contracts/mods_spec.go`; `cmd/ploy/mig_run_spec.go`; `internal/server/handlers/register.go`; `internal/server/handlers/migs_spec.go`; `internal/server/handlers/runs_submit.go`; `internal/nodeagent/execution_orchestrator_jobs.go`; `internal/nodeagent/execution_orchestrator_router_runtime.go`; `internal/workflow/step/container_spec.go`.

- [x] 1.1 Lock Contract Boundary And Preconditions
  - Repository: `ploy`
  - Component: `internal/workflow/contracts/mods_spec.go`; `internal/workflow/contracts/build_gate_config.go`; `docs/envs/README.md`; `docs/schemas/mig.example.yaml`
  - Implementation:
    1. Replace canonical spec contract for tmp injection from per-file bytes to a single bundle reference object for each step/router/healing block.
    2. Define and validate bundle reference fields (`bundle_id`, `cid`, `digest`, `entries`) and reject legacy inline `tmp_dir[].content` payloads.
    3. Add/adjust docs for required env discoverability and mark missing variables as TODOs in `docs/envs/README.md` if discovered.
  - Verification:
    1. `go test ./internal/workflow/contracts -run 'Test.*Tmp.*'`
    2. `go test ./docs/...`
  - Reasoning: `medium`

- [ ] 1.2 Add Spec-Bundle Storage And API Surface
  - Repository: `ploy`
  - Component: `internal/store/schema.sql`; `internal/store/queries/*.sql`; `internal/server/handlers`; `internal/server/handlers/register.go`; `docs/api/OpenAPI.yaml`
  - Assumptions:
    - Bundle lifecycle is tied to spec usage, with GC based on unreferenced bundle rows.
    - A control-plane API is acceptable for CLI uploads; direct Garage credentials are not exposed to CLI.
  - Implementation:
    1. Add persistent metadata for uploaded spec bundles (`id`, `cid`, `digest`, `size`, `object_key`, `created_by`, `created_at`, `last_ref_at`).
    2. Implement control-plane upload endpoint for bundle bytes and worker download endpoint for bundle retrieval by `bundle_id`.
    3. Register new endpoints and OpenAPI contracts with explicit request size limits and auth roles.
  - Verification:
    1. `go test ./internal/store -run 'Test.*SpecBundle.*'`
    2. `go test ./internal/server/handlers -run 'Test.*SpecBundle.*'`
    3. `go test ./docs/api/...`
  - Reasoning: `xhigh`

- [ ] 1.3 Rewrite CLI TmpDir Preprocessing To Archive And Upload
  - Repository: `ploy`
  - Component: `cmd/ploy/mig_run_spec.go`; `cmd/ploy/run_submit.go`; `cmd/ploy/mig_add.go`; `cmd/ploy/mig_spec.go`; `internal/cli/migs/*`; `cmd/ploy/*tmpdir*_test.go`
  - Implementation:
    1. Resolve each user-facing `tmp_dir` source path as file-or-directory and build a deterministic archive with stable ordering.
    2. Upload the archive via the new control-plane API and capture returned bundle metadata.
    3. Replace each tmp block in outgoing spec payload with one bundle reference and remove local `path` entries before canonical validation.
  - Verification:
    1. `go test ./cmd/ploy -run 'Test.*TmpDir.*|Test.*BuildSpecPayload.*'`
    2. `go test ./internal/cli/migs -run 'Test.*Spec.*'`
  - Reasoning: `medium`

- [ ] 1.4 Implement Node Download, Safe Unpack, And Mount Wiring
  - Repository: `ploy`
  - Component: `internal/nodeagent/execution_orchestrator_jobs.go`; `internal/nodeagent/execution_orchestrator_router_runtime.go`; `internal/nodeagent/uploaders.go`; `internal/workflow/step/container_spec.go`
  - Implementation:
    1. Download referenced bundle for each execution block and verify digest before extraction.
    2. Extract archives into tmp staging with traversal-safe rules (reject `..`, absolute paths, symlink/hardlink entries, and duplicate canonical paths).
    3. Mount extracted top-level files/directories read-only at `/tmp/<name>` without mounting `/tmp` itself read-only.
  - Verification:
    1. `go test ./internal/nodeagent -run 'Test.*Tmp.*Bundle.*|Test.*Tmp.*Materialization.*'`
    2. `go test ./internal/workflow/step -run 'Test.*ContainerSpec.*Tmp.*'`
  - Reasoning: `high`

- [ ] 1.5 Remove Legacy Inline TmpDir Path
  - Repository: `ploy`
  - Component: `internal/workflow/contracts/mods_spec_tmpdir_test.go`; `cmd/ploy/mig_run_spec_tmpdir_test.go`; `docs/envs/README.md`; `docs/schemas/mig.example.yaml`; `cmd/ploy/README.md`
  - Implementation:
    1. Delete legacy per-file inline tmp payload parsing/validation and corresponding tests.
    2. Rewrite user docs and schema examples to the bundle-upload model only.
    3. Remove outdated comments and references that describe `tmp_dir` as file-by-file `name/content`.
  - Verification:
    1. `go test ./internal/workflow/contracts ./cmd/ploy`
    2. `~/@iw2rmb/amata/scripts/check_docs_links.sh`
  - Reasoning: `medium`

- [ ] 1.6 End-To-End Validation And Hygiene
  - Repository: `ploy`
  - Component: `tests/e2e/migs/*`; `Makefile` workflows
  - Implementation:
    1. Add one E2E scenario with mixed file+directory tmp inputs and validate runtime visibility under `/tmp`.
    2. Add one negative E2E scenario for blocked archive entries (traversal/symlink) and assert deterministic failure messages.
    3. Run full project hygiene and unit test targets for touched modules.
  - Verification:
    1. `make test`
    2. `make vet`
    3. `make staticcheck`
  - Reasoning: `medium`
