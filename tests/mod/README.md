**Mods E2E (Java 11→17) — Ploy Next**

- Goal: Recreate the historic Nomad-based Mods E2E using the current Ploy implementation (own job orchestration + integrated Build Gate) and the original sample repo ploy-orw-java11-maven. Two scenarios:
  - OpenRewrite apply upgrades Java 11→17 and Build Gate passes.
  - Same apply, but first Build Gate fails which triggers a healing loop (llm-plan + llm-exec) before Build Gate re-runs.

**Prereqs**

- Ploy cluster descriptor present (CLI auto-discovers). To override, export `PLOY_CONTROL_PLANE_URL` for this session.
- GitLab access for the sample repo's MRs: export `PLOY_GITLAB_PAT` (or set via cluster's signer if configured).
- Optional: `PLOY_OPENAI_API_KEY` if you bring a real LLM; the provided E2E images include a deterministic llm "healer" stub that does not call external APIs.

**Build + Publish Mods Images (Docker Hub)**

- Build Docker contexts under `docker/mods/...` locally (requires Docker):
  - `docker buildx build --platform linux/amd64 -t mods-openrewrite:e2e docker/mods/mod-orw`
- Repeat for `mods-llm` and `mods-plan` (contexts: `mod-llm`, `mod-plan`).
- Push to Docker Hub using the helper script:
  - `DOCKERHUB_USERNAME=<you> DOCKERHUB_PAT=*** scripts/push-mods-via-cli.sh`
  - Images publish as `docker.io/$DOCKERHUB_USERNAME/<name>:latest`.

Notes:
- Directory→repo mapping: `mod-foo` (folder) corresponds to registry repo `ploy/mods-foo`. Special-case: `mod-orw` maps to `ploy/mods-openrewrite` to match runner templates.
- The OpenRewrite image executes Maven plugin `org.openrewrite.maven:rewrite-maven-plugin` and expects a recipe JSON with keys: `group`, `artifact`, `version`, `name`. See `docs/next/manifest/examples/orw-apply.json`.
- The LLM image is a safe E2E stub: when it sees the sample’s failing branch, it creates `src/main/java/e2e/UnknownClass.java` to fix the compile.

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

**Scenario A — ORW Apply (Java 11→17) + Passing Build Gate**

- Run mods using the control plane defaults (planner → orw-apply → orw-gen → llm-plan → llm-exec → human → build-gate → static-checks → test):
  - `dist/ploy mod run \
      --repo-url https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git \
      --repo-base-ref main \
      --repo-target-ref mods-upgrade-java17 \
      --follow`

What to verify:
- Final state is Succeeded.
- Artifacts include diffs and logs for the ORW stage and a passing Build Gate report.
- Optional: Download artifacts into a directory (manifest is generated):
  - `dist/ploy mod run ... --follow --artifact-dir ./artifacts/java17-pass`

**Scenario B — ORW Apply + First Build Gate Fails → Healing (llm-plan + llm-exec) → Gate Re-run**

- Use the failing baseline branch to force the initial compile to fail. The runner will append a healing sequence (#healN) and re-run gates afterwards:
  - `dist/ploy mod run \
      --repo-url https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git \
      --repo-base-ref e2e/fail-missing-symbol \
      --repo-target-ref mods-upgrade-java17-heal \
      --follow`

What to expect with the provided E2E images:
- Initial Build Gate failure triggers healing. You should see additional stages suffixed with `#heal1` scheduled after the failure (mods-plan, orw-apply, llm-plan, llm-exec, human, then build-gate/static-checks/test again). The stub `mods-llm` creates a small class to resolve the compile error, allowing the subsequent Build Gate to pass.
- On success, artifacts across stages are attached to the ticket. Download them as needed:
  - `dist/ploy mod run ... --follow --artifact-dir ./artifacts/java17-heal`

Tip: The control plane exposes streaming events and per-stage artifacts. The CLI prints status and can also fetch artifacts via `--artifact-dir`.

**Environment Considerations**

- Cluster targeting:
  - Prefer your cached cluster descriptor under `~/.config/ploy/clusters/`. To override, set `PLOY_CONTROL_PLANE_URL`.
- LLM:
  - The stub `mods-llm` does not require `PLOY_OPENAI_API_KEY`. If you swap the image for a real implementation, export your API key and any MCP endpoints as needed.
- Build Gate image override:
  - To change the Java build executor container (e.g., custom Maven image), use `PLOY_BUILDGATE_JAVA_IMAGE` on worker nodes.

**How This Maps From the Legacy Nomad E2E**

- The legacy suite (Nomad jobs) used two flows:
  1) Apply OpenRewrite Java 11→17 and compile successfully (MR created on success).
  2) Same apply, but force an initial compile failure; runner reacts with llm-plan + llm-healing, then retries compile.
- In Ploy Next:
  - The runner plans a deterministic stage graph; Build Gate is integrated and its failures are recognized as retryable. On a retryable failure, the runner appends a healing branch (`#healN`) with Mods planner stages (including LLM) and replays gates afterwards. See `internal/workflow/runner/healing.go` for the healing logic and `internal/workflow/runner/job_templates.go` for image bindings.

**Troubleshooting**

- Images not found / pull errors:
  - Ensure images are pushed to Docker Hub and nodes can pull them. For private repos, log in on each node: `echo "$DOCKERHUB_PAT" | docker login -u "$DOCKERHUB_USERNAME" --password-stdin`.
- Git access / MR creation:
  - Export `PLOY_GITLAB_PAT` and confirm the control plane has connectivity to GitLab. The sample repo is public for read; MRs require auth for branch writes.
- Build Gate keeps failing in Scenario B:
  - Confirm the `mods-llm` image version the cluster pulls includes the healer stub. Re-publish if needed.
- Live logs:
  - Use the CLI `--follow` flag to stream events. Check the control plane logs if stages appear stuck (cluster scheduling/resources).

**References**

- Historic E2E assets (legacy Nomad-based) found in repo history under `tests/e2e/mods/...` and service Dockerfiles for OpenRewrite. The current implementation replaces that orchestration with an internal job runner and integrated Build Gate. Relevant current references:
  - `docs/next/manifest/examples/orw-apply.json` — OpenRewrite step manifest example (Java 11→17 recipe).
  - `docs/next/manifest/examples/llm-plan.json` — LLM step manifest example.
  - `internal/workflow/runner/job_templates.go` — Mods image bindings for lanes.
  - `internal/workflow/runner/healing.go` — Healing flow appended after Build Gate failures.
