Publish Migs Images to a Local Registry

Overview
- Migs images live under `images/orw/` and `images/amata/`:
  - `orw-cli-maven` (`images/orw/orw-cli-maven`) -> `orw-cli-maven`
  - `orw-cli-gradle` (`images/orw/orw-cli-gradle`) -> `orw-cli-gradle`
  - `orw-cli-maven-jdk21` (`images/orw/orw-cli-maven-jdk21`) -> `orw-cli-maven-jdk21`
  - `orw-cli-maven-jdk25` (`images/orw/orw-cli-maven-jdk25`) -> `orw-cli-maven-jdk25`
  - `orw-cli-gradle-jdk21` (`images/orw/orw-cli-gradle-jdk21`) -> `orw-cli-gradle-jdk21`
  - `orw-cli-gradle-jdk25` (`images/orw/orw-cli-gradle-jdk25`) -> `orw-cli-gradle-jdk25`
  - `java-17-codex-amata-maven` (`images/amata/java-17-codex-amata-maven`) -> `java-17-codex-amata-maven`
  - `java-17-codex-amata-gradle` (`images/amata/java-17-codex-amata-gradle`) -> `java-17-codex-amata-gradle`
  - `java-21-codex-amata-maven` (`images/amata/java-21-codex-amata-maven`) -> `java-21-codex-amata-maven`
  - `java-21-codex-amata-gradle` (`images/amata/java-21-codex-amata-gradle`) -> `java-21-codex-amata-gradle`
  - `java-25-codex-amata-maven` (`images/amata/java-25-codex-amata-maven`) -> `java-25-codex-amata-maven`
  - `java-25-codex-amata-gradle` (`images/amata/java-25-codex-amata-gradle`) -> `java-25-codex-amata-gradle`
- The runner resolves most mig images as `$PLOY_CONTAINER_REGISTRY/<name>:latest`.

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
# Builds and pushes:
# - Amata lanes: java-17/21/25-codex-amata-{maven,gradle}
# - ORW lanes: orw-cli-{maven,gradle} and orw-cli-{maven,gradle}-jdk{21,25}
# - Gate Gradle: gate-gradle:jdk11,jdk17,jdk21,jdk25
# - Gate Maven: maven:3-eclipse-temurin-11,17,21,25
# - Java bases: java-base-maven:jdk11,jdk17,jdk21,jdk25
#               java-base-gradle:jdk11,jdk17,jdk21,jdk25
#               java-base-temurin:jdk17,jdk21,jdk25
# - Runtime images: server and node
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
  - `$PLOY_CONTAINER_REGISTRY/gate-gradle:jdk21`
  - `$PLOY_CONTAINER_REGISTRY/gate-gradle:jdk25`
  - `$PLOY_CONTAINER_REGISTRY/maven:3-eclipse-temurin-11`
  - `$PLOY_CONTAINER_REGISTRY/maven:3-eclipse-temurin-17`
  - `$PLOY_CONTAINER_REGISTRY/maven:3-eclipse-temurin-21`
  - `$PLOY_CONTAINER_REGISTRY/maven:3-eclipse-temurin-25`

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
  - `java-21-codex-amata-maven` -> `java-21-codex-amata-maven`
  - `java-21-codex-amata-gradle` -> `java-21-codex-amata-gradle`
  - `java-25-codex-amata-maven` -> `java-25-codex-amata-maven`
  - `java-25-codex-amata-gradle` -> `java-25-codex-amata-gradle`
  - `orw-cli-maven` -> `orw-cli-maven`
  - `orw-cli-gradle` -> `orw-cli-gradle`
  - `orw-cli-maven-jdk21` -> `orw-cli-maven-jdk21`
  - `orw-cli-maven-jdk25` -> `orw-cli-maven-jdk25`
  - `orw-cli-gradle-jdk21` -> `orw-cli-gradle-jdk21`
  - `orw-cli-gradle-jdk25` -> `orw-cli-gradle-jdk25`
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
docker buildx imagetools inspect "$PLOY_CONTAINER_REGISTRY/orw-cli-maven-jdk21:latest"
docker buildx imagetools inspect "$PLOY_CONTAINER_REGISTRY/orw-cli-maven-jdk25:latest"
docker buildx imagetools inspect "$PLOY_CONTAINER_REGISTRY/orw-cli-gradle-jdk21:latest"
docker buildx imagetools inspect "$PLOY_CONTAINER_REGISTRY/orw-cli-gradle-jdk25:latest"
docker buildx imagetools inspect "$PLOY_CONTAINER_REGISTRY/java-17-codex-amata-maven:latest"
docker buildx imagetools inspect "$PLOY_CONTAINER_REGISTRY/java-17-codex-amata-gradle:latest"
docker buildx imagetools inspect "$PLOY_CONTAINER_REGISTRY/java-21-codex-amata-maven:latest"
docker buildx imagetools inspect "$PLOY_CONTAINER_REGISTRY/java-21-codex-amata-gradle:latest"
docker buildx imagetools inspect "$PLOY_CONTAINER_REGISTRY/java-25-codex-amata-maven:latest"
docker buildx imagetools inspect "$PLOY_CONTAINER_REGISTRY/java-25-codex-amata-gradle:latest"
```
