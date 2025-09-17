# Mods Runtime Knobs

- Controller substitution: internal/mods/execution.go
- Event push endpoint: /v1/mods/:id/events

Environment examples:
- `MODS_MODEL=gpt-4o-mini ./bin/ploy mod plan --preserve`
- `MODS_TOOLS='{"file":{"allow":["src/**","pom.xml","build.gradle"]}}' ./bin/ploy mod plan`
- `MODS_LIMITS='{"max_steps":4,"max_tool_calls":6,"timeout":"10m"}' ./bin/ploy mod reduce`

\n---\n(Appended from original docs/mods/knobs.md)\n
 Mods Knobs

- orw-apply resources
    - memory: 1024 MB (template: platform/nomad/mods/orw_apply.hcl)
    - cpu: 300 (same template)
    - When to change: if Maven/OpenRewrite OOMs (increase) or cluster is tight (decrease carefully with heap tuning).
    - When to change: if Maven/OpenRewrite OOMs (increase) or cluster is tight (decrease carefully with heap tuning).
-
JVM/Maven heap for orw-apply
    - env: MAVEN_OPTS="-Xms256m -Xmx768m" (optional; set inside orw_apply.hcl env if needed)
    - Guidance: keep Xmx comfortably below the task memory to avoid cgroup OOM.
-
Recipe configuration
    - RECIPE (class), RECIPE_GROUP, RECIPE_ARTIFACT, RECIPE_VERSION, MAVEN_PLUGIN_VERSION
    - Set via mods YAML step; runner requires explicit coords, no discovery.
-
Controller/registration endpointst   ⌃C quit   2450012 tokens used   1% context left
    - PLOY_CONTROLLER: https://api.dev.ployman.app/v1 (used by runner/controller)
    - PLOY_API_URL: https://api.dev.ployman.app (controller base; runner uses for recipe metadata POST)
    - Where set: PLOY_CONTROLLER is passed from CLI; PLOY_API_URL is auto-derived by controller HCL substitution.
-
Storage endpoints
    - PLOY_SEAWEEDFS_URL: http://seaweedfs-filer.service.consul:8888
    - DIFF_KEY: mods//branches//steps//diff.patch (auto-set per run)
    - Artifact key policy: keys must start with `mods/` and cannot contain `..` or backslashes. Non-conforming keys are rejected client-side.
-
Images
    - MODS_ORW_APPLY_IMAGE: registry.dev.ployman.app/openrewrite-jvm:latest
    - MODS_PLANNER_IMAGE / MODS_REDUCER_IMAGE / MODS_LLM_EXEC_IMAGE: registry.dev.ployman.app/langgraph-runner:latest
    - MODS_REGISTRY: registry.dev.ployman.app (fallback registry prefix)
-
Nomad/DC
    - NOMAD_DC: dc1 (used in templates)
-
Git provider
    - GITLAB_URL, GITLAB_TOKEN (write_repository/api scope). Used for push/MR.
    - GIT_AUTHOR_NAME/GIT_AUTHOR_EMAIL, GIT_COMMITTER_NAME/GIT_COMMITTER_EMAIL (defaults to `Ploy Bot <ploy-bot@dev.ployman.app>` if unset).
-
Build gate teardown (ephemeral lane‑c)
    - build_only=true (query param set by the build checker)
    - Behavior: API deregisters the lane‑c job after the build gate passes to avoid leftovers.

Where To Set

- Persistent service env (VPS): /home/ploy/api.env
    - Managed by Ansible: iac/dev/playbooks/api-env.yml (prefers workstation GITLAB_TOKEN; writes NOMAD_DC, PLOY_SEAWEEDFS_URL, MODS_IMAGEs, MODS_REGISTRY, GIT_AUTHOR/GIT_COMMITTER with sane defaults).
    - `MODS_SKIP_DEPLOY_LANES` (comma-separated lane codes) optionally skips runtime deployment for specific lanes (leave unset to exercise full deploy). Override with `C` only when remote lane-C deploy is unavailable and compile-only validation is desired.
