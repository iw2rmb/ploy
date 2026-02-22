**Mods E2E (Java 11→17) — Ploy Next**

- Goal: Recreate the historic Mods E2E using the current Ploy implementation (own job orchestration + integrated Build Gate) and the original sample repo ploy-orw-java11-maven. Two scenarios:
  - OpenRewrite apply upgrades Java 11→17 and Build Gate passes.
  - Same apply, but first Build Gate fails which triggers a healing loop (Codex-based healer) before Build Gate re-runs.

**Prereqs**

- Local Docker cluster deployed via `deploy/local/run.sh`.
- CLI configured for the local cluster:
  - `export PLOY_CONFIG_HOME="$PWD/deploy/local/cli"`
  - Scenario scripts auto-rebuild/repair `clusters/default` and validate the bearer token before run submission.
  - Repair first tries `deploy/local/generated-tokens.env`, then mints a local admin token from known local secrets when needed.
  - If both descriptor and token seed are missing, rerun `deploy/local/run.sh`.
- GitLab access for the sample repo's MRs: export `PLOY_GITLAB_PAT` (or set via cluster's signer if configured).
- Optional: `PLOY_OPENAI_API_KEY` if you bring a real LLM; the provided E2E images include a deterministic llm "healer" stub that does not call external APIs.

**Build + Publish Mods Images (Docker Hub)**

- Build Mods images (requires Docker):
  - OpenRewrite Maven: `docker buildx build --platform linux/amd64 -t mods-orw-maven:e2e deploy/images/mods/orw-maven`
  - OpenRewrite Gradle: `docker buildx build --platform linux/amd64 -t mods-orw-gradle:e2e deploy/images/mods/orw-gradle`
  - Codex healer: build from repo root: `docker buildx build --platform linux/amd64 -f deploy/images/mods/mod-codex/Dockerfile -t mods-codex:e2e .`
  - Optional: `mods-llm`, `mods-plan` as needed.
- Push to Docker Hub using the helper script:
  - `DOCKERHUB_USERNAME=<you> DOCKERHUB_PAT=*** deploy/images/build-and-push-mods.sh`
  - The script special‑cases `mod-codex` to use repo‑root context automatically.
  - Images publish as `$PLOY_CONTAINER_REGISTRY/<name>:latest`.

Notes:
- Directory→repo mapping: `mod-foo` (folder) corresponds to registry repo `ploy/mods-foo`; `orw-maven` → `mods-orw-maven`; `orw-gradle` → `mods-orw-gradle`.
- OpenRewrite coordinates are passed via environment: set `RECIPE_GROUP`, `RECIPE_ARTIFACT`, `RECIPE_VERSION`, `RECIPE_CLASSNAME` (and optional `MAVEN_PLUGIN_VERSION`).
- The LLM image is a safe E2E stub: when it sees the sample’s failing branch, it creates `src/main/java/e2e/UnknownClass.java` to fix the compile.
- The Codex healer now uses a **workspace diff handshake**: Codex edits the workspace and exits when done. The node agent then inspects the workspace via `git status --porcelain` and only re-runs the Build Gate externally when changes are present. Codex no longer invokes Build Gate tooling directly from inside the container.

See also:
- `docs/how-to/publish-mods.md` for end-to-end Mods image publishing via CLI.

**Sample Repository**

- Canonical E2E target: `https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git`.
  - Passing baseline branch: `main`.
  - Failing baseline branch: `e2e/fail-missing-symbol` (references `UnknownClass`, ensuring the first compile fails).

**CLI Build**

- Build and place the CLI in `dist/ploy`:
  - `make build`
  - Smoke tests locally: `make test` (unit + guardrails). E2E runs target the cluster.

**Spec‑Driven Flow (recommended)**

Use the YAML spec to define mod parameters, Build Gate, and healing.
Example spec:
  - `tests/e2e/mods/scenario-orw-fail/mod.yaml`

**Using `--spec`:**

The `--spec` flag accepts a YAML or JSON file defining:
- **Main mod configuration** (`image`, `command`, `env`, `env_from_file`)
- **Build Gate settings** (`build_gate.enabled`, `build_gate.images`)
- **Healing configuration** (`build_gate_healing.retries`, `build_gate_healing.mod`)
- **GitLab MR integration** (`gitlab_domain`, `gitlab_pat`, `mr_on_success`, `mr_on_fail`)

CLI flags override spec values when both are present. For example:
```bash
ploy mod run --spec mod.yaml --mod-image custom:tag --gitlab-pat "$TOKEN"
```
This uses `mod.yaml` as the base but overrides the image and PAT.

