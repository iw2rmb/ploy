# Codex Build-Gate Handshake (Exit + Resume)

> When following this template:
> - Align to the template structure
> - Include steps to update relevant docs

Scope: Refactor the `mods-codex` healing flow so Codex never runs Build Gate directly. Instead, Codex exits with a sentinel when it wants validation, Ploy reruns the Build Gate, and failed gates trigger a resumed Codex session that continues from the previous context across healing retries.

Documentation: `docs/mods-lifecycle.md`, `docs/envs/README.md`, `docs/schemas/mod.example.yaml`, `tests/e2e/mods/README.md`, `docker/mods/README.md`, `../auto/ROADMAP.md`, upstream Codex CLI non-interactive docs for `codex exec` / `codex exec resume`.

Legend: [ ] todo, [x] done.

## Phase A — Healing prompts and spec contract
- [x] Define sentinel protocol for Codex healing runs — Allow Codex to signal “ready for Build Gate” without running the gate itself.
  - Component: `tests/e2e/mods`, E2E specs for `build_gate_healing`.
  - Scope: Update `tests/e2e/mods/scenario-orw-fail/mod.yaml` and `tests/e2e/mods/scenario-multi-node-rehydration/mod.yaml` so the embedded `CODEX_PROMPT` uses a sentinel-only contract:
    - Remove explicit instructions to call `buildgate-validate` or run Maven/Gradle.
    - Add a strict rule that, when Codex believes the workspace is ready for validation, it must reply with exactly `[[REQUEST_BUILD_VALIDATION]]` as its final message and then stop.
    - Keep the task text pointing at `/in/build-gate.log` for failure context; only the validation loop moves out of Codex.
    - Example prompt fragment in YAML:
      ```yaml
      build_gate_healing:
        mods:
          - image: docker.io/you/mods-codex:latest
            env:
              CODEX_PROMPT: |-
                Rules:
                - Use /workspace and /in/build-gate.log to understand the compile error.
                - Edit files under /workspace as needed.
                - When you believe the code is ready for a full build validation, reply with exactly:
                  [[REQUEST_BUILD_VALIDATION]]
                  as your final message and then stop.
                Task:
                Fix the compilation error described in /in/build-gate.log.
      ```
  - Test: `bash tests/e2e/mods/scenario-orw-fail/run.sh` and `bash tests/e2e/mods/scenario-multi-node-rehydration/run.sh` — Logs show Codex producing `[[REQUEST_BUILD_VALIDATION]]` and no in-container `buildgate-validate` calls; Build Gate still re-runs and the scenarios pass.

- [x] Remove buildgate-validate usage from Codex-specific docs and examples — Keep documentation aligned with the new handshake.
  - Component: `tests/e2e/mods/README.md`, `docs/schemas/mod.example.yaml`, `docs/how-to/publish-mods.md`.
  - Scope:
    - In `tests/e2e/mods/README.md`, replace the Codex healing example that calls `buildgate-validate` with a sentinel-based description: Codex edits the workspace, emits `[[REQUEST_BUILD_VALIDATION]]`, and the control plane re-runs the Build Gate.
    - In `docs/schemas/mod.example.yaml`, adjust the `build_gate_healing` example to:
      - Remove explicit `buildgate-validate` commands.
      - Show a `mods-codex` entry that relies on `/in/build-gate.log` plus the sentinel contract.
    - In `docs/how-to/publish-mods.md`, update the `mod-codex` section to describe:
      - The sentinel protocol.
      - That Build Gate is always run by Ploy (docker gate) / Build Gate HTTP API, not by Codex.
  - Test: `rg "buildgate-validate" -n` shows no accidental references in Codex healing prompts/docs (only the standalone script under `docker/mods/mod-codex` or legacy docs, if retained intentionally). Run `make test` to ensure doc-related tests or linters (if any) still pass.

