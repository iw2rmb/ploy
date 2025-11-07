# Build Gate + Healing (pre‑mod gate, optional LLM heal, MR on failure)

Scope: Implement pre‑mod Build Gate; on failure and when `--heal-on-build` is set, trigger `mod-llm` healing and re‑run the gate; on pass, proceed to OpenRewrite as planned. Ensure `tests/e2e/mods/scenario-orw-fail.sh` passes: MR is created on failure without healing.

Documentation: README.md, docs/envs/README.md, tests/e2e/mods/README.md; design notes in CHECKPOINT.md; code refs below.

Legend: [ ] todo, [x] done.

## Constraints
- RED → GREEN → REFACTOR for each slice.
- Coverage ≥60% overall; ≥90% for runner/orchestration touched code.
- Minimal blast radius: CLI flag plumbing; nodeagent execution orchestration; no schema changes.

## CLI: flag + spec plumbing
- [ ] `--heal-on-build` flag; plumb into submit spec as `heal_on_build` — Enables healing branch when the first Build Gate fails.
  - Change: cmd/ploy/mod_run.go (parse flag; include `payload["heal_on_build"]=true`; update usage string).
  - Test: cmd/ploy/testdata/help_mod.txt — help output includes `--heal-on-build`.

## Node Agent: pre‑mod Build Gate (Phase A)
- [ ] Run Build Gate before any mod container execution.
  - Change: internal/nodeagent/execution.go — split current single `runner.Run` into phases:
    1) Gate‑only pass: manifest with noop command, `Gate.Enabled=true`.
    2) Conditional LLM heal + re‑gate when enabled.
    3) Proceed to ORW only if a gate has passed.
  - Change: internal/nodeagent/manifest.go — helper builder(s) for: gate‑only, llm‑exec, orw‑apply.
  - Test: unit — inject stub `GateExecutor` to force fail; expect terminal status=failed and reason="build-gate" when healing disabled.

## Healing (Phase B): `mod-llm` on gate fail when enabled
- [ ] Accept `options.heal_on_build` and optional `options.heal_llm_image` (default `docker.io/${DOCKERHUB_USERNAME}/mods-llm:latest` or `mods-llm:latest`).
  - Change: internal/nodeagent/handlers.go comment + option docs; internal/nodeagent/execution.go reads options.
  - [ ] Emit prompt for LLM: write first gate’s logs to `/workspace/.ploy/build-gate.log`.
    - Change: internal/nodeagent/execution.go — persist `result.BuildGate.LogsText` into workspace.
  - [ ] Run LLM container once: command `mods-llm --execute --input /workspace --out /out/plan.json`.
    - Change: internal/nodeagent/execution.go — second `runner.Run` with llm manifest; keep `Gate.Enabled=false`.
  - Test: mods/mod-llm stub already heals failing sample; unit — verify second gate reads healed workspace and passes.

## Re‑gate then ORW (Phase C)
- [ ] Re‑run Build Gate after LLM; proceed only on pass.
  - Change: internal/nodeagent/execution.go — third `runner.Run` with gate‑only manifest.
  - [ ] Run ORW mod when a gate has passed at least once.
    - Change: internal/nodeagent/execution.go — fourth `runner.Run` with ORW manifest (image/env from spec).
    - Note: keep post‑ORW gate out for this slice; add as follow‑up if needed.
  - Test: e2e (heal scenario) — Build Gate fails → LLM heals → re‑gate passes → ORW runs → final success.

## Terminal status + MR on failure
- [ ] Treat Build Gate failure as terminal when healing disabled or unsuccessful.
  - Change: internal/nodeagent/execution.go — set `terminalStatus="failed"`, `reason="build-gate"`.
  - Test: unit — with `mr_on_fail=true` ensure MR creation attempted; with success branch ensure `mr_on_success` path unchanged.

## Artifacts + metadata
- [ ] Upload `build-gate.log` artifact (already wired) and attach gate JSON in stats.
  - Change: internal/nodeagent/execution.go — ensure artifact upload from Phase A/B gate runs; include `stats["gate"].passed=false|true`.
  - Test: server handlers list artifacts for stage; CLI `mod inspect` shows reason `build-gate`.

## CLI/docs updates
- [ ] Update docs with new flag and flow.
  - Change: cmd/ploy/README.md — include `--heal-on-build` and examples.
  - Change: docs/envs/README.md — describe per‑run flag.
  - Change: tests/e2e/mods/README.md — document fail→heal flow and no‑heal behavior (scenario‑orw‑fail).
  - Test: lint/docs check, spot read.

## E2E coverage
- [ ] No‑heal failure path (this ticket): ensure MR on failure.
  - Change: none; script uses `tests/e2e/mods/scenario-orw-fail.sh`.
  - Expect: ticket final=failed; MR URL present; artifacts downloaded.
- [ ] Heal path (new): add `tests/e2e/mods/scenario-orw-heal.sh`.
  - Change: new script enabling `--heal-on-build`; target ref `mods-upgrade-java17-heal`.
  - Expect: first gate fail → llm heal → gate pass → ORW → final succeeded.

## Risks / rollback
- Risk: pre‑gate adds latency; mitigated by noop container before gate.
- Risk: regression in passing scenario; validate with `tests/e2e/mods/scenario-orw-pass.sh`.
- Rollback: feature‑flag via `--heal-on-build`; pre‑gate is safe (does not alter repo).

## Estimates
- CLI flag + docs: 0.5d
- Node multi‑phase orchestration: 1.5d
- Tests (unit + new e2e): 1.0d
- Buffer/refactor: 0.5d
- Total: ~3.5d

