# Amata Step For Migs Healing

Scope: add a typed `amata` execution mode for Build Gate router/healing that runs `amata run /in/amata.yaml` with optional `--set` arguments when `amata.spec` is provided, keeps direct `codex exec` behavior when `amata.spec` is not provided, treats `CODEX_PROMPT` as optional for `amata.spec` mode and required for direct Codex mode, and uses a locally built `../amata` binary copied into the mig image.

Documentation: `AGENTS.md`; `docs/migs-lifecycle.md`; `docs/envs/README.md`; `docs/schemas/mig.example.yaml`; `deploy/images/migs/README.md`; `docs/how-to/publish-migs.md`; `tests/e2e/migs/README.md`; `deploy/images/migs/mig-codex/Dockerfile`; `deploy/images/migs/mig-codex/mig-codex.sh`; `deploy/images/build-and-push-migs.sh`; `deploy/images/garage.sh`; `deploy/vps/run.sh`; `internal/workflow/contracts/build_gate_config.go`; `internal/workflow/contracts/mods_spec.go`; `internal/workflow/contracts/mods_spec_parse.go`; `internal/nodeagent/run_options.go`; `internal/nodeagent/manifest.go`; `internal/nodeagent/execution_orchestrator_jobs.go`.

- [x] 1.1 Lock `amata` spec contract and edge cases for healing/router
  - Repository: `ploy`
    1. Add `contracts.AmataRunSpec` and `contracts.AmataSetParam` in `internal/workflow/contracts/build_gate_config.go` and embed them in `contracts.RouterSpec` and `contracts.HealingActionSpec`.
    2. Extend `contracts.ModsSpec.Validate` in `internal/workflow/contracts/mods_spec.go` to treat `amata` as optional, require non-empty `amata.spec` only when `amata` block is present, validate `amata.set[].param`, and preserve direct-Codex execution semantics when `amata.spec` is absent.
    3. Extend raw-field checks in `internal/workflow/contracts/mods_spec_parse.go` so forbidden/legacy fields stay rejected, nested `amata.spec` and `amata.set` are accepted only in router/healing nodes, and flat `spec`/`set` keys are rejected.
    4. Define deterministic command mapping rules: when `amata.spec` exists, materialize to `/in/amata.yaml` and emit ordered `amata.set` as `--set '<param>=<value>'` with no `CODEX_PROMPT` requirement; when `amata.spec` is absent, execute existing direct `codex exec` path with `CODEX_PROMPT` required.
  - Verification:
    1. `go test ./internal/workflow/contracts -run 'Test.*ModsSpec.*(Router|Healing|Amata|Parse|Validate)'`
    2. `go test ./internal/workflow/contracts -run 'Test.*TmpDir|Test.*Healing'`
  - Reasoning: high

- [ ] 1.2 Thread typed `amata` config into node manifests and execution
  - Repository: `ploy`
    1. Extend `nodeagent.ModContainerSpec` in `internal/nodeagent/run_options.go` with `Amata *contracts.AmataRunSpec` and map it in `modsSpecToRunOptions` for both `build_gate.router` and selected healing action.
    2. Add a command builder in `internal/nodeagent/manifest.go` that selects `amata run /in/amata.yaml` plus `--set` entries when `Amata.Spec` exists and selects existing direct `codex exec` path when `Amata.Spec` is absent.
    3. Ensure `/in/amata.yaml` is hydrated before execution in `internal/nodeagent/execution_orchestrator_jobs.go` only for `Amata.Spec` path by reusing existing in-dir preparation flow with explicit file target and deterministic overwrite.
    4. Keep router/healing metadata parsing unchanged (`/out/codex-last.txt`) so current recovery pipeline remains stable while moving execution entry to `amata`.
  - Verification:
    1. `go test ./internal/nodeagent -run 'Test.*(RunOptions|Manifest|Healing|Router|Amata)'`
    2. `go test ./internal/nodeagent -run 'Test.*TmpDir|Test.*Recovery'`
  - Reasoning: xhigh

