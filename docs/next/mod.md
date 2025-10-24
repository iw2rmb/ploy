# Mods Workflow Example: Java 11 → Java 17 Upgrade

This walkthrough demonstrates how Ploy Next orchestrates a multi-step Mod using GitLab as the source
repository. The Mod upgrades a project from Java 11 to Java 17 using OpenRewrite (ORW) recipes.

## 1. Operator Invokes the Mod

```bash
ploy mod run \
  --repo gitlab://gitlab.example.com/group/project.git@main \
  --mod orw-apply \
  --mod-env MAVEN_PLUGIN_VERSION=6.18.0 \
  --mod-env RECIPE_NAME=org.openrewrite.java.migrate.UpgradeToJava17 \
  --mod-env RECIPE_GROUP=org.openrewrite.recipe \
  --mod-env RECIPE_ARTIFACT=rewrite-migrate-java \
  --mod-env RECIPE_VERSION=3.17.0 \
  --mod-env MAX_RETRIES=2 \
  --mod-env MAX_DEPTH=5 \
  --build-gate-env BUILD_GATE_ENV=value
```

- The CLI resolves the GitLab credential from etcd (uploaded beforehand via `ploy config set
  gitlab.api_key ...`) and embeds the access token in the submission payload.
- The request describes the target repository, the desired Mod sequence (OpenRewrite apply), and the
  recipe metadata via environment variables consumed by the ORW container. Build gate customisation
  is also provided through environment overrides.
- Ploy control plane assigns the Mod ticket to an available node, streaming the plan back to the CLI.

## 2. Node Prepares the Repository

1. The node checks IPFS Cluster for an existing snapshot of the requested repo@HEAD.  
2. If missing, it clones the repository from GitLab using the stored API key and pushes the snapshot
   into IPFS Cluster (capturing the CID for later steps).  
3. The workflow runner hydrates the workspace (base repo + cumulative diffs) via the job service so
   the container sees the exact state expected for the step.

## 3. Initial Build Gate Run

- SHIFT build gate executes unit tests and static analysis against the baseline repository.
- If the build gate fails, the node emits diagnostics and flags the Mod for healing.
- Healing leverages the LLM planner: the node triggers the `llm-plan` step, which produces a refined
  sequence of actions (additional rewrites, dependency adjustments) executed sequentially or in
  parallel as needed. Operators can cap the healing loop with `--mod-env MAX_RETRIES` (number of
  planner invocations for the same failure) and `--mod-env MAX_DEPTH` (maximum depth of a single
  healing path; default 5). When either limit is exceeded the planner stops and the step escalates
  as a failure.
- The node rehydrates the repo with cumulative diffs and repeats the build gate until the plan succeeds or the Mod aborts.

## 4. Execute ORW-Apply Step

- With the baseline verified, the node launches an OpenRewrite container image using the provided recipe and credentials.
- Input includes:
  - Repository state (original tree + any healing diffs).
  - Snapshot artifacts (CIDs) for deterministic checkout.
  - Recipe configuration describing the Java 11 → Java 17 migration.
- The step generates new diffs (e.g., pom updates, code fixes), uploads the bundle and execution log
  to IPFS Cluster through the node’s local cluster client, and records the diff/log CIDs plus digests
  in the job metadata stored in etcd.

## 5. Post-Apply Build Gate

- SHIFT runs again on the updated repository.
- On failure:
  - The node re-enters the healing loop (`llm-plan`) to suggest follow-up steps (e.g., dependency
    bumps, code tweaks), applying each recommended fix and re-running the build gate.
- On success:
  - The node records the Mod stage as complete, uploads the final diff tarball and build gate report
    JSON to IPFS Cluster, and attaches all resulting CIDs/digests to the job outcome. Replicated
    artifacts are immediately available to the CLI and follow-on stages.

## 6. Completion & Output

- The control plane updates the Mod ticket with:
  - Final diff and log CIDs (ready for MR creation or manual review).
  - Build gate report CID and static analysis summaries.
  - Execution metadata (timings, node, container images) captured in the job record.
- Optional post-processing can push the branch/MR back to GitLab using the stored API key.
- The CLI prints a success summary, including a **Stage Artifacts** section listing diff/log CIDs and
  retention TTLs. Operators can immediately inspect outputs with `ploy artifact pull <cid>` or open
  the generated merge request.

## Key Behaviours Highlighted

- **GitLab Integration** — Credentials live in etcd, enabling secure repo cloning and MR operations
  without manual token management on nodes.
- **Artifact Reuse** — Snapshots hydrate locally from cached tarballs and each step’s diff/log bundle
  is replicated to IPFS Cluster in real time, avoiding redundant clones when repeating Mods.
- **Build Gate Enforcement** — Each step runs the SHIFT sandbox automatically; static checks are
  re-enabled once the artifact publisher exposes the detailed reports.
- **Deterministic Replay** — Each step reconstructs repository state from the original HEAD plus
  ordered diffs, ensuring consistent outcomes across nodes.
