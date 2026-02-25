Publish Mods Images to a Garage-Backed Registry

Overview
- Mods images live under `deploy/images/migs/`:
  - `orw-maven` -> `migs-orw-maven`
  - `orw-gradle` -> `migs-orw-gradle`
  - `mig-codex` -> `migs-codex`
  - `mig-llm` -> `migs-llm`
  - `mig-plan` -> `migs-plan`
- Local default registry prefix is `127.0.0.1:5000/ploy`.
- The runner resolves images as `$PLOY_CONTAINER_REGISTRY/<name>:latest`.

Local Registry Prerequisites
- Deploy the local stack:
  - `deploy/local/run.sh`
- Export the registry prefix for specs/scripts:
  - `export PLOY_CONTAINER_REGISTRY=127.0.0.1:5000/ploy`

Publish all Mods images
```bash
deploy/images/build-and-push-migs.sh
# Discovers deploy/images/migs/* and pushes :latest tags.
# Defaults to IMAGE_PREFIX=${PLOY_CONTAINER_REGISTRY:-127.0.0.1:5000/ploy}.
```

Sync all local workflow images (migs + build-gate base images)
```bash
deploy/images/garage.sh
# Adds build-gate images and mirrored base images required by etc/ploy/gates/build-gate-images.yaml.
# Skips refs that already exist in registry.
# Use --force to rebuild/repush everything.
```

If your network requires a custom CA for package downloads during image builds,
set:
```bash
export PLOY_CA_CERTS=/absolute/path/to/ca-bundle.pem
```
`deploy/images/garage.sh` passes this bundle as a BuildKit secret (`ploy_ca_bundle`)
so mig images can trust internal TLS endpoints.

Publish a single Mods image (example: orw-maven)
```bash
name=orw-maven
IMAGE_PREFIX="${PLOY_CONTAINER_REGISTRY:-127.0.0.1:5000/ploy}" \
  docker buildx build --platform linux/amd64 \
  -t "${IMAGE_PREFIX}/migs-orw-maven:latest" \
  --push "deploy/images/migs/${name}"
```

Publish `migs-codex` (manual one-off)
```bash
IMAGE_PREFIX="${PLOY_CONTAINER_REGISTRY:-127.0.0.1:5000/ploy}"
docker buildx build \
  --platform linux/amd64 \
  -f deploy/images/migs/mig-codex/Dockerfile \
  -t "${IMAGE_PREFIX}/migs-codex:latest" \
  --push .
```

Stack-aware image mapping example
```yaml
image:
  default: ${PLOY_CONTAINER_REGISTRY}/migs-orw-maven:latest
  java-maven: ${PLOY_CONTAINER_REGISTRY}/migs-orw-maven:latest
  java-gradle: ${PLOY_CONTAINER_REGISTRY}/migs-orw-gradle:latest
```

Notes
- Directory mapping:
  - `mig-foo` -> `migs-foo`
  - `orw-maven` -> `migs-orw-maven`
  - `orw-gradle` -> `migs-orw-gradle`
- To use a different registry/namespace, override:
  - `IMAGE_PREFIX=... deploy/images/build-and-push-migs.sh`

Multi-arch push
```bash
PLATFORM=linux/amd64,linux/arm64 deploy/images/build-and-push-migs.sh
```

Verification
```bash
docker buildx imagetools inspect "${PLOY_CONTAINER_REGISTRY}/migs-orw-maven:latest"
docker buildx imagetools inspect "${PLOY_CONTAINER_REGISTRY}/migs-orw-gradle:latest"
docker buildx imagetools inspect "${PLOY_CONTAINER_REGISTRY}/migs-codex:latest"
```