- Job template (orw-apply): platform/nomad/mods/orw_apply.hcl
    - Resources, env block (ORW_IMAGE, PLOY_API_URL, SEAWEEDFS_URL, INPUT_URL, DIFF_KEY, controller/exec IDs).
- Controller substitution: internal/mods/execution.go
    - Computes ${ORW_IMAGE}, ${INPUT_URL}, ${PLOY_API_URL}, ${DIFF_KEY}, etc.

Recommended Defaults (dev)

- orw-apply: memory=1024 MB, cpu=300; MAVEN_OPTS unset unless needed (then -Xmx768m).
- MODS_ORW_APPLY_IMAGE: registry.dev.ployman.app/openrewrite-jvm:latest
- MODS_PLANNER/REDUCER/LLM_EXEC_IMAGE: registry.dev.ployman.app/langgraph-runner:latest
- NOMAD_DC=dc1, PLOY_SEAWEEDFS_URL=http://seaweedfs-filer.service.consul:8888
- PLOY_CONTROLLER: ensure it is set to `https://api.dev.ployman.app/v1` for Dev; `PLOY_API_URL` auto-derived to https://api.dev.ployman.app
- GITLAB_URL=https://gitlab.com, GITLAB_TOKEN=glpat-… (write scope); Git identity defaults to `Ploy Bot <ploy-bot@dev.ployman.app>` unless overridden. Only set `MODS_SKIP_DEPLOY_LANES` when you need to bypass remote deployments (e.g., lane-C compile-only dry runs).

Per-run MR auth selection (mods.yaml)

- To choose which environment variables power Git push/MR for a specific mods run without placing secrets in YAML, use the `mr` block with env var names:

  mr:
    forge: gitlab
    repo_url_env: GITLAB_URL
    token_env: GITLAB_TOKEN
    labels: ["ploy", "tfl", "healing-llm"]

