Mods Image Contexts

- This directory contains Docker build contexts for the Mods lanes referenced by the runner templates (`internal/workflow/runner/job_templates.go`).
  - `mod-orw` — OpenRewrite apply (Maven) wrapper; entrypoint script `mods-orw`.
  - `mod-llm` — Deterministic E2E planner/executor stub; fixes the known compile error in the sample repo when present.
  - `mod-plan` — Lightweight planner stub to exercise planner integration during E2E.
  - (Human gate image removed for now.)

Build and publish (Docker Hub)
- Use: `scripts/push-mods-via-cli.sh` to iterate all subdirectories and `docker buildx build --push` images as `docker.io/$DOCKERHUB_USERNAME/<name>:latest`.
- Set `DOCKERHUB_USERNAME` (and `DOCKERHUB_PAT` if pushing to private repos) in your shell.

Notes
- Images are intentionally minimal: they’re designed for E2E and cluster smoke. Swap with production-capable images as needed by editing runner job templates or re-pushing different tags.
