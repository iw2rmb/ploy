 Transflow Knobs

- orw-apply resources
    - memory: 1024 MB (template: platform/nomad/transflow/orw_apply.hcl)
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
    - Set via transflow YAML step; runner requires explicit coords, no discovery.
-
Controller/registration endpointst   ⌃C quit   2450012 tokens used   1% context left
    - PLOY_CONTROLLER: https://api.dev.ployman.app/v1 (used by runner/controller)
    - PLOY_API_URL: https://api.dev.ployman.app (controller base; runner uses for recipe metadata POST)
    - Where set: PLOY_CONTROLLER is passed from CLI; PLOY_API_URL is auto-derived by controller HCL substitution.
-
Storage endpoints
    - PLOY_SEAWEEDFS_URL: http://seaweedfs-filer.service.consul:8888
    - DIFF_KEY: transflow//branches//steps//diff.patch (auto-set per run)
    - Artifact key policy: keys must start with `transflow/` and cannot contain `..` or backslashes. Non-conforming keys are rejected client-side.
-
Images
    - TRANSFLOW_ORW_APPLY_IMAGE: registry.dev.ployman.app/openrewrite-jvm:latest
    - TRANSFLOW_PLANNER_IMAGE / TRANSFLOW_REDUCER_IMAGE / TRANSFLOW_LLM_EXEC_IMAGE: registry.dev.ployman.app/langgraph-runner:py-0.1.0
    - TRANSFLOW_REGISTRY: registry.dev.ployman.app (fallback registry prefix)
-
Nomad/DC
    - NOMAD_DC: dc1 (used in templates)
-
Git provider
    - GITLAB_URL, GITLAB_TOKEN (write_repository/api scope). Used for push/MR.
-
Build gate teardown (ephemeral lane‑c)
    - build_only=true (query param set by transflow build checker)
    - Behavior: API deregisters the lane‑c job after the build gate passes to avoid leftovers.

Where To Set

- Persistent service env (VPS): /home/ploy/api.env
    - Managed by Ansible: iac/dev/playbooks/api-env.yml (prefers workstation GITLAB_TOKEN; writes NOMAD_DC, PLOY_SEAWEEDFS_URL, TRANSFLOW_IMAGEs, TRANSFLOW_REGISTRY, GIT_AUTHOR).
- Job template (orw-apply): platform/nomad/transflow/orw_apply.hcl
    - Resources, env block (ORW_IMAGE, PLOY_API_URL, SEAWEEDFS_URL, INPUT_URL, DIFF_KEY, controller/exec IDs).
- Controller substitution: internal/cli/transflow/execution.go
    - Computes ${ORW_IMAGE}, ${INPUT_URL}, ${PLOY_API_URL}, ${DIFF_KEY}, etc.

Recommended Defaults (dev)

- orw-apply: memory=1024 MB, cpu=300; MAVEN_OPTS unset unless needed (then -Xmx768m).
- TRANSFLOW_ORW_APPLY_IMAGE: registry.dev.ployman.app/openrewrite-jvm:latest
- TRANSFLOW_PLANNER/REDUCER/LLM_EXEC_IMAGE: registry.dev.ployman.app/langgraph-runner:py-0.1.0
- NOMAD_DC=dc1, PLOY_SEAWEEDFS_URL=http://seaweedfs-filer.service.consul:8888
- PLOY_CONTROLLER=https://api.dev.ployman.app/v1 (CLI), PLOY_API_URL auto-derived to https://api.dev.ployman.app
- GITLAB_URL=https://gitlab.com, GITLAB_TOKEN=glpat-… (write scope)

Notes

- PLOY_API_URL vs PLOY_CONTROLLER: PLOY_API_URL is the base (no /v1) used by in-job HTTP calls (e.g., recipe registration); PLOY_CONTROLLER includes /v1 and is used by runner/controller for control-plane events.
- Cleanup is automatic: transflow sets build_only so the API deregisters the lane‑c sandbox after the build gate, preventing tfw-…-lane-c leftovers.
- For production/staging, mirror the same knobs but point images to the appropriate registry and increase resources if projects are larger.

