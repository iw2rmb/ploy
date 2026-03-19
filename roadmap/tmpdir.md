# TmpDir Step File Injection

Scope: add spec-driven `tmpDir` support so each selected container step can receive caller-provided files mounted read-only as `/tmp/<filename>`.

Documentation: `AGENTS.md`; `docs/schemas/mig.example.yaml`; `docs/envs/README.md`; `cmd/ploy/mig_run_spec.go`; `internal/workflow/contracts/mods_spec.go`; `internal/workflow/contracts/build_gate_config.go`; `internal/nodeagent/run_options.go`; `internal/nodeagent/manifest.go`; `internal/nodeagent/execution_orchestrator_jobs.go`; `internal/workflow/step/container_spec.go`.

- [ ] 1.1 Define canonical `tmpDir` contract in shared workflow types.
  - Repository: `ploy`
    1. Add shared tmp file payload type in `internal/workflow/contracts` with explicit `name` and `content` fields.
    2. Add `tmpDir` field to `ModStep`, `HealingActionSpec`, and `RouterSpec` so step/heal/router use the same contract.
    3. Add validation helpers in `ModsSpec.Validate` to enforce non-empty names/content and reject duplicate destination names per container spec block.
    4. Extend `StepManifest` with tmp file payload and validation for container-runtime consumption.
  - Verification:
    1. `go test ./internal/workflow/contracts -run 'Test.*ModsSpec.*|Test.*BuildGate.*|Test.*StepManifest.*'`
    2. Add table-driven tests for invalid `tmpDir` payloads and duplicate names.
  - Reasoning: medium

- [ ] 1.2 Resolve `tmpDir` file paths in CLI spec preprocessing.
  - Repository: `ploy`
    1. Extend `cmd/ploy/mig_run_spec.go` to process `tmpDir` entries in `steps[]`, `build_gate.router`, and `build_gate.healing.by_error_kind.*`.
    2. Resolve each filepath using existing path rules (`~`, env expansion) and read file content on CLI side.
    3. Normalize each filepath entry into canonical tmp file payload before `ParseModsSpecJSON` validation.
    4. Keep error messages deterministic for invalid path type, missing files, and unreadable files.
  - Verification:
    1. `go test ./cmd/ploy -run 'Test.*Spec.*TmpDir|Test.*BuildSpecPayload.*TmpDir'`
    2. Add tests for mixed valid and invalid `tmpDir` entries in step/healing/router blocks.
  - Reasoning: medium

- [ ] 1.3 Thread `tmpDir` through nodeagent typed options and manifest builders.
  - Repository: `ploy`
    1. Extend `ModContainerSpec` and `StepMod` in `internal/nodeagent/run_options.go` with tmp file payload.
    2. Copy `tmpDir` from parsed `ModsSpec` in `modsSpecToRunOptions` for single-step, multi-step, healing, and router flows.
    3. Propagate tmp file payload into `StepManifest` in `buildManifestFromRequest`, `buildHealingManifest`, and `buildRouterManifest`.
  - Verification:
    1. `go test ./internal/nodeagent -run 'Test.*ParseSpec.*|Test.*ModsSpecToRunOptions.*|Test.*BuildManifest.*'`
    2. Add tests that assert `tmpDir` survives parse-to-manifest conversion for step/heal/router.
  - Reasoning: medium

- [ ] 1.4 Materialize and mount tmp files at runtime.
  - Repository: `ploy`
    1. Add execution helper in `internal/nodeagent/execution_orchestrator_jobs.go` to materialize manifest tmp files into a node-local temp directory per job run.
    2. Extend `step.Request` and `buildContainerSpec` plumbing to receive tmp file staging directory.
    3. Mount each staged file read-only to `/tmp/<name>` in `internal/workflow/step/container_spec.go` without replacing the full `/tmp` mount.
    4. Ensure temp staging cleanup is deterministic on success and failure paths.
  - Verification:
    1. `go test ./internal/workflow/step -run 'TestBuildContainerSpec_.*Tmp.*'`
    2. `go test ./internal/nodeagent -run 'Test.*Execute.*TmpDir|Test.*Cleanup.*Tmp.*'`
  - Reasoning: high

- [ ] 1.5 Update docs and perform validation pass.
  - Repository: `ploy`
    1. Update `docs/schemas/mig.example.yaml` with `tmpDir` examples for step/healing/router blocks.
    2. Update `docs/envs/README.md` with `tmpDir` behavior and CLI preprocessing boundaries.
    3. Cross-reference docs and run link-integrity check from repo root.
    4. Run focused package tests and final project hygiene commands for touched areas.
  - Verification:
    1. `~/@iw2rmb/amata/scripts/check_docs_links.sh`
    2. `go test ./cmd/ploy ./internal/workflow/contracts ./internal/nodeagent ./internal/workflow/step`
    3. `make test`
  - Reasoning: medium