## Phase B — Simplify mods-codex image and wrapper
- [ ] Detach `buildgate-validate` from the `mods-codex` image — Ensure Codex cannot run the gate from inside the container.
  - Component: `docker/mods/mod-codex`, unit tests referencing `buildgate-validate.sh`.
  - Scope:
    - In `docker/mods/mod-codex/Dockerfile`:
      - Remove the `COPY docker/mods/mod-codex/buildgate-validate.sh /usr/local/bin/buildgate-validate` line.
      - Remove any `chmod` or symlink specific to `buildgate-validate`; keep `mod-codex` / `mods-codex` entrypoints unchanged.
    - Delete `docker/mods/mod-codex/buildgate-validate.sh` if it is no longer needed by any tests or scripts, or move it under `scripts/` with a clearly documented, non-mods-specific purpose.
    - Update tests that reference this script:
      - `tests/unit/buildgate_validate_sh_test.sh` — either delete or relocate to match the new home of `buildgate-validate.sh`.
      - `tests/integration/mods/mod-codex/mod_codex_test.go` — remove assertions or prompt content that reference `buildgate-validate`, replacing them with sentinel-based expectations.
  - Test:
    - `docker build -t mods-codex:latest -f docker/mods/mod-codex/Dockerfile .` — Image builds successfully without `buildgate-validate`.
    - `GOFLAGS=${GOFLAGS:-} go test -v ./tests/integration/mods/mod-codex -run TestModCodex_HealsUsingBuildGateLog_FromFailingBranch -count=1` — Confirms the integration test passes with the new image and prompt contract.

- [ ] Capture Codex last message and thread/session id from `codex exec` — Provide artifacts the node agent can use for resume and sentinel detection.
  - Component: `docker/mods/mod-codex/mod-codex.sh`.
  - Scope:
    - Extend the `codex exec` invocation to enable structured output:
      - After constructing `cmd=(codex exec ...)`, detect support for JSON/FS options via `--help`:
        ```bash
        if grep -q -- "--json" <<<"$help_out"; then
          cmd+=(--json)
        fi
        if grep -q -- "--output-last-message" <<<"$help_out"; then
          cmd+=(--output-last-message "$out_dir/codex-last.txt")
        fi
        if grep -q -- "--output-dir" <<<"$help_out"; then
          cmd+=(--output-dir "$out_dir/codex-transcript")
        fi
        ```
      - Keep the `--add-dir` detection in place so `/workspace` and `/in` remain attached.
    - Capture JSON events to a sidecar file for session/thread extraction:
      - Pipe Codex output to both `codex.log` and a JSONL file:
        ```bash
        jsonl="$out_dir/codex-events.jsonl"
        echo "[mod-codex] starting codex exec with repo context" > "$logfile"
        set +e
        printf "%s" "$prompt" | "${cmd[@]}" 2>&1 | tee -a "$logfile" | tee "$jsonl" >/dev/null
        status=${PIPESTATUS[1]}
        set -e
        ```
      - Use `jq` (already installed in the Dockerfile) to extract the thread/session id from the first `thread.started` event:
        ```bash
        session_id=""
        if command -v jq >/dev/null 2>&1 && [[ -s "$jsonl" ]]; then
          session_id="$(jq -r 'select(.type=="thread.started") | .thread_id // empty' "$jsonl" | head -1 || true)"
        fi
        ```
      - Write `/out/codex-session.txt` (or similar) with the `session_id` when non-empty:
        ```bash
        if [[ -n "$session_id" ]]; then
          printf "%s\n" "$session_id" > "$out_dir/codex-session.txt"
        fi
        ```
    - Record whether Codex requested Build Gate:
      - After `codex exec` completes, check for `codex-last.txt` and set a flag in `codex-run.json`:
        ```bash
        requested_build=false
        if [[ -f "$out_dir/codex-last.txt" ]]; then
          if [[ "$(tr -d '\r\n' < "$out_dir/codex-last.txt")" == "[[REQUEST_BUILD_VALIDATION]]" ]]; then
            requested_build=true
            printf "true\n" > "$out_dir/request_build_validation"
          fi
        fi
        ts=$(date -u +%Y-%m-%dT%H:%M:%SZ)
        printf '{"ts":"%s","exit_code":%s,"model":"%s","input":"%s","requested_build_validation":%s,"session_id":"%s"}\n' \
          "$ts" "${status:-0}" "${model}" "$input_dir" \
          "${requested_build:?false}" "${session_id}" > "$manifest"
        ```
  - Test:
    - Add or extend tests in `tests/integration/mods/mod-codex/mod_codex_test.go` to assert:
      - `codex-last.txt` exists and contains `[[REQUEST_BUILD_VALIDATION]]` when the prompt requests a gate.
      - `request_build_validation` and `codex-session.txt` exist in `/out` when running with a real Codex auth (or a stubbed CLI).
    - Run `make test` and `scripts/validate-tdd-discipline.sh ./docker/mods/...` to ensure coverage thresholds referenced in `AGENTS.md` remain satisfied.