Centralized Defaults (helpers)
- ResolveImagesFromEnv: resolves planner/reducer/llm/orw image refs and registry using Defaults fallbacks.
- ResolveInfraFromEnv: resolves controller, DC, and SeaweedFS with Defaults; also derives API base (controller without `/v1`).
- These helpers back all var maps (planner/reducer/LLM/ORW) across preview, fanout, and production submission flows, replacing ad‑hoc environment lookups.

Example var maps used for HCL substitution

- Planner/Reducer/LLM (substituteHCLTemplateWithMCPVars):
  - TRANSFLOW_CONTEXT_DIR, TRANSFLOW_OUT_DIR
  - TRANSFLOW_REGISTRY, TRANSFLOW_PLANNER_IMAGE, TRANSFLOW_REDUCER_IMAGE, TRANSFLOW_LLM_EXEC_IMAGE
  - TRANSFLOW_MODEL, TRANSFLOW_TOOLS, TRANSFLOW_LIMITS (optional)
  - PLOY_CONTROLLER (from ResolveInfra), PLOY_TRANSFLOW_EXECUTION_ID, NOMAD_DC (from ResolveInfra)

- ORW Apply (substituteORWTemplateVars):
  - TRANSFLOW_CONTEXT_DIR, TRANSFLOW_OUT_DIR
  - TRANSFLOW_ORW_APPLY_IMAGE, TRANSFLOW_REGISTRY (from ResolveImages)
  - PLOY_CONTROLLER, PLOY_SEAWEEDFS_URL, NOMAD_DC (from ResolveInfra)
  - TRANSFLOW_DIFF_KEY (branch-scoped step diff key)
  - Internally derived by template helper: PLOY_API_URL (from PLOY_CONTROLLER, no `/v1`), INPUT_KEY and INPUT_URL for input.tar

Notes
- Always pass explicit var maps to substitution helpers; do not mutate process-wide environment.
- Prefer Defaults/Resolvers for values that have sensible platform fallbacks (registry/images/seaweed/DC/controller).
# Transflow Configuration Knobs

The following environment variables control Transflow defaults. All are optional; sensible defaults are provided.

Registry and Images
- TRANSFLOW_REGISTRY: Default registry (default: registry.dev.ployman.app)
- TRANSFLOW_PLANNER_IMAGE: Planner job image (default: <REGISTRY>/langgraph-runner:py-0.1.0)
- TRANSFLOW_REDUCER_IMAGE: Reducer job image (default: same as planner)
- TRANSFLOW_LLM_EXEC_IMAGE: LLM exec job image (default: same as planner)
- TRANSFLOW_ORW_APPLY_IMAGE: OpenRewrite apply image (default: <REGISTRY>/openrewrite-jvm:latest)

Infrastructure
- NOMAD_DC: Nomad datacenter (default: dc1)
- PLOY_SEAWEEDFS_URL: SeaweedFS filer URL (default: http://seaweedfs-filer.service.consul:8888)

Security and Paths
- TRANSFLOW_ALLOWLIST: CSV allowlist globs for diff validation (default: "src/**,pom.xml")

Timeouts
- TRANSFLOW_PLANNER_TIMEOUT: Planner job timeout (default: 15m)
- TRANSFLOW_REDUCER_TIMEOUT: Reducer job timeout (default: 10m)
- TRANSFLOW_LLM_EXEC_TIMEOUT: LLM exec job timeout (default: 30m)
- TRANSFLOW_ORW_APPLY_TIMEOUT: ORW apply job timeout (default: 30m)
- TRANSFLOW_BUILD_APPLY_TIMEOUT: Apply-diff + build-gate phase timeout (default: 10m)

Behavior
- TRANSFLOW_ALLOW_PARTIAL_ORW: Allow continuing when ORW job reports failure but produced a non-empty diff.patch (default: false). Accepts true/false/1/0/yes/no.

Notes
- CLI may still respect explicit configuration options in YAML; env vars only provide defaults.
- The controller event reporter is used when PLOY_TRANSFLOW_EXECUTION_ID is set.

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