- Behavior:
  - `repo_url_env`: name of the env var that holds the GitLab base URL (e.g., https://gitlab.com). The runner maps it to `GITLAB_URL` internally.
  - `token_env`: name of the env var that holds the token. The runner maps it to `GITLAB_TOKEN` internally. Token must have write_repository (and usually api) scopes.
  - `labels`: optional MR labels override; defaults to ["ploy", "tfl"].

- Notes:
  - Only env var NAMES are specified in YAML; values remain in the environment on the controller/VPS.
  - The runner emits info/warn events noting which env names were used, never the secret values.

Notes

- PLOY_API_URL vs PLOY_CONTROLLER: PLOY_API_URL is the base (no /v1) used by in-job HTTP calls (e.g., recipe registration); PLOY_CONTROLLER includes /v1 and is used by runner/controller for control-plane events.
- Cleanup is automatic: the build gate sets build_only so the API deregisters the lane‑c sandbox after the build gate, preventing mod-…-lane-c leftovers.
- For production/staging, mirror the same knobs but point images to the appropriate registry and increase resources if projects are larger.

Centralized Defaults (helpers)
- ResolveImagesFromEnv: resolves planner/reducer/llm/orw image refs and registry using Defaults fallbacks.
- ResolveInfraFromEnv: resolves controller, DC, and SeaweedFS with Defaults; also derives API base (controller without `/v1`).
- ResolveLLMDefaultsFromEnv: resolves `MODS_MODEL`, `MODS_TOOLS`, and `MODS_LIMITS` with sensible built-in defaults.
- These helpers back all var maps (planner/reducer/LLM/ORW) across preview, fanout, and production submission flows, replacing ad‑hoc environment lookups.

Example var maps used for HCL substitution

- Planner/Reducer/LLM (substituteHCLTemplateWithMCPVars):
  - MODS_CONTEXT_DIR, MODS_OUT_DIR
  - MODS_REGISTRY, MODS_PLANNER_IMAGE, MODS_REDUCER_IMAGE, MODS_LLM_EXEC_IMAGE
  - MODS_MODEL, MODS_TOOLS, MODS_LIMITS (resolved via ResolveLLMDefaultsFromEnv; env overrides honored)
  - PLOY_CONTROLLER (from ResolveInfra), MOD_ID, NOMAD_DC (from ResolveInfra)

- ORW Apply (substituteORWTemplateVars):
  - MODS_CONTEXT_DIR, MODS_OUT_DIR
  - MODS_ORW_APPLY_IMAGE, MODS_REGISTRY (from ResolveImages)
  - PLOY_CONTROLLER, PLOY_SEAWEEDFS_URL, NOMAD_DC (from ResolveInfra)
  - MODS_DIFF_KEY (branch-scoped step diff key)
  - Internally derived by template helper: PLOY_API_URL (from PLOY_CONTROLLER, no `/v1`), INPUT_KEY and INPUT_URL for input.tar

Notes
- Always pass explicit var maps to substitution helpers; do not mutate process-wide environment.
- Prefer Defaults/Resolvers for values that have sensible platform fallbacks (registry/images/seaweed/DC/controller).

LLM Defaults Reference
- Model default: `gpt-4o-mini@2024-08-06`
- Tools default: `{"file":{"allow":["src/**","pom.xml"]},"search":{"provider":"rg","allow":["src/**"]}}`
- Limits default: `{"max_steps":8,"max_tool_calls":12,"timeout":"30m"}`

Override examples (per run):
- `MODS_MODEL=gpt-4o-mini ./bin/ploy mod plan --preserve`
- `MODS_TOOLS='{"file":{"allow":["src/**","pom.xml","build.gradle"]}}' ./bin/ploy mod plan`
- `MODS_LIMITS='{"max_steps":4,"max_tool_calls":6,"timeout":"10m"}' ./bin/ploy mod reduce`
# Mods Configuration Knobs

The following environment variables control Mods defaults. All are optional; sensible defaults are provided.

Registry and Images
- MODS_REGISTRY: Default registry (default: registry.dev.ployman.app)
- MODS_PLANNER_IMAGE: Planner job image (default: <REGISTRY>/langgraph-runner:latest)
- MODS_REDUCER_IMAGE: Reducer job image (default: same as planner)
- MODS_LLM_EXEC_IMAGE: LLM exec job image (default: same as planner)
- MODS_ORW_APPLY_IMAGE: OpenRewrite apply image (default: <REGISTRY>/openrewrite-jvm:latest)

Infrastructure
- NOMAD_DC: Nomad datacenter (default: dc1)
- PLOY_SEAWEEDFS_URL: SeaweedFS filer URL (default: http://seaweedfs-filer.service.consul:8888)

Security and Paths
- MODS_ALLOWLIST: CSV allowlist globs for diff validation (default: "src/**,pom.xml")

Timeouts
- MODS_PLANNER_TIMEOUT: Planner job timeout (default: 15m)
- MODS_REDUCER_TIMEOUT: Reducer job timeout (default: 10m)
- MODS_LLM_EXEC_TIMEOUT: LLM exec job timeout (default: 30m)
- MODS_ORW_APPLY_TIMEOUT: ORW apply job timeout (default: 30m)
- MODS_BUILD_APPLY_TIMEOUT: Apply-diff + build-gate phase timeout (default: 10m)

Behavior
- MODS_ALLOW_PARTIAL_ORW: Allow continuing when ORW job reports failure but produced a non-empty diff.patch (default: false). Accepts true/false/1/0/yes/no.

Notes
- CLI may still respect explicit configuration options in YAML; env vars only provide defaults.
- The controller event reporter is used when <MOD_ID> is set.

How to add new HCL template variables

- Prefer central resolvers instead of reading environment variables directly:
  - Use `ResolveImagesFromEnv()` to get image refs and registry.
  - Use `ResolveInfraFromEnv()` to get controller (and API base), SeaweedFS URL, and DC.
- Build var maps explicitly (do not mutate process env) and pass them to:
  - `substituteHCLTemplateWithMCPVars` for planner/reducer/llm-exec.
  - `substituteORWTemplateVars` for ORW apply jobs.
- If a new template needs an additional var:
  1) Add the key in the relevant resolver if it derives from defaults (images/infra).
  2) Thread the value through the var map where the template is substituted (preview/fanout/runner).
  3) Add a small unit test for the substitution to avoid regressions.
