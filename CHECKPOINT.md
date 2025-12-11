# CHECKPOINT: Mods / Healing / Diff Pipeline

- Clarify and fix Codex artifact expectations in E2E scripts.
  - `tests/e2e/mods/scenario-orw-fail/run.sh` (and any similar Codex scenarios) currently assume `codex.log`, `codex-session.txt`, and `codex-run.json` appear as plain files in `$ARTIFACT_DIR`.
  - The nodeagent uploads `/out` as a tar.gz bundle named `mod-out`, and `ploy mod run --artifact-dir` downloads this bundle as a single `.bin` file plus `manifest.json`.
  - Change the E2E scripts to:
    - Parse `manifest.json` or glob for `*_mod-out.bin` corresponding to the Codex healing job.
    - Untar that `mod-out` bundle into `$ARTIFACT_DIR` before checking for `codex.log`, `codex-session.txt`, and `codex-run.json`.
    - Keep the validation logic the same, but operate on the extracted contents instead of assuming flat files.

- Remove inline healing diffs entirely.
  - Inline healing = the gate-heal-regate loop inside a single job (`runGateWithHealing` in `internal/nodeagent/execution_healing.go`) that uses `uploadHealingModDiff`.
  - Delete `uploadHealingModDiff` from `internal/nodeagent/execution_healing_helpers.go` and remove its invocation from `runGateWithHealing`.
  - Remove or update any tests and docs that assume per-attempt `mod_type="healing"` diffs exist for inline healing; rely only on discrete healing jobs (`mod_type="heal"`, `uploadHealingJobDiff`) for diff-based observability.

- Clean up per-step diff helpers to be strictly baseline-based, with no legacy HEAD semantics or fake baselines.
  - `uploadModDiffWithBaseline` (mods):
    - Keep the current behavior: snapshot the rehydrated workspace into `modBaselineDir` before running the mod, then use `GenerateBetween(modBaselineDir, workspace)` after the mod finishes.
    - Remove the fallback that calls `uploadDiffForStep` when `baseDir` is empty; if no baseline snapshot is available, log and skip the mod diff instead of inventing a legacy path.
  - `uploadDiffForStep` (generic per-step diff):
    - Current implementation calls `GenerateBetween(workspace, workspace)`, which always produces an empty diff.
    - Either:
      - Delete `uploadDiffForStep` outright and remove its callsites and tests, or
      - Redesign it to require an explicit `baseDir` and refuse to run without a baseline (no HEAD-based generation, no workspace==workspace hack).
    - Align tests in `internal/nodeagent/execution_orchestrator_diff_test.go` with whatever shape is kept; they must assert true baseline+`GenerateBetween` semantics.
  - `uploadHealingJobDiff` (discrete healing job at step 1500):
    - Keep the baseline+`GenerateBetween(baseDir, workspace)` behavior and the `mod_type="mod"` tagging, with no fallback to generic per-step diff helpers.

- Keep nodeagent free of HEAD-based diff generation.
  - Do not reintroduce `DiffGenerator.Generate`/`git diff HEAD` in any runtime path under `internal/nodeagent`.
  - If a diff cannot be expressed as `GenerateBetween(baselineDir, workspace)`, skip it or redesign the callsite; do not add hidden fallbacks.

- Make Codex healing + MR behavior more observable when things go wrong.
  - MR wiring:
    - Confirm that `gitlab_pat`, `gitlab_domain`, and `mr_on_success` from the spec/CLI are visible in the manifest options (`manifest.Options`) for the mod job (step 2000).
    - When `shouldCreateMR` returns true and `createMR` fails (git push or GitLab API), log a clear warning that MR creation failed even though the run succeeded, so missing MRs are debuggable without guessing.
  - Codex artifacts:
    - Optionally extend CLI artifact download to support an opt-in mode that auto-extracts `mod-out` bundles for Codex healing stages into the `--artifact-dir`, keeping the default behavior (raw bundles + manifest) unchanged.

