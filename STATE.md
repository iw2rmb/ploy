# Ploy Docker Hub Migration — State and Next Steps

Date: 2025-10-31

## Summary

We migrated Mods image publishing and pulls from the in‑cluster OCI registry to Docker Hub. The CLI and server‑side registry surfaces were removed. Nodes now authenticate to Docker Hub via DOCKERHUB_USERNAME/DOCKERHUB_PAT. Mods images are published as:

- docker.io/<org>/mods-plan:latest
- docker.io/<org>/mods-llm:latest
- docker.io/<org>/mods-openrewrite:latest
- docker.io/<org>/mods-human:latest

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

## Current Issues / Findings

1) Mods plan stage failing due to missing GitLab signer/config

- Job inspect shows:
  - reason=executor_error
  - message: "step: workspace unavailable: step: hydrate workspace: hydration: gitlab config request returned 404 Not Found"
- Root cause: control plane lacks GitLab configuration, so repo materialization cannot obtain a token when needed.

2) Synthesised plan manifest points to Docker Hub (fixed)

- Control-plane synthesis for mods-plan now resolves image via Docker Hub precedence:
  DOCKERHUB_USERNAME -> MODS_IMAGE_PREFIX -> docker.io/iw2rmb. Path is `<prefix>/mods-plan:latest`.

3) CLI SSE streams (mods events, job logs) time out (fixed client side)

- CLI commands now clone the control‑plane HTTP client and set Timeout=0 for streaming calls (mods logs, jobs follow, mod run --follow).
  Server-side remains unchanged.

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

4) GitLab configuration

- Source ~/.zshenv locally to read PLOY_GITLAB_PAT.
- Create minimal config and apply via `dist/ploy config gitlab set --file`:
  - api_base_url: https://gitlab.com
  - allowed_projects: ["iw2rmb/*"]
  - default_token: value=PLOY_GITLAB_PAT, scopes=["api"]
  - rbac.updaters/readers: ["*"]

5) Rebuild + roll ployd; re‑run Mods smoke and verify images pull from Docker Hub and plan proceeds past hydration.

## Execution Log (this slice)

- Patched control-plane Mods synthesis to Docker Hub and updated test expectations.
- Added zero-timeout SSE clients for CLI streaming commands.
- Injected OPENAI_API_KEY for mods-llm at compose time.
- Added script to publish PLOY_OPENAI_API_KEY to nodes.
- Added `--cap` to `ploy mod run` to enforce an overall time limit and cancel the ticket on timeout.

## Next

1) `make test` then `make build` to ensure GREEN status locally. (done)
2) Source `~/.zshenv`; verify the following envs are present: DOCKERHUB_USERNAME, DOCKERHUB_PAT, PLOY_GITLAB_PAT, PLOY_OPENAI_API_KEY.
3) Apply GitLab signer config via CLI and verify with `ploy config gitlab show/status`.
4) Publish OpenAI key to lab nodes using `scripts/publish-openai-key-to-cluster.sh` and restart ployd.
5) Kick Mods smoke (plan->java->llm->human) and confirm pulls from Docker Hub + hydration succeeds.

## Notes

- Nodes already proved Docker Hub pulls for all Mods images.
- After fixes, if plan still fails, capture /v1/mods/<ticket>/logs (snapshot) and node journal logs for root cause.
