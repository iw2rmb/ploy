Scenario: orw-apply fails, llm-plan heals, llm-exec applies fix

Overview

- Goal: Reproduce a run where OpenRewrite (orw-apply) produces a diff but the build gate fails; the system then triggers the healing flow using llm-plan → llm-exec → reducer. The winning branch produces a patch that passes the build gate, and a Merge Request is created.
- This mirrors the validated Java 11→17 pipeline while forcing a compile failure post-ORW apply, so the planner has work to do.
- Uses SeaweedFS for artifacts and the controller’s /v1/mods/{id}/events endpoint for live status.

Prepared Repo (ready-to-use)

- A public GitLab repository is prepared with deterministic failure branches so you don’t need to fork or craft failures yourself:
  - Repo: https://gitlab.com/iw2rmb/ploy-orw-java11-maven
  - Branches intended to fail the build after orw-apply (to trigger healing):
    - e2e/fail-missing-symbol — introduces a missing symbol compile error
    - e2e/fail-java17-specific — introduces a Java 17–specific compile error
  - Use one of these branches as the target to validate the full sequence:
    orw-apply -> build (fail) -> llm-plan -> [apply fix] -> build (success) -> MR

Pre‑requisites

- Workstation: ploy CLI built at ./bin/ploy (make build or go build ./cmd/ploy)
- Tools: curl, jq
- Env: GITLAB_URL, GITLAB_TOKEN (write/api scopes), PLOY_CONTROLLER (e.g., https://api.dev.ployman.app/v1)
- VPS: API deployed and reachable; internal images configured per docs/mods/knobs.md
- Repo: EITHER your own fork OR the prepared repo above (recommended for quick validation)

How we force a predictable build failure

- Baseline is a Maven project that compiles on Java 11 and where the Java 17 migration recipe makes changes. To deterministically fail the build gate after orw-apply, add a small compile error that OpenRewrite will not fix. Pick exactly one of the following approaches in your fork:
  - Add a reference that only fails under 17: Introduce code calling SecurityManager APIs and mark it final (compilation breaks under 17 with certain flags). Example: create src/main/java/demo/LegacySecurity.java using removed/denied APIs.
  - Remove critical import: Add a tiny class referencing a symbol not on classpath and exclude it from orw-apply change targets so ORW doesn’t touch it.
  - Safer deterministic option: Introduce a failing snippet guarded by a profile the build gate enables. For instance, add a module that intentionally references a missing type, and enable -DfailOnCompilationError=true. Keep it minimal (one file) to ease healing.

Expected event timeline (observability)

- orw-apply
  - apply/orw-apply info: job started (task logs via job wrapper)
  - apply/orw-apply info: orw-apply job completed
  - apply/diff-found info: diff ready (N bytes)
  - apply/diff-apply-started info: Applying diff to repository
  - build/build-gate-start info: Running build gate
  - build/build-gate-failed error: apply/build failed: …
- healing
  - planner/planner info: job started → job submitted (alloc id) → job completed
  - llm-exec/llm-exec info: job started → job completed (produces diff.patch)
  - reducer/reducer info: job started → job submitted (alloc id) → job completed (action: stop)
- finalize
  - apply/diff-found (branch chain re-apply if needed)
  - apply/diff-applied info: Diff applied and build gate passed
  - Merge Request created, recorded in status.result.mr_url

Artifacts persisted by controller

- plan_json → artifacts/mods/<exec_id>/plan.json
- next_json → artifacts/mods/<exec_id>/next.json
- diff_patch → artifacts/mods/<exec_id>/diff.patch (or branch-scoped under branches/<id>/steps/<sid>/diff.patch)
- error_log (if produced by orw-apply)

Files here

- scenario.yaml — Mods config with orw-apply then llm healing context (planner configured implicitly on failure)
- run.sh — Runs scenario end-to-end, streams /v1/mods/logs SSE, polls status, downloads artifacts
- watch-events.sh — Attach to the live SSE stream for a given EXEC_ID (optional standalone)
- fetch-artifacts.sh — Download plan_json, next_json, diff_patch for a finished EXEC_ID
- check-steps.sh — Validates presence of key steps (diff-found, build-gate-failed, planner/llm-exec/reducer lifecycle)
- prepare-branches.sh — Creates e2e/success, e2e/fail-missing-symbol, e2e/fail-java17-specific in your repo

Quick start (scripts) — operator flow

1) Option A — Use prepared repo/branches (recommended):
   - export PLOY_CONTROLLER=https://api.dev.ployman.app/v1
   - export GITLAB_URL=https://gitlab.com
   - (Optional) export GITLAB_TOKEN=glpat-…  # only used by cleanup scripts/tests to delete MR source branches
   - Choose one branch to trigger healing:
     - export E2E_HEALING_REPO=https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git
     - export E2E_HEALING_BRANCH=e2e/fail-missing-symbol    # or e2e/fail-java17-specific
   - Run via scripts (SSE stream, artifacts, step checks):
     - ./run.sh
     - or: ./watch-events.sh <EXEC_ID> (after run prints EXEC_ID)
     - After completion: ./fetch-artifacts.sh <EXEC_ID> and ./check-steps.sh <EXEC_ID>

   Option B — Prepare/fork your own repo with an intentional compile failure (see “How we force a predictable build failure”).
