Scenario: orw-apply fails, llm-plan heals, llm-exec applies fix
=============================================================

This scenario exercises the full healing loop for the Java 11 -> 17 migration: OpenRewrite applies the recipe, the compile gate fails, LLM planner/executor generate a corrective patch, and the run finishes with a passing build and Merge Request.

Current Cycle Key Takeaways
---------------------------
- Lane C enables the compile, static-analysis, and vuln-scan gates; deploy and tests are disabled, so local compile success is the healing signal.
- `run.sh` submits `scenario.yaml`, streams `/mods/{id}/logs`, polls status, and fetches artifacts to `logs/<MOD_ID>/`. Expect `plan_json`, `next_json`, and `diff.patch` when healing succeeds.
- Build failures must be deterministic. Use the prepared `e2e/fail-missing-symbol` branch so orw-apply produces a diff yet Maven still fails, triggering self-heal.
- Keep enough Nomad capacity free for the OpenRewrite task (about 1 GiB). If the run stalls, grab platform logs through `collect-logs.sh` and confirm the planner/executor received SeaweedFS and MOD_ID env vars.
- Compile-gate diagnostics: controller may return a generic `internal_error`. Mods now emits build events with `(deployment_id=…)`; fetch Maven logs via `GET /v1/apps/:app/builds/:id/logs?lines=…` to see the real failure (wired for lanes C/E/G).
- LLM-exec diffs: older runs can upload a sentinel `.llm-healing` patch (rejected by the allowlist). Ensure `langgraph-runner:latest` is deployed; it resolves `first_error_file:line` and emits `resolved target file: …` before generating a minimal edit diff.
- Artifact access: when `PLOY_SEAWEEDFS_URL` is not resolvable from the workstation, use SSH within `collect-logs.sh` to pull SeaweedFS artifacts (planner plans, LLM diffs) and last_job logs from the VPS.
- Nomad logs slicing: `collect-logs.sh` now derives a `--since` timestamp from SSE and passes it to the Nomad log wrapper to slice allocation logs by time for faster, targeted inspection.
- What to watch in SSE: look for `uploaded diff to …/steps/<RUN_ID>/diff.patch`, `download succeeded`, `replay starting: branch_id=llm-1`, and build events including `(deployment_id=…)` for log drill‑downs.

Prepared Repository
-------------------
- GitLab repo: https://gitlab.com/iw2rmb/ploy-orw-java11-maven
- Default failure branch: `e2e/fail-missing-symbol` (intentionally breaks compile post-ORW)
- Optional variants: `e2e/fail-java17-specific`, `e2e/success`

Prerequisites
-------------
- Build the CLI: `go build -o ./bin/ploy ./cmd/ploy` (or `make build`)
- Environment:
  - `PLOY_CONTROLLER` (for example `https://api.dev.ployman.app/v1`)
  - `GITLAB_URL`, `GITLAB_TOKEN` with api/write scopes
  - SeaweedFS variables if artifacts are mirrored (`PLOY_SEAWEEDFS_URL`)
- Tooling: `curl`, `jq`, `rg`
- Git identity configured locally (`git config user.name/user.email`) so the commit step cannot fail

Run the Cycle
-------------
1. Review or tweak `scenario.yaml` (lane C, compile gate, self-heal enabled).
2. Execute `./run.sh` with the required environment variables exported.
3. The script prints `MOD_ID`, tails SSE events, downloads artifacts, and stores everything under `logs/<MOD_ID>/`.

Verify the Run
--------------
- Success criteria: status `completed`, non-empty `result.mr_url`, compile gate passes after healing, diff captured from LLM executor.
- `./check-steps.sh <MOD_ID>` ensures the key phases occurred in order (ORW diff, build failure, planner -> llm-exec -> reducer).
- `./generate-evidence.sh <logs/mod-*>` summarizes build errors, prompts, and diffs for attachments or regressions.
- `./collect-logs.sh <MOD_ID>` downloads controller/platform logs plus referenced SeaweedFS artifacts when deeper debugging is required.
- Builder failures now emit a SeaweedFS pointer (`build-logs/<JOB>.log`). `collect-logs.sh` writes the key to `builder_logs.key`, fetches the artifact locally, and also downloads the full log through the controller route `GET /v1/apps/<app>/builds/<JOB>/logs/download` when SeaweedFS isn’t reachable.

Next Step
---------
Automate the scenario by wiring `./check-steps.sh` into the healing E2E so the MR path is continuously verified when the compile gate is the terminal signal.
