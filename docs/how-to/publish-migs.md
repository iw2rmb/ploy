Publish Migs Images to a Local Registry

Overview
- Migs images live under `deploy/images/orw/`, `deploy/images/codex/`, `deploy/images/amata/`, and `deploy/images/shell/`:
  - `orw-cli-maven` (`deploy/images/orw/orw-cli-maven`) -> `orw-cli-maven`
  - `orw-cli-gradle` (`deploy/images/orw/orw-cli-gradle`) -> `orw-cli-gradle`
  - `codex` (`deploy/images/codex`) -> `migs-codex`
  - `amata` (`deploy/images/amata`) -> `migs-amata`
- Local default registry prefix is `127.0.0.1:5000/ploy`.
- The runner resolves images as `$PLOY_CONTAINER_REGISTRY/<name>:latest`.

Local Registry Prerequisites
- Deploy the local stack:
  - `deploy/local/run.sh`
- Export the registry prefix for specs/scripts:
  - `export PLOY_CONTAINER_REGISTRY=127.0.0.1:5000/ploy`

Publish all Migs images
```bash
deploy/images/build-and-push.sh
# Builds and pushes: migs-amata, migs-codex, migs-shell, orw-cli-maven, orw-cli-gradle.
# Also builds/pushes runtime images: ploy-server and ploy-node.
```

There is no separate registry sync helper script. Publish explicitly via `build-and-push.sh`
or targeted `docker buildx build ... --push` commands.

Custom CA support is runtime-only. Do not inject corporate certs during image build.
Use `PLOY_CA_CERTS` at deployment/runtime so the same bundle is mounted into runtime
containers and propagated as `CA_CERTS_PEM_BUNDLE`.

Publish a single Migs image (example: orw-cli-maven)
```bash
IMAGE_PREFIX="${PLOY_CONTAINER_REGISTRY:-127.0.0.1:5000/ploy}" \
  docker buildx build --platform linux/amd64 \
  -f deploy/images/orw/orw-cli-maven/Dockerfile \
  -t "${IMAGE_PREFIX}/orw-cli-maven:latest" \
  --push .
```

Publish `migs-codex` (manual one-off)

```bash
IMAGE_PREFIX="${PLOY_CONTAINER_REGISTRY:-127.0.0.1:5000/ploy}"
docker buildx build \
  --platform linux/amd64 \
  -f deploy/images/codex/Dockerfile \
  -t "${IMAGE_PREFIX}/migs-codex:latest" \
  --push .
```

Publish `migs-amata` (manual one-off)

```bash
# Step 1: build and stage the amata binary (requires ../amata source sibling repo)
PLATFORM=linux/amd64 deploy/images/amata/build-amata.sh

# Step 2: build and push the migs-amata image
IMAGE_PREFIX="${PLOY_CONTAINER_REGISTRY:-127.0.0.1:5000/ploy}"
docker buildx build \
  --platform linux/amd64 \
  -f deploy/images/amata/Dockerfile \
  -t "${IMAGE_PREFIX}/migs-amata:latest" \
  --push .
```

Stack-aware image mapping example
```yaml
image:
  default: ${PLOY_CONTAINER_REGISTRY}/orw-cli-maven:latest
  java-maven: ${PLOY_CONTAINER_REGISTRY}/orw-cli-maven:latest
  java-gradle: ${PLOY_CONTAINER_REGISTRY}/orw-cli-gradle:latest
```

Notes
- Directory mapping:
  - `orw-cli-maven` -> `orw-cli-maven`
  - `orw-cli-gradle` -> `orw-cli-gradle`
- To use a different registry/namespace, override:
  - `IMAGE_PREFIX=... deploy/images/build-and-push.sh`

Multi-arch push
```bash
PLATFORM=linux/amd64 deploy/images/build-and-push.sh
```

Verification
```bash
docker buildx imagetools inspect "${PLOY_CONTAINER_REGISTRY}/orw-cli-maven:latest"
docker buildx imagetools inspect "${PLOY_CONTAINER_REGISTRY}/orw-cli-gradle:latest"
docker buildx imagetools inspect "${PLOY_CONTAINER_REGISTRY}/migs-codex:latest"
```
