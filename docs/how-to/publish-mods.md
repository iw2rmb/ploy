Publish Mods Images to Docker Hub

Overview
- Mods images live under `docker/mods/`:
  - `mod-orw` — OpenRewrite apply (Maven) → `mods-openrewrite`
  - `mod-llm` — LLM plan/execute stub → `mods-llm`
  - `mod-plan` — Planner stub → `mods-plan`
  - (Human gate image removed for now.)
- The runner pulls images from Docker Hub by default: `docker.io/$DOCKERHUB_USERNAME/<name>:latest`.

Prerequisites
- Set the following environment variables in your shell (e.g., in `~/.zshenv`):
  - `DOCKERHUB_USERNAME` — your Docker Hub username or org
  - `DOCKERHUB_PAT` — personal access token with write access (used for login)
- Optional: set `MODS_IMAGE_PREFIX` to an absolute prefix (e.g., `docker.io/org`). If set, it overrides `DOCKERHUB_USERNAME`.

Publish all Mods images
```bash
scripts/push-mods-via-cli.sh
# Discovers docker/mods subfolders, builds for linux/amd64, and pushes :latest to Docker Hub.
```

Publish a single Mods image
```bash
name=mod-orw
IMAGE_PREFIX="docker.io/${DOCKERHUB_USERNAME}" \
  docker buildx build --platform linux/amd64 -t "${IMAGE_PREFIX}/mods-openrewrite:latest" --push docker/mods/${name}
```

Configure node pulls (private repos)
- During deploy: set `DOCKERHUB_USERNAME` and `DOCKERHUB_PAT` on each node before running the bootstrap script. The installer logs into Docker Hub automatically.
- Existing clusters (manual): SSH to each node and run:
```bash
echo "$DOCKERHUB_PAT" | docker login -u "$DOCKERHUB_USERNAME" --password-stdin
```

Notes
- Directory name to repo mapping: `mod-foo` → `mods-foo`; special‑case `mod-orw` → `mods-openrewrite`.
- To use a different registry/namespace, set `MODS_IMAGE_PREFIX` (for example, `docker.io/acme`).
