# Build Gate + Healing — Spec‑Driven (Codex + ploy‑buildgate)

Scope: Ship a spec‑driven healing loop around the existing Build Gate. On initial gate failure, execute configured healing mods (Codex by default), re‑run the Gate, and proceed on pass. Keep MR on failure behaviour. Align E2E with YAML spec and a single runner script.

Documentation: README.md, docs/envs/README.md, tests/e2e/mods/README.md, docs/schemas/mod.example.yaml; design notes in CHECKPOINT.md.

Legend: [ ] todo, [x] done.

## Constraints
- RED → GREEN → REFACTOR for each slice.
- Coverage ≥60% overall; ≥90% for runner/orchestration touched code.
- Minimal blast radius: add `--spec` to CLI; nodeagent execution orchestration reads spec; no DB schema changes.

## Canonical Inputs: migrate to /in (drop /.ploy)
- [ ] Make `/in` the sole, read‑only mount for cross‑phase inputs.
  - Node: after the first Gate fails, persist the log to a temp host file and mount it at `/in/build-gate.log` for healing steps; do not write into the repository.
  - Remove any writes to `/workspace/.ploy`; treat previous `/.ploy` usage as deprecated and eliminate to avoid accidental inclusion in diffs.
- [ ] mod-codex: prefer `/in` as context.
  - Always include `--add-dir /in` when present; stop referencing `/.ploy`.
  - Examples/prompts should reference `/in/build-gate.log`.
- [ ] Tests/docs/spec:
  - Update `docs/schemas/mod.example.yaml`, E2E specs, and README to reference `/in/build-gate.log` and `/in/prompt.txt` exclusively.
  - Update integration/E2E runners to mount `/in` and stop creating `/.ploy`.
  - Default prompt location: `/in/prompt.txt` (node mounts it R/O when provided in spec); mod-codex should accept `--prompt-file /in/prompt.txt` by default when present.

## Current Status (DONE)
- [x] Build Gate log quality: Maven `-e`; Gradle `--stacktrace`. (internal/workflow/runtime/step/gate_docker.go)
- [x] Introduced `mod-codex` image with Codex CLI and `ploy-buildgate` (same Gate as workers) embedded. (mods/mod-codex/*, internal/cmd/ploy-buildgate)
- [x] Codex logs and stdin prompt handling (`-`) captured in `/out/codex.log`.
- [x] Spec example and E2E spec: `docs/schemas/mod.example.yaml`; `tests/e2e/mods/scenario-orw-fail/mod.yaml`.
- [x] E2E runner script: `tests/e2e/mods/scenario-orw-fail/run.sh`.
- [x] Mods E2E README updated for spec‑driven flow.

## CLI: `--spec` support
- [ ] Add `--spec <file>` (YAML/JSON) to `ploy mod run` and include raw JSON into submit payload `Spec`.
  - Change: cmd/ploy/mod_run.go (parse file; YAML→JSON; merge with `--mod-*` overrides if both present; document precedence).
  - Test: cmd/ploy/testdata/help_mod.txt; unit to assert payload contains `build_gate_healing`.
- [ ] Back‑compat shim: `--heal-on-build` (deprecated) injects a default `build_gate_healing` when spec lacks it.
  - Change: cmd/ploy/mod_run.go usage text; keep for one release.

- [ ] Spec env file resolution (secrets UX):
  - Support `env_from_file` (map name→path) alongside `env` (name→value) in both `mod` and each `build_gate_healing.mods[]` entry.
  - CLI resolves `~`, reads the file content, and inlines it as the env value before submit; do not log contents; redact in debug/JSON output.
  - Optional shorthand: accept `env: NAME: {from_file: "~/.codex/auth.json"}` for YAML users who prefer inline objects.
  - Tests: unit for resolver; integration with a temporary file path.

## Node Agent: Gate‑Heal‑Re‑Gate orchestration
- [ ] Pre‑mod Gate: run the Build Gate before the first mod container.
  - Change: internal/nodeagent/execution.go — split execution into phases (gate → maybe heal → re‑gate → mod).
  - Test: unit — stub Gate to fail; final status=failed (no healing block) with `reason="build-gate"`.
- [ ] Healing loop (spec‑driven): consume `build_gate_healing` from `req.Options`.
  - Execute each `mods[]` entry (image/command/env/retain) in order under `/workspace`, publish `/out` artifacts, re‑run Gate after the sequence; repeat up to `retries`.
  - Change: internal/nodeagent/execution.go (loop + re‑gate); internal/nodeagent/manifest.go helpers to build container manifests from entries.
  - Test: integration (local) with `mods-codex` as healer and failing sample; verify first gate fail, healer runs, re‑gate pass.
- [ ] Proceed to main mod only after a passing Gate.
  - Change: execution.go — skip ORW when re‑gate still fails; exit failed.

## Artifacts + metadata
- [ ] Persist first‑gate logs to `/workspace/.ploy/build-gate.log` (input to codex prompt) and upload logs as artifact bundle.
  - Change: internal/nodeagent/execution.go — write, then upload via ArtifactUploader.
- [ ] Include gate stats in run `stats.gate` (passed/resources/logs_artifact_id) — already partially present for post‑mod gate; ensure present for pre‑gate and re‑gate runs.

## Terminal status + MR on failure
- [ ] Treat Build Gate failure as terminal when no healing is configured or after retries exhausted; keep MR on failure behaviour.
  - Change: internal/nodeagent/execution.go — set `terminalStatus="failed"`, `reason="build-gate"`.
  - Test: unit — with `mr_on_fail=true` ensure MR path still executes; success branch with `mr_on_success` unchanged.

## CLI/docs updates
- [ ] Update docs to describe `--spec` and `build_gate_healing`.
  - Change: cmd/ploy/README.md, docs/envs/README.md, tests/e2e/mods/README.md.
  - Test: lint/docs check.

## E2E coverage
- [x] Spec‑driven fail→heal scenario: `tests/e2e/mods/scenario-orw-fail/run.sh` using `mod.yaml`.
- [ ] Add a “pass” scenario spec (optional) mirroring the older passing path.

## Risks / rollback
- Risk: pre‑gate adds latency; mitigate by fast detection/no‑op when trivial.
- Risk: regression in passing scenario; validate with passing spec scenario.
- Rollback: feature‑gate healing by presence of `build_gate_healing` block.

## Estimates
- CLI `--spec` + docs: 0.5d
- Node Gate‑Heal‑Re‑Gate orchestration: 1.0–1.5d
- Tests (unit + integration): 1.0d
- Buffer/refactor: 0.5d
- Total: ~3.0–3.5d