**Build Gate Healing:**

When `build_gate_healing` is configured in the spec:
1. The node runs the Build Gate before the main mod.
2. If the gate fails, the healing `mod` under `build_gate_healing.mod` executes.
3. After the healing mod completes, the gate is re-run. If it passes, the main mod proceeds.
4. The loop retries up to `build_gate_healing.retries` times (default: 1).
5. If the gate still fails after retries, the run terminates with `status=failed` and `reason=build-gate`.

**Repo+Diff Verification Semantics:**

Healing verification uses the same repo+diff semantics as the unified jobs-based Build Gate:

- **Initial workspace**: The Build Gate validates code cloned from `repo_url+ref`.
- **Healing modifications**: Healing mods modify the workspace in-place. Changes accumulate as diffs on top of the repo baseline.
- **Re-gate verification**: After healing, the gate re-runs against `workspace = repo_url+ref + healing changes` using the local Docker gate executor (no HTTP Build Gate API call).
- **Diff chain**: Workspace state equals base clone + ordered diff sequence. This matches Mods multi-step execution where each step's changes can be replayed for rehydration.

Historically, this repo+diff model was exposed via the HTTP Build Gate API (`POST /v1/buildgate/validate` with `diff_patch`); that API has been removed in favor of the unified jobs pipeline.

**Codex Healing Handshake (workspace diff):**

The recommended approach for Codex-based healing is the workspace diff handshake. Codex edits the workspace and, when ready for validation, simply exits. The node agent re-runs the Build Gate externally after healing completes only when workspace diffs exist; a clean workspace (no diff) means no re-gate and the run remains failed.

**Codex Healing Handshake Checklist (TDD Validation):**

Per RED→GREEN→REFACTOR discipline, the following artifacts should be validated after Codex-based healing runs:

| Artifact | Location | Description | Required |
|----------|----------|-------------|----------|
| Session ID | `codex-session.txt` | Thread ID for resume mode across healing retries | Recommended |
| Run manifest | `codex-run.json` | JSON with `session_id`, `resumed` fields | Required |

**Validation steps:**

1. **Workspace diff driven re-gate**: After each healing attempt, verify that:
   - Healing mods edit files under `/workspace` as needed to fix the failure.
   - The node agent re-runs the Build Gate only when workspace diffs are present (`git status --porcelain` non-empty).
   - When healing performs no net changes (clean `git status`), the gate is not re-run and the run terminates as failed.

2. **Session resume across healing retries**: When `retries > 1` in healing config:
   - After first healing attempt: `codex-session.txt` is written to `/out` with thread ID
   - Before second healing attempt: Session ID is propagated to `/in/codex-session.txt`
   - Subsequent healing mods receive `CODEX_RESUME=1` environment variable
   - `codex.log` shows "resume mode enabled; session=<id>" on retry attempts
   - `codex-run.json` contains `"resumed":true` for resumed runs

3. **Run manifest fields** (`codex-run.json`):
   - `session_id`: Thread ID for conversation continuity (may be empty)
   - `resumed`: `true` if this was a resumed session, `false` otherwise

See `tests/unit/mod_codex_sh_test.sh` for unit tests covering these behaviors.
Cross-reference: `docs/testing-workflow.md` and `AGENTS.md`.

**Cross-phase inputs available to healing mods:**
- `/in/build-gate.log` — First Build Gate failure log (read-only mount)
- `/in/prompt.txt` — Optional prompt file (mounted when provided in spec)

**Environment variables injected by the node agent for healing mods:**
- `PLOY_REPO_URL` — Git repository URL (same as the Mods run)
- `PLOY_BUILDGATE_REF` — Git ref for Build Gate baseline (base_ref or commit_sha)
- `PLOY_HOST_WORKSPACE` — Host path to workspace (for direct host verification)
- `PLOY_SERVER_URL` — ploy server URL for Build Gate HTTP API

**Generating diff patches for Build Gate verification (legacy healers only):**

Legacy (non-Codex) healing mods may optionally generate unified diff patches and
use the repo+diff Build Gate API for mid-healing verification. This avoids
shipping full workspace archives over HTTP:

1. Generate a unified diff of healing changes:
   ```bash
   cd /workspace && git diff > /out/heal.patch
   ```

2. Optionally call the Build Gate HTTP API directly (for non-Codex healers) using the injected `PLOY_*` env vars if you need mid-healing verification.

