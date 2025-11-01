# Ploy Docker Hub Migration — State and Next Steps

Date: 2025-10-31

## Summary

We migrated Mods image publishing and pulls from the in‑cluster OCI registry to Docker Hub. The CLI and server‑side registry surfaces were removed. Nodes now authenticate to Docker Hub via DOCKERHUB_USERNAME/DOCKERHUB_PAT. Mods images are published as:

- docker.io/<org>/mods-plan:latest
- docker.io/<org>/mods-llm:latest
- docker.io/<org>/mods-openrewrite:latest
  (Human gate image removed for now.)

Org defaults to DOCKERHUB_USERNAME if set; otherwise MODS_IMAGE_PREFIX can override fully; fallback is docker.io/iw2rmb.

## What’s Done

- Removed the CLI “registry” commands and server‑side /v1/registry + /v2 aliases.
- Updated docs and OpenAPI to remove registry endpoints; kept /v2/artifacts aliases.
- Adjusted runner job templates to resolve images from Docker Hub by default.
- Added scripts:
  - scripts/push-mods-via-cli.sh — buildx --push to Docker Hub
  - scripts/publish-dockerhub-creds-to-cluster.sh — publish env + docker login to nodes
  - scripts/update_ployd_cluster.sh — rebuild/deploy ployd
- Published Mods images to docker.io/iwtormb and pre-pulled them on all lab nodes.
- Fixed worker env:
  - Ensured PLOY_IPFS_CLUSTER_API via systemd drop‑in on all three nodes; ployd active.
- Auto-MR publishing:
  - Control plane now creates a GitLab branch and merge request using the ORW diff bundle once all Mods stages succeed. Uses `PLOY_GITLAB_PAT` via the signer config, commits files to the `repo_target_ref` branch (creating it off `repo_base_ref` if needed), and opens an MR.

## Current Issues / Findings

1) Mods plan stage failing due to missing GitLab signer/config (fixed)

- Initially job inspect showed:
  - reason=executor_error
  - message: "step: workspace unavailable: step: hydrate workspace: hydration: gitlab config request returned 404/503"
  Root cause: control plane lacked GitLab signer (missing AES key) and default secret. Fixed by publishing `PLOY_GITLAB_SIGNER_AES_KEY` to nodes and rotating `default` secret with `PLOY_GITLAB_PAT`.

2) Synthesised plan manifest points to Docker Hub (fixed)

- Control-plane synthesis for mods-plan now resolves image via Docker Hub precedence:
  DOCKERHUB_USERNAME -> MODS_IMAGE_PREFIX -> docker.io/iw2rmb. Path is `<prefix>/mods-plan:latest`.

3) CLI SSE streams (mods events, job logs) time out or hang (fixed client side)

- Streaming clients now use Timeout=0 to avoid premature header timeouts, and include an idle guard: `--idle-timeout` (default 45s). When no events arrive for the idle window, the commands exit with a clear error instead of hanging.

4) LLM/OpenAI key propagation (partially fixed)

- Runner now injects OPENAI_API_KEY into mods-llm lane when PLOY_OPENAI_API_KEY is set on the control plane.
- New helper script `scripts/publish-openai-key-to-cluster.sh` publishes PLOY_OPENAI_API_KEY to nodes via systemd drop‑in and restarts ployd.
  Execution on lab nodes still pending below.

## Plan / Required Next Steps

1) Control-plane code (done)

- internal/controlplane/mods/service.go now uses Docker Hub prefix for synthesized plan manifest.
- internal/controlplane/mods/service_manifest_test.go updated to expect docker.io/iw2rmb/mods-plan:latest.

2) CLI SSE reliability (done)

- `cmd/ploy/common_http.go` adds `cloneForStream()`; mods logs, jobs follow, and mod run now use a zero-timeout stream client.

3) LLM key injection (half done)

- `internal/workflow/runner/job_templates.go` injects OPENAI_API_KEY into mods-llm when PLOY_OPENAI_API_KEY is present at compose time.
- Pending: publish key to nodes and verify LLM stage sees it.

