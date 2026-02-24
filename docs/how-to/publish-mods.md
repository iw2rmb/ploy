Publish Mods Images to Docker Hub

Overview
- Mods images live under `deploy/images/migs/`:
  - `orw-maven` — OpenRewrite apply for Maven-only workspaces → `migs-orw-maven`
    - Requires `pom.xml` in the workspace; runs the Rewrite Maven plugin.
    - Coordinates are passed via environment: `RECIPE_GROUP`, `RECIPE_ARTIFACT`, `RECIPE_VERSION`, `RECIPE_CLASSNAME` (optional `MAVEN_PLUGIN_VERSION`).
  - `orw-gradle` — OpenRewrite apply for Gradle-only workspaces (Kotlin DSL) → `migs-orw-gradle`
    - Requires `build.gradle.kts` in the workspace; prefers `./gradlew`, falls back to `gradle` in `PATH`.
    - Same `RECIPE_*` environment variables as Maven; plugin injected via Kotlin DSL.
  - `mig-codex` — Codex CLI wrapper (workspace diff handshake) → `migs-codex`
    - Codex edits the workspace and exits; the node agent inspects the workspace via `git status --porcelain` and only re-runs the Build Gate when changes are present.
    - Build requires no special context; uses a standard Node base image; Codex never runs the Build Gate or build tools directly.
  - `mig-llm` — LLM plan/execute stub → `migs-llm`
  - `mig-plan` — Planner stub → `migs-plan`
  - (Human gate image removed for now.)
- The runner pulls images from Docker Hub by default: `$PLOY_CONTAINER_REGISTRY/<name>:latest`.

Stack-aware images
- Use the stack-aware `image` map in `mod.yaml` to select `orw-maven` or `orw-gradle` based on the Build Gate detected stack:
  ```yaml
  image:
    default: $PLOY_CONTAINER_REGISTRY/migs-orw-maven:latest
    java-maven: $PLOY_CONTAINER_REGISTRY/migs-orw-maven:latest
    java-gradle: $PLOY_CONTAINER_REGISTRY/migs-orw-gradle:latest
  ```
- The Build Gate detects `java-maven` when `pom.xml` is present, `java-gradle` when only Gradle files exist.

Prerequisites
- Set the following environment variables in your shell (e.g., in `~/.zshenv`):
  - `DOCKERHUB_USERNAME` — your Docker Hub username or org
  - `DOCKERHUB_PAT` — personal access token with write access (used for login)
- Optional: set `MODS_IMAGE_PREFIX` to an absolute prefix (e.g., `docker.io/org`). If set, it overrides `DOCKERHUB_USERNAME`.

Publish all Mods images
```bash
deploy/images/build-and-push-migs.sh
# Discovers mods subfolders, builds for linux/amd64, and pushes :latest to Docker Hub.
# Special-cases mig-codex to use repo-root context automatically.
```

Publish a single Mods image (example: orw-maven)
```bash
name=orw-maven
IMAGE_PREFIX="$PLOY_CONTAINER_REGISTRY" \
  docker buildx build --platform linux/amd64 -t "${IMAGE_PREFIX}/migs-orw-maven:latest" --push deploy/images/migs/${name}
```

Publish migs-codex (manual one-off)
```bash
IMAGE_PREFIX="$PLOY_CONTAINER_REGISTRY"
docker buildx build \
  --platform linux/amd64 \
  -f deploy/images/migs/mig-codex/Dockerfile \
  -t "${IMAGE_PREFIX}/migs-codex:latest" \
  --push .
```

Configure node pulls (private repos)
- Local Docker cluster: log in on the host Docker engine (the node uses the host Docker daemon via `/var/run/docker.sock`):
```bash
echo "$DOCKERHUB_PAT" | docker login -u "$DOCKERHUB_USERNAME" --password-stdin
```

Notes
- Directory name to repo mapping: `mig-foo` → `migs-foo`; `orw-maven` → `migs-orw-maven`; `orw-gradle` → `migs-orw-gradle`.
- To use a different registry/namespace, set `MODS_IMAGE_PREFIX` (for example, `docker.io/acme`).

Multi‑arch (Mac + Linux) push
- Requirements: Docker engine running, Buildx available. On first run, create a builder and bootstrap emulation:
  ```bash
  docker buildx create --name ploy-local --driver docker-container --use
  docker buildx inspect --bootstrap
  ```
- Build and push both amd64 and arm64 for all Mods via the script by overriding `PLATFORM`:
  ```bash
  PLATFORM=linux/amd64,linux/arm64 deploy/images/build-and-push-migs.sh
  ```
- Or build a single image:
  ```bash
  IMAGE_PREFIX="$PLOY_CONTAINER_REGISTRY"
  docker buildx build \
    --platform linux/amd64,linux/arm64 \
    -t "${IMAGE_PREFIX}/migs-plan:latest" \
    --push deploy/images/migs/mig-plan
  ```
- Verify manifests list both platforms:
  ```bash
  docker buildx imagetools inspect $PLOY_CONTAINER_REGISTRY/migs-plan:latest
  ```

Verification for required images
```bash
docker buildx imagetools inspect $PLOY_CONTAINER_REGISTRY/migs-orw-maven:latest
docker buildx imagetools inspect $PLOY_CONTAINER_REGISTRY/migs-orw-gradle:latest
docker buildx imagetools inspect $PLOY_CONTAINER_REGISTRY/migs-codex:latest
```
