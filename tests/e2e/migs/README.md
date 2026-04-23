**Migs E2E (Java 11→17) — Ploy Next**

- Goal: Recreate the historic Migs E2E using the current Ploy implementation (own job orchestration + integrated Build Gate) and the original sample repo ploy-orw-java11-maven. Two scenarios:
  - OpenRewrite apply upgrades Java 11→17 and Build Gate passes.
  - Same apply, but first Build Gate fails which triggers a healing loop (Codex-based healer) before Build Gate re-runs.

**Prereqs**

- Local Docker cluster deployed via `ploy cluster deploy`.
- CLI configured for the local cluster:
  - `export PLOY_CONFIG_HOME="${PLOY_CONFIG_HOME:-$HOME/.config/ploy}"`
  - Scenario scripts auto-rebuild/repair `default` and validate the bearer token before run submission.
  - Repair first tries `cmd/ploy/assets/runtime/generated-tokens.env`, then mints a local admin token from known local secrets when needed.
  - If both descriptor and token seed are missing, rerun `ploy cluster deploy`.
- GitLab access for the sample repo's MRs: export `PLOY_GITLAB_PAT` (or set via cluster's signer if configured).
- Optional: `PLOY_OPENAI_API_KEY` if you bring a real LLM; the provided E2E images include a deterministic llm "healer" stub that does not call external APIs.

**Build + Publish Migs Images (Local Registry)**

- Build Migs images (requires Docker):
  - OpenRewrite CLI (Maven): `docker buildx build --platform linux/amd64 -f images/orw/orw-cli-maven/Dockerfile -t orw-cli-maven:e2e .`
  - OpenRewrite CLI (Gradle): `docker buildx build --platform linux/amd64 -f images/orw/orw-cli-gradle/Dockerfile -t orw-cli-gradle:e2e .`
  - Codex+Amata Maven lane: from repo root run `bash images/amata/build-amata.sh`, then `docker buildx build --platform linux/amd64 -f images/amata/java-17-codex-amata-maven/Dockerfile -t java-17-codex-amata-maven:e2e .`
  - Optional: `migs-llm`, `migs-plan` as needed.
- Push to local registry using the helper script:
  - `IMAGE_PREFIX=localhost:5000/ploy VERSION=v0.1.0 images/build-and-push.sh`
  - The script pushes `java-17-codex-amata-maven`, `java-17-codex-amata-gradle`, `orw-cli-maven`, `orw-cli-gradle`, plus `server` and `node`.
  - Images publish as `$IMAGE_PREFIX/<name>:<tag>`.

Notes:
- Directory→repo mapping: `mig-foo` (folder) corresponds to registry repo `ploy/migs-foo`; `orw-cli-maven`/`orw-cli-gradle` keep their directory names.
- The LLM image is a safe E2E stub: when it sees the sample’s failing branch, it creates `src/main/java/e2e/UnknownClass.java` to fix the compile.
- The Codex healer now uses a **workspace diff handshake**: Codex edits the workspace and exits when done. The node agent then inspects the workspace via `git status --porcelain` and only re-runs the Build Gate externally when changes are present. Codex no longer invokes Build Gate tooling directly from inside the container.
- ORW runtime isolation contract: OpenRewrite runs through `orw-cli` only; `transform.log` must not contain `rewriteRun` or `rewrite-maven-plugin:run`.

See also:
- `docs/how-to/publish-migs.md` for end-to-end Migs image publishing via CLI.

**Sample Repository**

- Canonical E2E target: `https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git`.
  - Passing baseline branch: `main`.
  - Failing baseline branch: `e2e/fail-missing-symbol` (references `UnknownClass`, ensuring the first compile fails).

**CLI Build**

- Build and place the CLI in `dist/ploy`:
  - `make build`
  - Smoke tests locally: `make test` (unit + guardrails). E2E runs target the cluster.

**Prep Lifecycle Scenarios (Track 1)**

Run the prep lifecycle scripts to validate prep orchestration and run gating:

