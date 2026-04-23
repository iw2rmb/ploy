Publish Migs Images to a Local Registry

Overview
- Migs images live under `images/orw/`, `images/amata/`, and `images/sbom/`:
  - `orw-cli-maven` (`images/orw/orw-cli-maven`) -> `orw-cli-maven`
  - `orw-cli-gradle` (`images/orw/orw-cli-gradle`) -> `orw-cli-gradle`
  - `java-17-codex-amata-maven` (`images/amata/java-17-codex-amata-maven`) -> `java-17-codex-amata-maven`
  - `java-17-codex-amata-gradle` (`images/amata/java-17-codex-amata-gradle`) -> `java-17-codex-amata-gradle`
  - `sbom-maven` (`images/sbom/maven`) -> `ploy/sbom-maven:jdk11|jdk17`
  - `sbom-gradle` (`images/sbom/gradle`) -> `ploy/sbom-gradle:jdk11|jdk17`
- The runner resolves most mig images as `$PLOY_CONTAINER_REGISTRY/<name>:latest`.
- SBOM runtime lanes are resolved as fixed tags: `sbom-maven:jdk11|jdk17` and `sbom-gradle:jdk11|jdk17`.

Local Registry Prerequisites
- Deploy the local stack:
  - `ploy cluster deploy`
- Export the registry prefix for specs/scripts:
  - `export PLOY_CONTAINER_REGISTRY=ghcr.io/iw2rmb/ploy`

Publish all Migs images
```bash
# Optional: custom PEM bundle (path or inline PEM content) for build-time TLS interception environments
# export PLOY_CA_CERTS=/path/to/ca-bundle.pem

images/build-and-push.sh
# Builds and pushes: java-17-codex-amata-maven, java-17-codex-amata-gradle,
# orw-cli-maven, orw-cli-gradle,
# sbom-maven:jdk11,jdk17 and sbom-gradle:jdk11,jdk17,
# gate-gradle:jdk11, gate-gradle:jdk17.
# Also builds/pushes Maven gate wrappers into your registry namespace:
# maven:3-eclipse-temurin-11, maven:3-eclipse-temurin-17.
# Also builds/pushes shared Java base lanes:
# java-base-maven:jdk11,jdk17, java-base-gradle:jdk11,jdk17, java-base-temurin:jdk17.
# Also builds/pushes runtime images: server and node.
```

There is no separate registry sync helper script. Publish explicitly via `build-and-push.sh`
or targeted `docker buildx build ... --push` commands.

Build-time custom CA bundle (`PLOY_CA_CERTS`):
- Accepts either:
  - filesystem path to PEM bundle, or
  - inline PEM content in the env variable itself.
- Build scripts forward it as BuildKit secret `ploy_ca_certs`.
- Dockerfiles mount that secret and place it at `/etc/ploy/certs/ca.crt` before network steps.

Build Gate image mapping source of truth:
- `gates/stacks.yaml`
- Java defaults expect:
  - `$PLOY_CONTAINER_REGISTRY/gate-gradle:jdk11`
  - `$PLOY_CONTAINER_REGISTRY/gate-gradle:jdk17`
  - `$PLOY_CONTAINER_REGISTRY/maven:3-eclipse-temurin-11`
  - `$PLOY_CONTAINER_REGISTRY/maven:3-eclipse-temurin-17`

Runtime CA support is separate from build-time CA injection.
Register runtime CA bundles via `ploy config ca set --file /path/to/ca-bundle.pem` so the
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

Publish a Codex+Amata lane manually (example: Maven lane)

```bash
# Step 1: build and stage the amata binary (requires ../amata source sibling repo)
PLATFORM=linux/amd64 images/amata/build-amata.sh

# Step 2: build and push java-17-codex-amata-maven
IMAGE_PREFIX="${PLOY_CONTAINER_REGISTRY:-ghcr.io/iw2rmb/ploy}"
docker buildx build \
  --platform linux/amd64 \
  -f images/amata/java-17-codex-amata-maven/Dockerfile \
  -t "${IMAGE_PREFIX}/java-17-codex-amata-maven:latest" \
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
  - `java-17-codex-amata-maven` -> `java-17-codex-amata-maven`
  - `java-17-codex-amata-gradle` -> `java-17-codex-amata-gradle`
  - `orw-cli-maven` -> `orw-cli-maven`
  - `orw-cli-gradle` -> `orw-cli-gradle`
  - `sbom-maven` -> `ploy/sbom-maven:jdk11|jdk17`
  - `sbom-gradle` -> `ploy/sbom-gradle:jdk11|jdk17`
- To use a different registry/namespace, override:
  - `IMAGE_PREFIX=... images/build-and-push.sh`

Multi-arch push
```bash
PLATFORM=linux/amd64,linux/arm64 images/build-and-push.sh
```

Verification
```bash
docker buildx imagetools inspect "$PLOY_CONTAINER_REGISTRY/orw-cli-maven:latest"
docker buildx imagetools inspect "$PLOY_CONTAINER_REGISTRY/orw-cli-gradle:latest"
docker buildx imagetools inspect "$PLOY_CONTAINER_REGISTRY/java-17-codex-amata-maven:latest"
docker buildx imagetools inspect "$PLOY_CONTAINER_REGISTRY/java-17-codex-amata-gradle:latest"
docker buildx imagetools inspect "$PLOY_CONTAINER_REGISTRY/sbom-maven:jdk11"
docker buildx imagetools inspect "$PLOY_CONTAINER_REGISTRY/sbom-maven:jdk17"
docker buildx imagetools inspect "$PLOY_CONTAINER_REGISTRY/sbom-gradle:jdk11"
docker buildx imagetools inspect "$PLOY_CONTAINER_REGISTRY/sbom-gradle:jdk17"
```