- [ ] Implement Codex session resume mode inside `mod-codex.sh` — Allow subsequent healing attempts to re-use the original session.
  - Component: `docker/mods/mod-codex/mod-codex.sh`.
  - Scope:
    - Introduce an opt-in resume mode controlled via environment:
      - If `CODEX_RESUME=1` and `/in/codex-session.txt` exists, call `codex exec resume` instead of a fresh `codex exec`:
        ```bash
        resume_session=""
        if [[ "${CODEX_RESUME:-}" == "1" && -f "/in/codex-session.txt" ]]; then
          resume_session="$(tr -d '\r\n' < /in/codex-session.txt)"
        fi
        cmd=(codex exec)
        # ... feature detection for flags as above ...
        if [[ -n "$resume_session" ]]; then
          cmd+=(resume "$resume_session")
        fi
        cmd+=( - )
        ```
      - For resume runs, prepend a short instruction to the prompt (either via an env var or by adjusting the YAML prompt) clarifying that:
        - The previous Build Gate still failed (new `/in/build-gate.log` content).
        - Codex should continue healing from the existing context and again emit `[[REQUEST_BUILD_VALIDATION]]` once ready.
    - Ensure the resume path still writes `codex-last.txt`, `codex-session.txt`, and `codex-run.json` in the same format so the node agent does not have to distinguish between first and subsequent runs.
  - Test:
    - Extend `tests/integration/mods/mod-codex/mod_codex_test.go` with a (skipped-by-default) test that:
      - Runs `mods-codex` once to create a session and `codex-session.txt`.
      - Invokes `mods-codex` again with `CODEX_RESUME=1` and `/in/codex-session.txt` mounted.
      - Verifies via logs that `codex exec resume` was used (e.g., by injecting a small marker into the prompt and checking the behavior).
    - Manual smoke: run the E2E healing scenarios with real Codex to confirm logs show a resumed session (continuous conversation) instead of a fresh run.

## Phase C — Node agent healing loop orchestration
- [ ] Propagate Codex session and sentinel artifacts through the healing loop — Let the node agent re-use Codex sessions across retries without changing gate semantics.
  - Component: `internal/nodeagent/execution_healing.go`, `internal/nodeagent/manifest.go`, artifact upload helpers.
  - Scope:
    - In `executeWithHealing` (see `internal/nodeagent/execution_healing.go`), extend the healing loop to track Codex session and sentinel state:
      - Before the `for attempt := 1; attempt <= retries; attempt++` loop, declare variables to hold the last known Codex session id and a boolean indicating whether the last Codex run requested validation (e.g., `var codexSession string`).
      - After each healing mod run (`healResult, healErr := runner.Run(...)`), inspect `/out` when the image is `mods-codex` (or when the expected files exist):
        - Read `filepath.Join(outDir, "codex-session.txt")` into `codexSession` when present.
        - Read `filepath.Join(outDir, "request_build_validation")` or `codex-run.json` to determine whether Codex produced the sentinel.
      - Copy `codex-session.txt` into `/in` for subsequent attempts so `mod-codex.sh` can use resume mode:
        ```go
        if codexSession != "" && *inDir != "" {
          if writeErr := os.WriteFile(filepath.Join(*inDir, "codex-session.txt"), []byte(codexSession), 0o644); writeErr != nil {
            slog.Warn("healing: failed to persist codex-session.txt into /in", "run_id", req.RunID, "error", writeErr)
          }
        }
        ```
      - Keep Build Gate semantics unchanged: the node agent still re-runs the gate after healing mods complete on each attempt; sentinel state is used for observability and potential future policy, not to skip gates.
    - In `buildHealingManifest` (`internal/nodeagent/manifest.go`), inject `CODEX_RESUME=1` into the healing mod environment when:
      - A non-empty `codexSession` is available.
      - The healing mod image (or name) indicates a Codex-based healer (e.g., `mods-codex` or a configurable list).
      - For non-Codex healing mods, do not set `CODEX_RESUME`.
    - Ensure the `/in` directory remains read-only inside the container; the agent writes `codex-session.txt` on the host side only.
  - Test:
    - Add tests in `internal/nodeagent/manifest_healing_test.go` to confirm:
      - `buildHealingManifest` sets `CODEX_RESUME=1` only when a Codex session id is available and the healing mod image matches the Codex pattern.
      - Non-Codex healing mods remain unaffected.
    - Add tests in `internal/nodeagent/execution_healing_retry_test.go` to cover:
      - A failing gate with `build_gate_healing` that uses `mods-codex`:
        - First attempt populates `codex-session.txt` in `/out`.
        - Second attempt sees `CODEX_RESUME=1` in the healing manifest and `codex-session.txt` in `/in`.
      - Retries still stop once the gate passes or when the retry count is exhausted.

