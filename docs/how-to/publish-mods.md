Publish Mods Images to Docker Hub

Overview
- Mods images live under `docker/mods/`:
  - `mod-orw` — OpenRewrite apply (Maven) → `mods-openrewrite`
    - Coordinates are passed via environment only (no JSON spec for coords):
      set `RECIPE_GROUP`, `RECIPE_ARTIFACT`, `RECIPE_VERSION`, `RECIPE_CLASSNAME` (optional `MAVEN_PLUGIN_VERSION`).
  - `mod-codex` — Codex CLI wrapper (sentinel protocol) → `mods-codex`
    - Build requires no special context; uses standard Node base image.
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
scripts/docker/build-and-push-mods.sh
# Discovers mods subfolders, builds for linux/amd64, and pushes :latest to Docker Hub.
# Special-cases mod-codex to use repo-root context automatically.
```

Publish a single Mods image
```bash
name=mod-orw
IMAGE_PREFIX="docker.io/${DOCKERHUB_USERNAME}" \
  docker buildx build --platform linux/amd64 -t "${IMAGE_PREFIX}/mods-openrewrite:latest" --push docker/mods/${name}
```

Publish mods-codex (manual one-off)
```bash
IMAGE_PREFIX="docker.io/${DOCKERHUB_USERNAME}"
docker buildx build \
  --platform linux/amd64 \
  -f docker/mods/mod-codex/Dockerfile \
  -t "${IMAGE_PREFIX}/mods-codex:latest" \
  --push .
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

Multi‑arch (Mac + Linux) push
- Requirements: Docker engine running, Buildx available. On first run, create a builder and bootstrap emulation:
  ```bash
  docker buildx create --name ploy-local --driver docker-container --use
  docker buildx inspect --bootstrap
  ```
- Build and push both amd64 and arm64 for all Mods via the script by overriding `PLATFORM`:
  ```bash
  PLATFORM=linux/amd64,linux/arm64 scripts/docker/build-and-push-mods.sh
  ```
- Or build a single image:
  ```bash
  IMAGE_PREFIX="docker.io/${DOCKERHUB_USERNAME}"
  docker buildx build \
    --platform linux/amd64,linux/arm64 \
    -t "${IMAGE_PREFIX}/mods-plan:latest" \
    --push docker/mods/mod-plan
  ```
- Verify manifests list both platforms:
  ```bash
  docker buildx imagetools inspect docker.io/$DOCKERHUB_USERNAME/mods-plan:latest
  ```

Verification for required images
```bash
docker buildx imagetools inspect docker.io/$DOCKERHUB_USERNAME/mods-openrewrite:latest
docker buildx imagetools inspect docker.io/$DOCKERHUB_USERNAME/mods-codex:latest
```