- [ ] 1.3 Refactor `mig-codex` image to run `amata` and reuse existing image boundary
  - Repository: `ploy`
    1. Update `deploy/images/migs/mig-codex/mig-codex.sh` to execute `amata run /in/amata.yaml` plus ordered `--set` flags when `amata.spec` is provided and keep direct `codex exec` invocation when `amata.spec` is absent.
    2. Enforce mode-specific prompt requirements in `mig-codex.sh`: `CODEX_PROMPT` optional when `amata.spec` is provided, `CODEX_PROMPT` required when `amata.spec` is absent.
    3. Keep Codex credential pass-through in `mig-codex` runtime for `CODEX_API_KEY`, `CODEX_CONFIG_TOML`, and `CODEX_AUTH_JSON` with existing secure file materialization behavior.
    4. Keep artifact contract parity in `/out` (`codex.log`, `codex-last.txt`, `codex-run.json`, optional `codex-session.txt`) so node parsing and retries remain deterministic.
    5. Reuse the existing `mig-codex` image/repo name instead of introducing a parallel image to avoid duplicate healing contracts.
  - Verification:
    1. `bash tests/unit/mig_codex_sh_test.sh`
    2. `go test ./tests/integration/migs/mig-codex -run TestMigCodexContainer`
  - Reasoning: high

- [ ] 1.4 Build local `../amata` binary outside Dockerfile and copy into image
  - Repository: `ploy`
    1. Add a pre-build helper script under `deploy/images/migs/mig-codex/` that runs `go build` in `../amata` and stages the binary into ploy build context.
    2. Update `deploy/images/migs/mig-codex/Dockerfile` to `COPY` the prebuilt staged binary only and remove any in-image `amata` compilation logic.
    3. Wire the pre-build helper into all mig image build entrypoints (`deploy/images/build-and-push-migs.sh`, `deploy/images/garage.sh`, `deploy/vps/run.sh`) before `mig-codex` buildx invocation.
    4. Fail fast with a clear error when local `../amata` source or staged binary is missing to keep builds deterministic.
  - Verification:
    1. `deploy/images/build-and-push-migs.sh` (with `PUSH_RETRIES=1 PLATFORM=linux/amd64`)
    2. `deploy/images/garage.sh --force`
  - Reasoning: high

- [ ] 1.5 Update schemas/docs and E2E fixtures for `amata` step usage (Codex-first)
  - Repository: `ploy`
    1. Update `docs/schemas/mig.example.yaml` with router/healing `amata` examples using `amata.spec` and `amata.set`, plus examples without `amata` that keep direct Codex mode.
    2. Update `docs/migs-lifecycle.md`, `docs/envs/README.md`, and `tests/e2e/migs/README.md` to describe dual-mode execution (`amata` path and direct-Codex fallback), including `CODEX_PROMPT` optionality in `amata.spec` mode and requirement in direct-Codex mode.
    3. Update e2e fixture specs in `tests/e2e/migs/scenario-*/mig.yaml` to exercise both modes: `amata` in router/healing and direct-Codex fallback when `amata.spec` is undefined.
    4. Update publishing docs in `docs/how-to/publish-migs.md` with local `../amata` build prerequisite for `migs-codex` image publishing.
  - Verification:
    1. `go test ./tests/e2e/migs/...`
    2. `bash ~/@iw2rmb/amata/scripts/check_docs_links.sh`
  - Reasoning: medium

- [ ] 1.6 Validate end-to-end healing loop with dual execution modes
  - Repository: `ploy`
    1. Run failing-gate scenario with `amata.spec` defined and without `CODEX_PROMPT` and confirm router summary, healing attempt, and re-gate flow still produce deterministic statuses and metadata.
    2. Run failing-gate scenario without `amata.spec` and confirm direct `codex exec` path still requires `CODEX_PROMPT` and preserves the same status and metadata contract.
    3. Verify `amata.set` values are passed exactly as `--set` CLI flags and affect rendered prompt/template output in `../amata` Codex executor.
    4. Run full hygiene/test suite and keep one image with deterministic dual-mode routing.
  - Verification:
    1. `bash tests/e2e/migs/scenario-orw-fail/run.sh`
    2. `make test`
    3. `make vet`
    4. `make staticcheck`
  - Reasoning: xhigh
