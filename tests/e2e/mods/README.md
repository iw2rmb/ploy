**Mods E2E (Java 11→17) — Ploy Next**

- Goal: Recreate the historic Nomad-based Mods E2E using the current Ploy implementation (own job orchestration + integrated Build Gate) and the original sample repo ploy-orw-java11-maven. Two scenarios:
  - OpenRewrite apply upgrades Java 11→17 and Build Gate passes.
  - Same apply, but first Build Gate fails which triggers a healing loop (llm-plan + llm-exec) before Build Gate re-runs.

**Prereqs**

- Ploy cluster descriptor present (CLI auto-discovers from `~/.config/ploy/clusters/default`).
- GitLab access for the sample repo's MRs: export `PLOY_GITLAB_PAT` (or set via cluster's signer if configured).
- Optional: `PLOY_OPENAI_API_KEY` if you bring a real LLM; the provided E2E images include a deterministic llm "healer" stub that does not call external APIs.

**Build + Publish Mods Images (Docker Hub)**

- Build Mods images (requires Docker):
  - OpenRewrite: `docker buildx build --platform linux/amd64 -t mods-openrewrite:e2e docker/mods/mod-orw`
  - Codex healer: build from repo root: `docker buildx build --platform linux/amd64 -f docker/mods/mod-codex/Dockerfile -t mods-codex:e2e .`
  - Optional: `mods-llm`, `mods-plan` as needed.
- Push to Docker Hub using the helper script:
  - `DOCKERHUB_USERNAME=<you> DOCKERHUB_PAT=*** scripts/docker/build-and-push-mods.sh`
  - The script special‑cases `mod-codex` to use repo‑root context automatically.
  - Images publish as `docker.io/$DOCKERHUB_USERNAME/<name>:latest`.

Notes:
- Directory→repo mapping: `mod-foo` (folder) corresponds to registry repo `ploy/mods-foo`. Special-case: `mod-orw` maps to `ploy/mods-openrewrite` to match examples.
- Coordinates are passed via environment only (no JSON manifest support in mod-orw): set `RECIPE_GROUP`, `RECIPE_ARTIFACT`, `RECIPE_VERSION`, `RECIPE_CLASSNAME` (and optional `MAVEN_PLUGIN_VERSION`).
- The LLM image is a safe E2E stub: when it sees the sample’s failing branch, it creates `src/main/java/e2e/UnknownClass.java` to fix the compile.
- The Codex healer now uses the **sentinel protocol**: Codex edits the workspace and emits `[[REQUEST_BUILD_VALIDATION]]` when ready. The node agent then re-runs the Build Gate externally. Codex no longer invokes Build Gate tooling directly from inside the container.

See also:
- `docs/how-to/publish-mods.md` for end-to-end Mods image publishing via CLI.
- `docs/how-to/descriptor-https-quickstart.md` to configure descriptors for HTTPS-only operation.

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
- **Main mod configuration** (`mod.image`, `mod.command`, `mod.env`, `mod.env_from_file`)
- **Build Gate settings** (`build_gate.enabled`, `build_gate.profile`)
- **Healing sequence** (`build_gate_healing.retries`, `build_gate_healing.mods[]`)
- **GitLab MR integration** (`gitlab_domain`, `gitlab_pat`, `mr_on_success`, `mr_on_fail`)

CLI flags override spec values when both are present. For example:
```bash
ploy mod run --spec mod.yaml --mod-image custom:tag --gitlab-pat "$TOKEN"
```
This uses `mod.yaml` as the base but overrides the image and PAT.

**Build Gate Healing:**

When `build_gate_healing` is configured in the spec:
1. The node runs the Build Gate before the main mod.
2. If the gate fails, each healing mod in `build_gate_healing.mods[]` runs in sequence.
3. After all healing steps, the gate is re-run. If it passes, the main mod proceeds.
4. The loop retries up to `build_gate_healing.retries` times (default: 1).
5. If the gate still fails after retries, the run terminates with `status=failed` and `reason=build-gate`.

**Repo+Diff Verification Semantics:**