- [ ] Keep HTTP Build Gate and docker gate behavior consistent — Ensure node agent still re-runs the gate and records the full gate history.
  - Component: `internal/nodeagent/execution_healing.go`, `internal/workflow/runtime/step/gate_docker.go`, Build Gate API client docs.
  - Scope:
    - Verify that the existing re-gate logic in `executeWithHealing` remains intact:
      - Healing mods still operate on the same workspace; re-gate runs via `runner.Gate.Execute` with the updated workspace.
      - The `BuildGateStageMetadata` for pre-gate and each re-gate is captured in `PreGate` and `ReGates` respectively.
    - Ensure comments and docs reflect that:
      - Healing mods may optionally call the HTTP Build Gate API directly (using their own tooling), but this is now discouraged for `mods-codex`.
      - The canonical gate results are always those produced by the node agent’s `GateExecutor`, not by in-container scripts.
  - Test:
    - `go test ./internal/nodeagent -run TestRunController_ExecuteWithHealing` (and related tests in `execution_healing_test.go` / `execution_healing_retry_test.go`) — Confirm expectations around gate metadata and retry behavior still hold.
    - Run `scripts/validate-tdd-discipline.sh ./internal/...` to verify coverage and static analysis thresholds for critical workflow packages.

## Phase D — End-to-end validation and discipline
- [ ] Align tests and TDD guardrails with the new handshake — Ensure RED→GREEN→REFACTOR for the Codex healing pipeline.
  - Component: `tests/e2e/mods`, `tests/integration/mods/mod-codex`, `scripts/validate-tdd-discipline.sh`.
  - Scope:
    - Update or add E2E assertions for the sentinel and session behavior:
      - In `tests/e2e/mods/scenario-orw-fail/run.sh` and `scenario-multi-node-rehydration/run.sh`, validate (via logs or artifacts) that:
        - Codex emits `[[REQUEST_BUILD_VALIDATION]]` before each gate rerun.
        - After a failed gate, the next healing attempt logs that it is resuming a previous Codex session (e.g., by checking for the presence of `codex-session.txt` in artifacts).
      - Optionally enhance `tests/e2e/mods/README.md` with a short checklist for:
        - Sentinel visibility.
        - Session resume across healing retries.
    - Document the discipline in `GOLANG.md` and cross-link back to this roadmap section for future refactoring work.
  - Test:
    - `make test` — All unit and integration tests pass, including the Codex integration when `CODEX_AUTH_JSON` is set.
    - `./scripts/validate-tdd-discipline.sh` — Repository-wide TDD discipline passes, confirming coverage (≥60% overall, ≥90% for critical workflow packages) and Build Gate binary size constraints are still met after the changes.