Example healing spec block (Codex workspace diff handshake, single mod):
```yaml
build_gate_healing:
  retries: 1
  mod:
    image: docker.io/you/mods-codex:latest
    env:
      CODEX_PROMPT: |-
        Rules:
        - Use /workspace and /in/build-gate.log to understand the compile error.
        - Edit files under /workspace as needed to fix the error.
        - When you believe the code is ready for a full build validation, stop editing and end the session.

        Task:
        Fix the compilation error described in /in/build-gate.log.
    env_from_file:
      CODEX_AUTH_JSON: ~/.codex/auth.json
```

See `docs/schemas/mod.example.yaml` for the full spec schema.

Run the failing→healing scenario with a single script:
  - `bash tests/e2e/mods/scenario-orw-fail/run.sh`
  - It submits:
    - `--repo-url https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git`
    - `--repo-base-ref e2e/fail-missing-symbol`
    - `--repo-target-ref mods-upgrade-java17-heal`
    - `--spec tests/e2e/mods/scenario-orw-fail/mod.yaml`
    - `--follow --artifact-dir ./tmp/mods/scenario-orw-fail/<ts>`

What to verify:
- First Build Gate fails (Maven compile error), healing runs using `mods-codex` with the workspace diff handshake—Codex edits the code and exits, the node agent detects workspace diffs and re-runs the Build Gate, then ORW proceeds.

**Notes**

When `mods-codex` runs inside the repository directory (`/workspace`), it uses the mounted repo directly; no separate repo path is required for Codex itself. With the workspace diff handshake, Codex simply edits the code and exits; the node agent handles the actual gate execution and only re-runs the gate when workspace diffs are present.

Cross-phase inputs are mounted at `/in` (read-only):
- `/in/build-gate.log` — First Build Gate failure log, available for healing mods to reference
- `/in/prompt.txt` — Default prompt location (when provided in spec; node mounts it R/O)

What to expect with the provided E2E images:
- Spec-driven healing runs with `mods-codex`; artifacts across stages are attached to the run and can be downloaded via `--artifact-dir`.

**Follow Mode (`--follow`) and Job Graph**

The CLI `--follow` flag displays a summarized per-repo job graph that refreshes until
the run reaches a terminal state. The job graph shows:
- Repo count, per-repo blocks, and step rows with status glyph, step, job ID, node, image, and duration.
- For failures, a one-line error is shown directly under the failed step row.

**Note:** `--follow` does not stream container stdout/stderr. Use `ploy run logs <run-id>`
for log streaming.

The follow engine subscribes to SSE events from `/v1/runs/{id}/logs` for change
notifications and refreshes the job graph on each event. SSE reconnection uses:
- **Automatic reconnection**: On connection errors or mid-stream failures, the client reconnects with exponential backoff (250ms initial, 2x multiplier, capped at 30s).
- **Max retries**: Default `5` reconnect attempts. Use `--max-retries <n>` to change.
- **Time cap**: Use `--cap <duration>` to limit follow time. Add `--cancel-on-cap` to cancel the run when the cap is exceeded.

Tip: The CLI can also fetch artifacts via `--artifact-dir` when the run succeeds.

**Log Streaming (`ploy run logs`)**

For container stdout/stderr streaming, use `ploy run logs <run-id>`. This is the canonical
surface for viewing real-time log output. The log stream supports:
- **Automatic reconnection**: Same backoff policy as follow mode.
- **Last-Event-ID support**: Resumes from the last processed event after reconnect.
- **Idle timeout**: Default `45s` idle timeout. Configure via `--idle-timeout <duration>`.
- **Max retries**: Default `3` reconnect attempts. Use `--max-retries -1` for unlimited.

**Build Gate Status Visibility**

Gate execution results are exposed via:
- `GET /v1/runs/{id}/status` API — Returns gate summary in `RunSummary.Metadata["gate_summary"]` for programmatic access.
- `ploy run status <run-id>` — Human-readable summary of batch status and repo counts.

This makes gate health visible without requiring raw artifact inspection.

**Environment Considerations**

- Cluster targeting:
  - CLI reads the default descriptor at `~/.config/ploy/clusters/` (no env override).
- Build Gate image override:
  - To change the Build Gate executor container image, use `PLOY_BUILDGATE_IMAGE` on worker nodes.

**Troubleshooting**

- Images not found / pull errors:
  - Ensure images are pushed to Docker Hub and nodes can pull them. For private repos, log in on each node: `echo "$DOCKERHUB_PAT" | docker login -u "$DOCKERHUB_USERNAME" --password-stdin`.
- Git access / MR creation:
  - Export `PLOY_GITLAB_PAT` and confirm the control plane has connectivity to GitLab. The sample repo is public for read; MRs require auth for branch writes.