Healing verification aligns with the HTTP Build Gate API's repo+diff model:

- **Initial workspace**: The Build Gate validates code cloned from `repo_url+ref`.
- **Healing modifications**: Healing mods modify the workspace in-place. Changes accumulate as diffs on top of the repo baseline.
- **Re-gate verification**: After healing, the gate re-runs against `workspace = repo_url+ref + healing changes`. This is semantically equivalent to calling:
  ```
  POST /v1/buildgate/validate
  {"repo_url": "...", "ref": "...", "diff_patch": "<healing-changes>"}
  ```
  The in-process re-gate avoids network overhead since the workspace already contains the modified state.
- **Diff chain**: Workspace state equals base clone + ordered diff sequence. This matches Mods multi-step execution where each step's changes can be replayed for rehydration.

**Sentinel Protocol (Codex healing):**

The recommended approach for Codex-based healing is the sentinel protocol. Codex edits the workspace and, when ready for validation, emits `[[REQUEST_BUILD_VALIDATION]]` as its final message. The node agent re-runs the Build Gate externally after healing completes; the sentinel keeps Codex focused on fixing code while the control plane handles validation.

**Codex Healing Handshake Checklist (TDD Validation):**

Per ROADMAP.md Phase D (RED→GREEN→REFACTOR discipline), the following artifacts should be validated after Codex-based healing runs:

| Artifact | Location | Description | Required |
|----------|----------|-------------|----------|
| Sentinel message | `codex.log` or `codex-last.txt` | `[[REQUEST_BUILD_VALIDATION]]` signals Build Gate re-run | Recommended |
| Sentinel flag | `request_build_validation` | Boolean flag file for sentinel detection | Optional |
| Session ID | `codex-session.txt` | Thread ID for resume mode across healing retries | Recommended |
| Run manifest | `codex-run.json` | JSON with `requested_build_validation`, `session_id`, `resumed` fields | Required |

**Validation steps:**

1. **Sentinel visibility**: After each healing attempt, verify that:
   - `codex.log` or `codex-last.txt` contains `[[REQUEST_BUILD_VALIDATION]]`
   - OR `request_build_validation` flag file exists with value `true`
   - The node agent logs "codex requested build validation" upon sentinel detection

2. **Session resume across healing retries**: When `retries > 1` in healing config:
   - After first healing attempt: `codex-session.txt` is written to `/out` with thread ID
   - Before second healing attempt: Session ID is propagated to `/in/codex-session.txt`
   - Subsequent healing mods receive `CODEX_RESUME=1` environment variable
   - `codex.log` shows "resume mode enabled; session=<id>" on retry attempts
   - `codex-run.json` contains `"resumed":true` for resumed runs

3. **Run manifest fields** (`codex-run.json`):
   - `requested_build_validation`: `true` when sentinel was emitted
   - `session_id`: Thread ID for conversation continuity (may be empty)
   - `resumed`: `true` if this was a resumed session, `false` otherwise

See `tests/unit/mod_codex_sh_test.sh` for unit tests covering these behaviors.
Cross-reference: `ROADMAP.md` Phase D, `GOLANG.md` Codex Healing Pipeline section.

**Cross-phase inputs available to healing mods:**
- `/in/build-gate.log` — First Build Gate failure log (read-only mount)
- `/in/prompt.txt` — Optional prompt file (mounted when provided in spec)

**Environment variables injected by the node agent for healing mods:**
- `PLOY_REPO_URL` — Git repository URL (same as the Mods run)
- `PLOY_BUILDGATE_REF` — Git ref for Build Gate baseline (base_ref or commit_sha)
- `PLOY_HOST_WORKSPACE` — Host path to workspace (for direct host verification)
- `PLOY_SERVER_URL` — ploy server URL for Build Gate HTTP API

**Generating diff patches for Build Gate verification (legacy healers only):**

> NOTE: For Codex-based healing, use the **sentinel protocol** instead (see above).
> Codex should NOT invoke Build Gate tooling directly—it edits the workspace and
> emits `[[REQUEST_BUILD_VALIDATION]]`; the node agent handles gate execution.