2) Set env:
   - export GITLAB_URL=https://gitlab.com
   - export GITLAB_TOKEN=glpat-…
   - export PLOY_CONTROLLER=https://api.dev.ployman.app/v1
3) Edit scenario.yaml:
   - id: choose a unique id (e.g., java11to17-orw-llm)
   - target_repo: set to your fork URL
   - Ensure recipe coords in the orw-apply step match docs/continue.md “Working combo”
4) (Optional) Prepare branches in your repo once (if using your own fork):
   - ./prepare-branches.sh https://gitlab.com/your/repo.git main
   - This creates:
     - e2e/success
     - e2e/fail-missing-symbol
     - e2e/fail-java17-specific (Nashorn-based 17-incompatible code)
   - To verify locally with Maven:
     - ./prepare-branches.sh https://gitlab.com/your/repo.git main --verify
     - Expects e2e/success to compile, and the two fail branches to fail compile.
5) Run (workstation):
   - ./run.sh
   - Script prints EXEC_ID, follows SSE, and waits until terminal. Artifacts are saved under ./logs/<EXEC_ID>/

Quick start (Go E2E) — prepared repo/branches

- Healing flow validation (uses prepared failing branch). Ensure controller is reachable:
  - export PLOY_CONTROLLER=https://api.dev.ployman.app/v1
  - export E2E_HEALING_REPO=https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git
  - export E2E_HEALING_BRANCH=e2e/fail-missing-symbol   # or e2e/fail-java17-specific
  - go test -count=1 ./tests/e2e -tags e2e -v -run HealingFlow_ORWFail_LLMSucceeds -timeout 20m
  - Expected: build gate fails after orw-apply, planner/llm-exec/reducer run, build passes, MR URL logged.

Run Log & Key Takeaways

- Cycle 1 (before profile tweak):
  - Branches used: e2e/fail-missing-symbol, e2e/fail-java17-specific.
  - Outcome: orw-apply completed; build gate passed; MR created (no healing triggered):
    - MR: https://gitlab.com/iw2rmb/ploy-orw-java11-maven/-/merge_requests/22
    - MR: https://gitlab.com/iw2rmb/ploy-orw-java11-maven/-/merge_requests/23
  - Takeaway: the “fail” branches no longer produced a deterministic build failure in the current env.

- Cycle 2 (deterministic fail reintroduced via Maven profile + build-gate property):
  - Repo updated: added a Maven profile `healing-gate` activated by property `ploy.build.gate=1` that adds `src/healing/java` as a compile source (containing intentional compile errors). Failing classes were moved out of `src/main/java` and into `src/healing/java`.
  - API updated: build gate now passes `-Dploy.build.gate=1` so the failure only occurs during the gate, not during orw-apply.
  - Deploy: main branch redeployed to Dev VPS.
  - Outcomes observed:
    - On `e2e/success`: orw-apply completed; build gate attempted twice, then one path reported 502 (push/deploy layer) causing overall failure. Exec: tf-b51ed979.
    - On `e2e/fail-*` branches: in some runs orw-apply allocation failed early (exit 1) before build gate, likely transient infra/plugin fetch issue (no artifacts recorded). Execs: tf-a6a5596b, tf-48583b92.
  - Takeaways:
    - The profile approach is wired end-to-end (API passes the property); success branch exercised build gate but hit a 502 error path.
    - Intermittent `orw-apply` allocation failures require resilience/telemetry improvements (capture transform.log/error.log; transient retry/backoff).
    - Next: re-run once caches warm and verify that on `e2e/success` the build-gate triggers a deterministic compile fail → healing → success → MR.

Notes
- Scripts (`run.sh`, `watch-events.sh`, `fetch-artifacts.sh`, `check-steps.sh`) now have executable bits. `fetch-artifacts.sh` persists artifacts indices/logs under `logs/<EXEC_ID>/`.
- For deep debugging of `orw-apply`, enhance the runner to always upload `/workspace/out/transform.log` and `error.log` to artifacts, even on failures.