- Happy path (`PrepPending -> PrepRunning -> PrepReady`):
  - `bash tests/e2e/migs/scenario-prep-ready.sh`
- Failure path (`PrepPending -> PrepRunning -> PrepFailed`):
  - `bash tests/e2e/migs/scenario-prep-fail.sh`

These scenarios assert that repo jobs are not created before `PrepReady`, and that
failure metadata/evidence is exposed through `GET /v1/repos/{repo_id}/prep`.
Local deploy uses a deterministic prep fixture in the server image (`codex` stub):
- refs containing `fail` force prep failure
- all other refs emit a valid gate profile and transition to `PrepReady`
- scripts submit via `ploy run --spec` (single-repo run API surface)
- default repo/refs are public and deterministic:
  - ready: `https://github.com/octocat/Hello-World.git`, `master -> master`
  - fail: `https://github.com/octocat/Hello-World.git`, `prep-fail-trigger -> master`
- override with:
  - `PLOY_E2E_REPO_OVERRIDE`
  - `PLOY_E2E_BASE_REF`
  - `PLOY_E2E_TARGET_REF`

**Spec‑Driven Flow (recommended)**

Use the YAML spec to define mig parameters, Build Gate, and healing.
Example spec:
  - `tests/e2e/migs/scenario-orw-fail/mig.yaml`

**Using `--spec`:**

The `--spec` flag accepts a YAML or JSON file defining:
- **Main mig configuration** (`image`, `command`, `envs`, `ca`, `in`, `out`, `home`)
- **Build Gate settings** (`build_gate.enabled`, `build_gate.images`)
- **Healing configuration** (`build_gate.heal`)
- **GitLab MR integration** (`gitlab_domain`, `gitlab_pat`, `mr_on_success`, `mr_on_fail`)

CLI flags override spec values when both are present. For example:
```bash
ploy mig run --spec mig.yaml --job-image custom:tag --gitlab-pat "$TOKEN"
```
This uses `mig.yaml` as the base but overrides the image and PAT.

**Build Gate Healing:**

When `build_gate.heal` is configured in the spec:
1. The node runs the Build Gate before the main mig.
2. If the gate fails, the healing action under `build_gate.heal` executes.
3. After the healing mig completes, the gate is re-run. If it passes, the main mig proceeds.
4. The loop retries up to `build_gate.heal.retries` (default: 1).
5. If the gate still fails after retries, the run terminates with `status=failed` and `reason=build-gate`.

**Repo+Diff Verification Semantics:**

Healing verification uses the same repo+diff semantics as the unified jobs-based Build Gate:

- **Initial workspace**: The Build Gate validates code cloned from `repo_url+ref`.
- **Healing modifications**: Healing migs modify the workspace in-place. Changes accumulate as diffs on top of the repo baseline.
- **Re-gate verification**: After healing, the gate re-runs against `workspace = repo_url+ref + healing changes` using the local Docker gate executor (no HTTP Build Gate API call).
- **Diff chain**: Workspace state equals base clone + ordered diff sequence. This matches Migs multi-step execution where each step's changes can be replayed for rehydration.

**Codex Healing Handshake (workspace diff):**

The recommended approach for Codex-based healing is the workspace diff handshake. Codex edits the workspace and, when ready for validation, simply exits. The node agent re-runs the Build Gate externally after healing completes only when workspace diffs exist; a clean workspace (no diff) means no re-gate and the run remains failed.

**Codex Healing Handshake Checklist (Validation):**

Validate the following artifact after Codex-based healing runs:

| Artifact | Location | Description | Required |
|----------|----------|-------------|----------|
| Healing summary | `heal.json` | Last assistant message summary | Required |

**Validation steps:**

1. **Workspace diff driven re-gate**: After each healing attempt, verify that:
   - Healing migs edit files under `/workspace` as needed to fix the failure.
   - The node agent re-runs the Build Gate only when workspace diffs are present (`git status --porcelain` non-empty).
   - When healing performs no net changes (clean `git status`), the gate is not re-run and the run terminates as failed.