Legacy (non-Codex) healing mods may optionally generate unified diff patches and
use the repo+diff Build Gate API for mid-healing verification. This avoids
shipping full workspace archives over HTTP:

1. Generate a unified diff of healing changes:
   ```bash
   cd /workspace && git diff > /out/heal.patch
   ```

2. Optionally call the Build Gate HTTP API directly (for non-Codex healers) using the injected `PLOY_*` env vars if you need mid-healing verification.

Example healing spec block (sentinel protocol):
```yaml
build_gate_healing:
  retries: 1
  mods:
    - image: docker.io/you/mods-codex:latest
      env:
        CODEX_PROMPT: |-
          Rules:
          - Use /workspace and /in/build-gate.log to understand the compile error.
          - Edit files under /workspace as needed to fix the error.
          - When you believe the code is ready for a full build validation, reply with exactly:
            [[REQUEST_BUILD_VALIDATION]]
            as your final message and then stop.

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
- First Build Gate fails (Maven compile error), healing runs using `mods-codex` with the sentinel protocol—Codex emits `[[REQUEST_BUILD_VALIDATION]]` and the node agent re-runs the Build Gate, then ORW proceeds.

**Notes**

When `mods-codex` runs inside the repository directory (`/workspace`), it uses the mounted repo directly; no separate repo path is required for Codex itself. With the sentinel protocol, Codex simply emits `[[REQUEST_BUILD_VALIDATION]]` and the node agent handles the actual gate execution.

Cross-phase inputs are mounted at `/in` (read-only):
- `/in/build-gate.log` — First Build Gate failure log, available for healing mods to reference
- `/in/prompt.txt` — Default prompt location (when provided in spec; node mounts it R/O)

What to expect with the provided E2E images:
- Spec-driven healing runs with `mods-codex`; artifacts across stages are attached to the ticket and can be downloaded via `--artifact-dir`.

**Streaming Events and Reconnection**

The control plane exposes SSE streams for real-time event delivery. The CLI `--follow` flag uses resilient SSE streaming with:
- **Automatic reconnection**: On connection errors or mid-stream failures, the client reconnects with exponential backoff (250ms initial, 2x multiplier, capped at 30s).
- **Last-Event-ID support**: The client preserves the last event ID across reconnects to resume from the last processed event and avoid duplicate processing.
- **Idle timeout**: Default `45s` idle timeout cancels the stream if no events arrive. Configure via `--idle-timeout <duration>` or disable with `--idle-timeout 0`.
- **Overall timeout**: Use `--timeout <duration>` to cap total stream time (default unlimited).
- **Max retries**: Default `3` reconnect attempts. Use `--max-retries -1` for unlimited retries.

The streaming implementation uses `github.com/tmaxmax/go-sse` and the shared backoff policy from `internal/workflow/backoff`. Server `retry` hints are not consumed; reconnect delays are controlled by the backoff policy.

Tip: The CLI prints status and can also fetch artifacts via `--artifact-dir`.

**Build Gate Status Visibility**

Gate execution results are exposed via:
- `ploy mod inspect <ticket-id>` — Displays a concise gate summary line: `Gate: passed duration=1234ms` or `Gate: failed pre-gate duration=567ms`.
- `GET /v1/mods/{id}` API — Returns gate summary in `Ticket.Metadata["gate_summary"]` for programmatic access.

This makes gate health visible without requiring raw artifact inspection.

**Environment Considerations**

- Cluster targeting:
  - CLI reads the default descriptor at `~/.config/ploy/clusters/` (no env override).
- Build Gate image override:
  - To change the Java build executor container (e.g., custom Maven image), use `PLOY_BUILDGATE_JAVA_IMAGE` on worker nodes.

**How This Maps From the Legacy Nomad E2E**

- The legacy suite used two flows. With the spec, the fail→heal path is explicit under `build_gate_healing.mods` (here `mods-codex`). The same Build Gate is reused for verification via the buildgate API.

**Troubleshooting**

- Images not found / pull errors:
  - Ensure images are pushed to Docker Hub and nodes can pull them. For private repos, log in on each node: `echo "$DOCKERHUB_PAT" | docker login -u "$DOCKERHUB_USERNAME" --password-stdin`.
- Git access / MR creation:
  - Export `PLOY_GITLAB_PAT` and confirm the control plane has connectivity to GitLab. The sample repo is public for read; MRs require auth for branch writes.
- Build Gate keeps failing in Scenario B:
  - Confirm the `mods-llm` image version the cluster pulls includes the healer stub. Re-publish if needed.
- Live logs:
  - Use the CLI `--follow` flag to stream events. Check the control plane logs if stages appear stuck (cluster scheduling/resources).

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

**Remote Build Gate Mode Scenario**

E2E validation for Build Gate execution in remote-http mode where gate jobs are
routed through the HTTP API and executed by dedicated Build Gate worker nodes:
  - `PLOY_BUILDGATE_MODE=remote-http bash tests/e2e/mods/scenario-remote-buildgate/run.sh`

This scenario validates:
- HTTP-based gate routing: gates submitted via `/v1/buildgate/validate` instead of local docker.
- Multi-VPS gate execution: gate jobs can run on different nodes than mods steps.
- Job lifecycle: jobs transition through pending → claimed → running → passed/failed in `buildgate_jobs` table.
- Repo+diff semantics: re-gates after healing include `diff_patch` for remote workspace reconstruction.
- Healing flow compatibility: Codex sentinel protocol works identically in remote-http mode.

Configuration for remote-http mode:
- Workers must have `PLOY_BUILDGATE_MODE=remote-http` in their environment.
- At least one node should be designated as a Build Gate worker (claims jobs from the queue).
- `PLOY_SERVER_URL` must be set for HTTP gate client connectivity.
- The spec is identical to local-docker mode; execution mode is controlled by worker environment.

The scenario uses the failing branch (`e2e/fail-missing-symbol`) to trigger healing, demonstrating:
1. Initial gate routed to remote Build Gate worker (fails due to missing symbol).
2. Codex healing runs on the mods execution node.
3. Re-gate includes `diff_patch` with accumulated healing changes.
4. Build Gate worker clones repo+ref and applies diff_patch before validation.
5. Final ORW transformation proceeds after gate passes.

Comparison with local-docker mode:
- Results should be identical between modes (same pass/fail outcomes, same healing behavior).
- Only observable difference: gate execution location and latency.
- Run `scenario-orw-fail` without `PLOY_BUILDGATE_MODE` for baseline comparison.

Validation checklist (manual verification via DB/logs):
```sql
-- Check Build Gate job routing:
SELECT id, status, node_id, created_at, started_at, finished_at
FROM buildgate_jobs
ORDER BY created_at DESC
LIMIT 10;
```
- Jobs should show `node_id` set (claimed by Build Gate worker).
- Status transitions: pending → claimed → running → passed/failed.
- Control plane logs: "buildgate job claimed by node".
- Node agent logs: "using remote-http gate executor".

Skip artifact collection for faster runs (e.g., in CI):
```bash
SKIP_ARTIFACTS=1 PLOY_BUILDGATE_MODE=remote-http bash tests/e2e/mods/scenario-remote-buildgate/run.sh
```

**References**

- Historic E2E assets (legacy Nomad-based) found in repo history under `tests/e2e/mods/...` and service Dockerfiles for OpenRewrite. The current implementation replaces that orchestration with an internal job runner and integrated Build Gate. Relevant current references:
  - `internal/workflow/mods/plan/` — Stage graph construction and lane bindings.
  - `internal/workflow/contracts/` — Step manifest shapes and validation.
  - `internal/workflow/runner/job_templates.go` — Mods image bindings for lanes.
  - `internal/workflow/runner/healing.go` — Healing flow appended after Build Gate failures.
  - `internal/nodeagent/execution.go` — Workspace rehydration implementation (base clone + diff chain).
  - `internal/nodeagent/execution_rehydrate_test.go` — Unit tests for rehydration logic.