Go E2E tests — CI/VPS flow

- Primary method for automated validation. Tests live under tests/e2e with build tag `e2e`.
- New healing test: TestModsE2E_HealingFlow_ORWFail_LLMSucceeds
  - Skips unless PLOY_CONTROLLER and E2E_HEALING_REPO are set.
  - Branch selection: E2E_HEALING_BRANCH (default e2e/fail-missing-symbol).
  - Expects build-gate failure (deterministic branch) then planner → llm-exec → reducer, and asserts plan_json/next_json artifacts on the controller.
- Existing migration and learning tests were updated to use `type: orw-apply` with explicit recipe coordinates (no discovery).
- Run examples:
  - go test ./tests/e2e -tags e2e -v -run HealingFlow -timeout 20m
  - env PLOY_CONTROLLER=… E2E_HEALING_REPO=https://gitlab.com/… E2E_HEALING_BRANCH=e2e/fail-missing-symbol go test ./tests/e2e -tags e2e -v -run HealingFlow -timeout 20m
  - env PLOY_CONTROLLER=… E2E_BRANCH=e2e/success go test ./tests/e2e -tags e2e -v -run JavaMigrationComplete -timeout 15m

Branch strategy (single repo)

- Maintain scenario branches in the same repo for predictable, isolated cases:
  - e2e/success: compiles after orw-apply (happy path + MR).
  - e2e/fail-missing-symbol: add one trivial missing symbol reference to force a compile error post-ORW.
  - e2e/fail-java17-specific: optional, add a small 17-incompatible snippet to fail under Java 17.
- Tests select branches via env with sane defaults:
  - E2E_BRANCH defaults to e2e/success (success/learning tests).
  - E2E_HEALING_BRANCH defaults to e2e/fail-missing-symbol (healing test).

What success looks like

- Terminal status completed, with:
  - result.success: true
  - result.mr_url: non-empty GitLab MR URL
  - artifacts.plan_json and artifacts.next_json present
  - artifacts.diff_patch present (from winning llm-exec branch)
  - Steps include build-gate-failed before healing and a final build-gate-succeeded (or diff-applied + MR)

LLM runner event expectations (/v1/mods/{id}/events)

- Planner and reducer jobs should post events like the orw-apply task does. Reference services/langgraph-runner/entrypoint.sh for examples:
  - POST ${PLOY_CONTROLLER}/mods/${EXECUTION_ID}/events with JSON body:
    {"execution_id":"…","phase":"planner","step":"planner","level":"info","message":"job started","job_name":"…"}
  - On exit, post either level=info message=job completed or level=error message=job failed.
- orw-apply events are already emitted by services/openrewrite-jvm/runner.sh. Use the same endpoint and schema.

Tips and knobs

- See docs/mods/knobs.md for resource and image knobs, and continue.md for the latest behavior around SeaweedFS artifacts and event stream.
- Images must point to internal registry on VPS (MODS_*_IMAGE). Do not use public registries in VPS flows.
- For Go E2E, ensure env is set appropriately; tests will Skip with clear messages if not configured.
- To inspect VPS runtime (optional), ssh root@$TARGET_HOST; su - ploy; then use /opt/hashicorp/bin/nomad-job-manager.sh helpers to inspect recent allocs. Do not deploy from VPS.

Manual verification commands

- After run.sh prints EXEC_ID, in another terminal:
  - curl -s "$PLOY_CONTROLLER/mods/$EXEC_ID/status" | jq .
  - curl -s "$PLOY_CONTROLLER/mods/$EXEC_ID/artifacts" | jq .
  - curl -sN "$PLOY_CONTROLLER/mods/$EXEC_ID/logs?follow=1"

Cleanup

- Merge or close the test MR in your fork
- Remove test branches as needed
- Local: delete ./logs/<EXEC_ID>/ directories

Troubleshooting

- No artifacts: ensure SeaweedFS is reachable from jobs and controller (PLOY_SEAWEEDFS_URL). See continue.md for details.
- No SSE events: verify runners post to /v1/mods/{id}/events and that PLOY_CONTROLLER is set in job HCL substitution (see internal/mods/job_submission.go substituteHCLTemplate).
- Planner/reducer template images: check MODS_PLANNER_IMAGE, MODS_REDUCER_IMAGE, and MODS_LLM_EXEC_IMAGE env in API service; they must reference the internal registry.
