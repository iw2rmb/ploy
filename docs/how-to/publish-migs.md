Publish Migs Images to a Local Registry

Overview
- Migs images live under `images/orw/`, `images/codex/`, `images/amata/`, and `images/shell/`:
  - `orw-cli-maven` (`images/orw/orw-cli-maven`) -> `orw-cli-maven`
  - `orw-cli-gradle` (`images/orw/orw-cli-gradle`) -> `orw-cli-gradle`
  - `codex` (`images/codex`) -> `codex`
  - `amata` (`images/amata`) -> `amata`
ghcr.io/iw2rmb- The runner resolves images as `$PLOY_CONTAINER_REGISTRY/<name>:latest`.

Local Registry Prerequisites
- Deploy the local stack:
  - `ploy cluster deploy`
- Export the registry prefix for specs/scripts:
  - `export PLOY_CONTAINER_REGISTRY=ghcr.io/iw2rmb/ploy`

Publish all Migs images
```bash
images/build-and-push.sh
# Builds and pushes: amata, codex, shell, orw-cli-maven, orw-cli-gradle,
# gate-gradle:jdk11, gate-gradle:jdk17.
# Also mirrors Maven gate bases into your registry namespace:
# maven:3-eclipse-temurin-11, maven:3-eclipse-temurin-17.
# Also builds/pushes runtime images: server and node.
```

There is no separate registry sync helper script. Publish explicitly via `build-and-push.sh`
or targeted `docker buildx build ... --push` commands.

Build Gate image mapping source of truth:
- `gates/stacks.yaml`
- Java defaults expect:
  - `$PLOY_CONTAINER_REGISTRY/gate-gradle:jdk11`
  - `$PLOY_CONTAINER_REGISTRY/gate-gradle:jdk17`
  - `$PLOY_CONTAINER_REGISTRY/maven:3-eclipse-temurin-11`
  - `$PLOY_CONTAINER_REGISTRY/maven:3-eclipse-temurin-17`

Custom CA support is runtime-only. Do not inject corporate certs during image build.
Register CA bundles via `ploy config ca set --file /path/to/ca-bundle.pem` so the
cluster can mount them into runtime containers via Hydra `ca` records (mounted
read-only under `/etc/ploy/ca/`).

Publish a single Migs image (example: orw-cli-maven)
```bash
IMAGE_PREFIX="${PLOY_CONTAINER_REGISTRY:-ghcr.io/iw2rmb/ploy}" \
  docker buildx build --platform linux/amd64 \
  -f images/orw/orw-cli-maven/Dockerfile \
  -t "${IMAGE_PREFIX}/orw-cli-maven:latest" \
  --push .
```

Publish `codex` (manual one-off)

```bash
IMAGE_PREFIX="${PLOY_CONTAINER_REGISTRY:-ghcr.io/iw2rmb/ploy}"
docker buildx build \
  --platform linux/amd64 \
  -f images/codex/Dockerfile \
  -t "${IMAGE_PREFIX}/codex:latest" \
  --push .
```

Publish `amata` (manual one-off)

```bash
# Step 1: build and stage the amata binary (requires ../amata source sibling repo)
PLATFORM=linux/amd64 images/amata/build-amata.sh

# Step 2: build and push the amata image
IMAGE_PREFIX="${PLOY_CONTAINER_REGISTRY:-ghcr.io/iw2rmb/ploy}"
docker buildx build \
  --platform linux/amd64 \
  -f images/amata/Dockerfile \
  -t "${IMAGE_PREFIX}/amata:latest" \
  --push .
```

Stack-aware image mapping example
```yaml
image:
  default: $PLOY_CONTAINER_REGISTRY/orw-cli-maven:latest
  java-maven: $PLOY_CONTAINER_REGISTRY/orw-cli-maven:latest
  java-gradle: $PLOY_CONTAINER_REGISTRY/orw-cli-gradle:latest
```

Notes
- Directory mapping:
  - `orw-cli-maven` -> `orw-cli-maven`
  - `orw-cli-gradle` -> `orw-cli-gradle`
- To use a different registry/namespace, override:
  - `IMAGE_PREFIX=... images/build-and-push.sh`

Multi-arch push
```bash
PLATFORM=linux/amd64 images/build-and-push.sh
```

Verification
```bash
docker buildx imagetools inspect "$PLOY_CONTAINER_REGISTRY/orw-cli-maven:latest"
docker buildx imagetools inspect "$PLOY_CONTAINER_REGISTRY/orw-cli-gradle:latest"
docker buildx imagetools inspect "$PLOY_CONTAINER_REGISTRY/codex:latest"
```
