Parallel Healing Scenario
=========================

This scenario exercises healing when multiple fixes are available in parallel (LLM exec + OpenRewrite). The GitLab repo branch `e2e/fail-parallel` intentionally fails to compile due to both a missing symbol and a Nashorn reference, forcing the platform to fan out fixes.

Quickstart
----------
1. Ensure the failure branch exists: `tests/e2e/mods/parallel/prepare-branches.sh https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git main --verify`.
2. Run the scenario: `cd tests/e2e/mods/parallel && ./run.sh` (set `PLOY_CONTROLLER`, `PLOY_GITLAB_PAT`, etc.).
3. Inspect `../logs/<MOD_ID>/events.sse` and `status_last.json` for parallel branch outcomes.

Utilities
---------
- `../watch-events.sh <MOD_ID>` tails SSE in real time.
- `MOD_LOG_DIR=../logs/<MOD_ID> ../collect-logs.sh <MOD_ID>` downloads controller, platform, and builder logs.
- Logs live under `../logs/<MOD_ID>`; export `MOD_LOG_DIR` when running helpers from other directories.
- `../generate-evidence.sh ../logs/mod-*` summarizes compile failures, planner events, and diffs.
- `../fetch-artifacts.sh <MOD_ID>` pulls `plan.json`, `next.json`, and `diff.patch` artifacts from the controller.

Scenario File
-------------
`scenario.yaml` configures lane D, enables self-heal, and runs both `orw-apply` and `llm-plan` steps (with planner concurrency enabled).

Troubleshooting
---------------
- If OpenRewrite fails to download its recipe, check SSE for `orw env coords resolved`. We now normalize `rewrite-java-latest:latest` to `rewrite-migrate-java:3.17.0` automatically.
- When builds continue to fail with `UnknownClass`, the run succeeded at shifting the failure to the missing symbol; rerun the scenario to let the planner propose additional fixes.