- Build Gate keeps failing in Scenario B:
  - Confirm the `mods-llm` image version the cluster pulls includes the healer stub. Re-publish if needed.
- Monitoring runs:
  - Use `--follow` to display the job graph until completion.
  - Use `ploy run logs <run-id>` to stream container stdout/stderr.
  - Check the control plane logs if stages appear stuck (cluster scheduling/resources).

**Multi-Step, Multi-Node Rehydration Scenario**

E2E validation for multi-step Mods runs with multi-node execution and workspace rehydration:
  - `bash tests/e2e/mods/scenario-multi-node-rehydration/run.sh`

This scenario validates:
- Multi-step execution (3 sequential Java migration steps: Java 8 → 11 → 17)
- Multi-node scheduling: scheduler can assign different steps to different nodes
- Workspace rehydration: nodes correctly reconstruct workspace state from base clone + ordered diffs
- Build gate validation: gate runs after each step with accumulated changes
- MR content: final MR contains cumulative changes from all steps

When running on a multi-node cluster, steps may execute on different nodes. The scheduler creates independent step manifests and nodes claim them as capacity allows. Each node rehydrates the workspace by:
1. Copying the base clone (repo at base_ref + optional commit_sha)
2. Fetching diffs for all prior steps (0 through k-1) from the control plane
3. Applying diffs sequentially using `git apply` to reconstruct workspace state for step k

The scenario passes on both single-node and multi-node clusters. On single-node, all steps execute on the same node but still exercise the rehydration path.

Configuration:
- Uses the public test repository: `https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git`
- Three OpenRewrite transformation steps with build gate validation
- Optional: Set `PLOY_GITLAB_PAT` to validate MR creation with cumulative changes
- Artifacts saved to `./tmp/mods/scenario-multi-node-rehydration/<timestamp>/`

Skip artifact collection for faster runs (e.g., in CI):
```bash
SKIP_ARTIFACTS=1 bash tests/e2e/mods/scenario-multi-node-rehydration/run.sh
```

**Stack-Aware Image Selection Scenario**

E2E validation for stack-aware image resolution where different container images
are selected based on Build Gate stack detection:
  - `bash tests/e2e/mods/scenario-stack-aware-images/run.sh`

This scenario validates:
- Stack-aware image map parsing from the spec
- Build Gate stack detection (java-maven for Maven repositories)
- Image resolution using the exact stack key match
- Fallback to "default" key when exact match is not present
- Clear error messages when neither stack key nor default exists

Configuration:
- Uses the public test repository: `https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git`
- Spec uses stack-aware image map with `default`, `java-maven`, and `java-gradle` keys
- Build Gate detects "java-maven" stack (pom.xml present) and selects corresponding image

Stack resolution rules (from `internal/workflow/contracts/mod_image.go`):
1. **Universal image**: If `image` is a string, use it for all stacks.
2. **Exact match**: If `image` is a map and contains the detected stack key, use that.
3. **Default fallback**: If no exact match, use the `default` key when present.
4. **Error**: If neither stack key nor `default` exists, fail with actionable error.

Example stack-aware spec:
```yaml
mod:
  image:
    default: docker.io/user/mods-orw-maven:latest
    java-maven: docker.io/user/mods-orw-maven:latest
    java-gradle: docker.io/user/mods-orw-gradle:latest
  env:
    RECIPE_CLASSNAME: org.openrewrite.java.migrate.UpgradeToJava17
```

To test the error path (missing stack key without default):
```bash
dist/ploy mod run \
  --repo-url https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git \
  --repo-base-ref main \
  --repo-target-ref e2e/stack-aware-error \
  --spec tests/e2e/mods/scenario-stack-aware-images/mod-no-default.yaml \
  --follow
```
Expected error: `no image specified for stack "java-maven" and no default provided`

Skip artifact collection for faster runs (e.g., in CI):
```bash
SKIP_ARTIFACTS=1 bash tests/e2e/mods/scenario-stack-aware-images/run.sh
```

**References**

- Historic E2E assets from prior implementations are found in repo history under `tests/e2e/mods/...` and service Dockerfiles for OpenRewrite. The current implementation replaces that orchestration with an internal job runner and integrated Build Gate. Relevant current references:
  - `internal/workflow/contracts/` — Step manifest shapes and validation.
  - `internal/workflow/runner/job_templates.go` — Mods image bindings for lanes.
  - `internal/workflow/runner/healing.go` — Healing flow appended after Build Gate failures.
  - `internal/nodeagent/execution.go` — Workspace rehydration implementation (base clone + diff chain).
  - `internal/nodeagent/execution_rehydrate_test.go` — Unit tests for rehydration logic.
