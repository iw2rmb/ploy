**Mods E2E (Java 11→17) — Ploy Next**

- Goal: Recreate the historic Nomad-based Mods E2E using the current Ploy implementation (own job orchestration + integrated Build Gate) and the original sample repo ploy-orw-java11-maven. Two scenarios:
  - OpenRewrite apply upgrades Java 11→17 and Build Gate passes.
  - Same apply, but first Build Gate fails which triggers a healing loop (llm-plan + llm-exec) before Build Gate re-runs.

**Prereqs**

- Ploy cluster descriptor present (CLI auto-discovers from `~/.config/ploy/clusters/default`).
- GitLab access for the sample repo's MRs: export `PLOY_GITLAB_PAT` (or set via cluster's signer if configured).
- Optional: `PLOY_OPENAI_API_KEY` if you bring a real LLM; the provided E2E images include a deterministic llm "healer" stub that does not call external APIs.

**Build + Publish Mods Images (Docker Hub)**

- Build Docker contexts under `mods/...` locally (requires Docker):
  - `docker buildx build --platform linux/amd64 -t mods-openrewrite:e2e mods/mod-orw`
- Repeat for `mods-llm` and `mods-plan` (contexts: `mod-llm`, `mod-plan`).
- Push to Docker Hub using the helper script:
  - `DOCKERHUB_USERNAME=<you> DOCKERHUB_PAT=*** scripts/push-mods-via-cli.sh`
  - Images publish as `docker.io/$DOCKERHUB_USERNAME/<name>:latest`.

Notes:
- Directory→repo mapping: `mod-foo` (folder) corresponds to registry repo `ploy/mods-foo`. Special-case: `mod-orw` maps to `ploy/mods-openrewrite` to match examples.
- Coordinates are passed via environment only (no JSON manifest support in mod-orw): set `RECIPE_GROUP`, `RECIPE_ARTIFACT`, `RECIPE_VERSION`, `RECIPE_CLASSNAME` (and optional `MAVEN_PLUGIN_VERSION`).
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

**Spec‑Driven Flow (recommended)**

Use the YAML spec to define mod parameters, Build Gate, and healing.
Example spec:
  - `tests/e2e/mods/scenario-orw-fail/mod.yaml`

Run the failing→healing scenario with a single script:
  - `bash tests/e2e/mods/scenario-orw-fail/run.sh`
  - It submits:
    - `--repo-url https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git`
    - `--repo-base-ref e2e/fail-missing-symbol`
    - `--repo-target-ref mods-upgrade-java17-heal`
    - `--spec tests/e2e/mods/scenario-orw-fail/mod.yaml`
    - `--follow --artifact-dir ./tmp/mods/scenario-orw-fail/<ts>`

What to verify:
- First Build Gate fails (Maven compile error), healing runs using `mods-codex` with an embedded verification rule to call the exact Build Gate via `ploy-buildgate`, re‑gate passes, ORW proceeds.

**Notes**

When `mods-codex` runs inside the repository directory (`/workspace`), it uses the mounted repo directly; no separate repo path is required for Codex itself. The Build Gate verification inside Codex uses `ploy-buildgate` and requires Docker socket access and `PLOY_HOST_WORKSPACE` to point to the host path.

What to expect with the provided E2E images:
- Spec-driven healing runs with `mods-codex`; artifacts across stages are attached to the ticket and can be downloaded via `--artifact-dir`.

Tip: The control plane exposes streaming events and per-stage artifacts. The CLI prints status and can also fetch artifacts via `--artifact-dir`.

**Environment Considerations**

- Cluster targeting:
  - CLI reads the default descriptor at `~/.config/ploy/clusters/` (no env override).
- Build Gate image override:
  - To change the Java build executor container (e.g., custom Maven image), use `PLOY_BUILDGATE_JAVA_IMAGE` on worker nodes.

**How This Maps From the Legacy Nomad E2E**

- The legacy suite used two flows. With the spec, the fail→heal path is explicit under `build_gate_healing.mods` (here `mods-codex`). The same Build Gate is reused for verification via `ploy-buildgate`.

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
  - `internal/workflow/mods/plan/` — Stage graph construction and lane bindings.
  - `internal/workflow/contracts/` — Step manifest shapes and validation.
  - `internal/workflow/runner/job_templates.go` — Mods image bindings for lanes.
  - `internal/workflow/runner/healing.go` — Healing flow appended after Build Gate failures.