2. **Healing summary artifact**:
   - `heal.json` is written to `/out`
   - File may be empty when the healing tool does not emit a final summary payload

Cross-reference: `AGENTS.md` and `docs/testing-workflow.md`.

**Cross-phase inputs available to healing migs:**
- `/in/build-gate.log` — First Build Gate failure log (read-only mount)

**Environment variables injected by the node agent for healing migs:**
- `PLOY_REPO_URL` — Git repository URL (same as the Migs run)
- `PLOY_BUILDGATE_REF` — Git ref for Build Gate baseline (base_ref or commit_sha)
- `PLOY_HOST_WORKSPACE` — Host path to workspace (for direct host verification)
- `PLOY_SERVER_URL` — ploy control plane base URL

Healing containers support amata execution mode:

**amata mode** (recommended): set `amata.spec` — no prompt file required.
The node agent materializes the spec as `/in/amata.yaml` and runs
`amata run /in/amata.yaml` with optional `--set` flags from `amata.set`.

Example healing spec block:
```yaml
build_gate:
  enabled: true
  heal:
    retries: 1
    image: ghcr.io/iw2rmb/ploy/java-17-codex-amata-maven:latest
    amata:
      spec: |
        version: amata/v1
        name: code-healer
        entry: main
        workspace:
          root: /workspace
        flows:
          main:
            steps:
              - codex: |
                  Fix the build failure in /in/build-gate.log.
                  Your final message MUST be one line of JSON: {"action_summary":"..."}
    home:
      - ~/.codex/auth.json:.codex/auth.json
```

See `docs/schemas/mig.example.yaml` for the full spec schema.

Run the failing→healing scenario with a single script:
  - `bash tests/e2e/migs/scenario-orw-fail/run.sh`
  - It submits:
    - `--repo-url https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git`
    - `--repo-base-ref e2e/fail-missing-symbol`
    - `--repo-target-ref migs-upgrade-java17-heal`
    - `--spec tests/e2e/migs/scenario-orw-fail/mig.yaml`
    - `--follow --artifact-dir ./tmp/migs/scenario-orw-fail/<ts>`

What to verify:
- First Build Gate fails (Maven compile error), healing runs using `amata` with the workspace diff handshake, the node agent detects workspace diffs, re-runs the Build Gate, then ORW proceeds.

**Notes**

When `amata` runs inside the repository directory (`/workspace`), it uses the mounted repo directly. With the workspace diff handshake, the healer edits code and exits; the node agent handles gate execution and only re-runs the gate when workspace diffs are present.

Cross-phase inputs are mounted at `/in` (read-only):
- `/in/build-gate.log` — First Build Gate failure log, available for healing migs to reference

What to expect with the provided E2E images:
- Spec-driven healing runs with `amata`; artifacts across stages are attached to the run and can be downloaded via `--artifact-dir`.

**Follow Mode (`--follow`) and Job Graph**

The CLI `--follow` flag displays a summarized per-repo job graph that refreshes until
the run reaches a terminal state. The job graph shows:
- Repo count, per-repo blocks, and step rows with status glyph, step, job ID, node, image, and duration.
- For failures, a one-line error is shown directly under the failed step row.

**Note:** `--follow` does not stream container stdout/stderr. Use `ploy job follow <job-id>`
for container log streaming. `ploy run logs <run-id>` shows lifecycle events only.

The follow engine subscribes to SSE events from `/v1/runs/{id}/logs` for change
notifications and refreshes the job graph on each event. SSE reconnection uses:
- **Automatic reconnection**: On connection errors or mid-stream failures, the client reconnects with exponential backoff (250ms initial, 2x multiplier, capped at 30s).
- **Max retries**: Default `5` reconnect attempts. Use `--max-retries <n>` to change.
- **Time cap**: Use `--cap <duration>` to limit follow time. Add `--cancel-on-cap` to cancel the run when the cap is exceeded.

Tip: The CLI can also fetch artifacts via `--artifact-dir` when the run succeeds.

**Lifecycle Log Streaming (`ploy run logs`)**