4) GitLab configuration (done)

- Applied via CLI with PLOY_GITLAB_PAT from local env. `ploy config gitlab show` reports the expected values.

5) Rebuild + roll ployd; re‑run Mods smoke and verify images pull from Docker Hub and hydration proceeds.
   - Status: runs are flowing. We removed plan-stage artifact uploads and corrected IPFS /add params; snapshot publish guards are in place. Full Mods runs now complete on the lab.

## Execution Log (this slice)

- Patched control-plane Mods synthesis to Docker Hub and updated test expectations.
- Added zero-timeout SSE clients for CLI streaming commands.
- Added idle guard in stream client and flags `--idle-timeout` for `mods logs` and `jobs follow` (default 45s).
- Added overall stream timeouts `--timeout` for `mods logs` and `jobs follow`. Together with `mod run --cap`, streaming commands now cannot hang indefinitely.
- Injected OPENAI_API_KEY for mods-llm at compose time.
- Added script to publish PLOY_OPENAI_API_KEY to nodes.
- Added `--cap` to `ploy mod run` to enforce an overall time limit and cancel the ticket on timeout.
 - Added symlink aliases in Mods images (mods-*/mod-* parity) and a self-test fast path for mods-openrewrite.
 - Wrote Docker-based integration tests to validate Mods images locally and measure execution time: `tests/integration/mods/mods_images_test.go` (passes; total ~4s on this host).

## Next

1) `make test` then `make build` to ensure GREEN status locally. (done)
2) Source `~/.zshenv`; verify envs: DOCKERHUB_USERNAME, DOCKERHUB_PAT, PLOY_GITLAB_PAT, PLOY_OPENAI_API_KEY. (done)
3) Apply GitLab signer config via CLI and verify with `ploy config gitlab show/status`. (done)
4) Publish OpenAI key to lab nodes via `scripts/publish-openai-key-to-cluster.sh` and restart ployd. (done)
5) Publish signer AES key to lab and rotate default secret. (done)
6) Update ployd on nodes. (done)
7) Kick Mods smoke (plan->java->llm) and confirm pulls + hydration. (done)
8) Ensure MR creation by producing a non-empty diff in orw-apply (choose a baseline branch needing migration or a recipe guaranteed to rewrite sources), then re-run and verify MR opened.

## Notes

- Nodes already proved Docker Hub pulls for all Mods images.
- IPFS add/pin: fixed client params and added guardrails; manual /add up to 100MB succeeds.

## Ticket mods-014410 — MR Status and Local Repro

- Ticket: mods-014410
- Repo: https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git (base=main, target=mod/e2e-20251101014410)
- Outcome: succeeded; MR not created because `orw-apply` produced no diff bundle.
- Local reproduction (matching cluster args) confirmed this:
  - Ran `mods-openrewrite:latest` with recipe `org.openrewrite.java.migrate.UpgradeToJava17` against branch `main`.
  - Container completed successfully; `git status` showed only `target/` outputs and no source changes.
  - Conclusion: absence of MR is consistent with an empty diff; to create an MR we need a baseline that yields changes or a different recipe.

## Immediate Next Steps (IPFS Cluster)
- Validate Cluster health from node A (`curl http://127.0.0.1:9094/health` and `/id`, `/peers`). Ensure peers are connected.
- If only one peer is healthy, keep replication=1 (already set) and retry. If add still fails, set `FetchBaseURL` to a working gateway or temporarily bypass publish by switching hydration to local (feature flag TBD).
- Capture `journalctl -u ployd` around the failure for the exact add payload sizes and retry policy.

## Integration Test Reports
- Generated under `tests/integration/mods/report/` from local Docker runs:
  - plan.json
  - orw-report.json
  - llm-UnknownClass.java
  Execution times: plan ~0.35s, ORW ~1m18s (cached), LLM ~0.28s.
