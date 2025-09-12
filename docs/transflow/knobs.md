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