`ploy run logs <run-id>` streams run lifecycle events (state transitions, scheduling, completion).
For container stdout/stderr, use `ploy job follow <job-id>` instead. The lifecycle log stream supports:
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
  - CLI reads the default descriptor at `~/.config/ploy` (no env override).
- Build Gate image override:
  - To change the Build Gate executor container image, use `PLOY_BUILDGATE_IMAGE` on worker nodes.

**Troubleshooting**

- Images not found / pull errors:
  - Ensure images are pushed to the configured `$PLOY_CONTAINER_REGISTRY` (default: `ghcr.io/iw2rmb/ploy`) and the node host Docker daemon can pull them.
  - If you switch to a private remote registry, provide pull auth via `DOCKER_AUTH_CONFIG`/`PLOY_DOCKER_AUTH_CONFIG`.
- Git access / MR creation:
  - Export `PLOY_GITLAB_PAT` and confirm the control plane has connectivity to GitLab. The sample repo is public for read; MRs require auth for branch writes.
- Build Gate keeps failing in Scenario B:
  - Confirm the `migs-llm` image version the cluster pulls includes the healer stub. Re-publish if needed.
- Monitoring runs:
  - Use `--follow` to display the job graph until completion.
  - Use `ploy job follow <job-id>` to stream container stdout/stderr.
  - Check the control plane logs if stages appear stuck (cluster scheduling/resources).

**Multi-Step, Multi-Node Rehydration Scenario**

E2E validation for multi-step Migs runs with multi-node execution and workspace rehydration:
  - `bash tests/e2e/migs/scenario-multi-node-rehydration/run.sh`

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
- Artifacts saved to `./tmp/migs/scenario-multi-node-rehydration/<timestamp>/`

Skip artifact collection for faster runs (e.g., in CI):
```bash
SKIP_ARTIFACTS=1 bash tests/e2e/migs/scenario-multi-node-rehydration/run.sh
```

**Stack-Aware Image Selection Scenario**

E2E validation for stack-aware image resolution where different container images
are selected based on Build Gate stack detection:
  - `bash tests/e2e/migs/scenario-stack-aware-images/run.sh`

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

Stack resolution rules (from `internal/workflow/contracts/job_image.go`):
1. **Universal image**: If `image` is a string, use it for all stacks.
2. **Exact match**: If `image` is a map and contains the detected stack key, use that.
3. **Default fallback**: If no exact match, use the `default` key when present.
4. **Error**: If neither stack key nor `default` exists, fail with actionable error.

Example stack-aware spec:
```yaml
mig:
  image:
    java-maven: ghcr.io/iw2rmb/ploy/orw-cli-maven:latest
    java-gradle: ghcr.io/iw2rmb/ploy/orw-cli-gradle:latest
  env:
    RECIPE_CLASSNAME: org.openrewrite.java.migrate.UpgradeToJava17
```

To test the error path (missing stack key without default):
```bash
dist/ploy mig run \
  --repo-url https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git \
  --repo-base-ref main \
  --repo-target-ref e2e/stack-aware-error \
  --spec tests/e2e/migs/scenario-stack-aware-images/mig-no-default.yaml \
  --follow
```
Expected error: `no image specified for stack "java-maven" and no default provided`

Skip artifact collection for faster runs (e.g., in CI):
```bash
SKIP_ARTIFACTS=1 bash tests/e2e/migs/scenario-stack-aware-images/run.sh
```

**References**

- Historic E2E assets from prior implementations are found in repo history under `tests/e2e/migs/...` and service Dockerfiles for OpenRewrite. The current implementation replaces that orchestration with an internal job runner and integrated Build Gate. Relevant current references:
  - `internal/workflow/contracts/` — Step manifest shapes and validation.
  - `internal/workflow/runner/job_templates.go` — Migs image bindings for lanes.
  - `internal/workflow/runner/healing.go` — Healing flow appended after Build Gate failures.
  - `internal/nodeagent/execution.go` — Workspace rehydration implementation (base clone + diff chain).
  - `internal/nodeagent/execution_rehydrate_test.go` — Unit tests for rehydration logic.
