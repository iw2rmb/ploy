Publish Mods Images to a Garage-Backed Registry

Overview
- Mods images live under `deploy/images/migs/` and `deploy/images/mig/`:
  - `orw-cli` (`deploy/images/mig/orw-cli`) -> `orw-cli`
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
# Discovers deploy/images/migs/* and deploy/images/mig/* and pushes :latest tags.
# Defaults to IMAGE_PREFIX=${PLOY_CONTAINER_REGISTRY:-127.0.0.1:5000/ploy}.
```

Sync all local workflow images (migs + build-gate base images)
```bash
deploy/images/garage.sh
# Adds build-gate images and mirrored base images required by gates/stacks.yaml.
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

Publish a single Mods image (example: orw-cli)
```bash
IMAGE_PREFIX="${PLOY_CONTAINER_REGISTRY:-127.0.0.1:5000/ploy}" \
  docker buildx build --platform linux/amd64 \
  -t "${IMAGE_PREFIX}/orw-cli:latest" \
  ${PLOY_CA_CERTS:+--secret id=ploy_ca_bundle,src=${PLOY_CA_CERTS}} \
  --push "deploy/images/mig/orw-cli"
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
  default: ${PLOY_CONTAINER_REGISTRY}/orw-cli:latest
  java-maven: ${PLOY_CONTAINER_REGISTRY}/orw-cli:latest
  java-gradle: ${PLOY_CONTAINER_REGISTRY}/orw-cli:latest
```

Notes
- Directory mapping:
  - `mig-foo` -> `migs-foo`
  - `orw-cli` -> `orw-cli`
- To use a different registry/namespace, override:
  - `IMAGE_PREFIX=... deploy/images/build-and-push-migs.sh`

Multi-arch push
```bash
PLATFORM=linux/amd64,linux/arm64 deploy/images/build-and-push-migs.sh
```

Verification
```bash
docker buildx imagetools inspect "${PLOY_CONTAINER_REGISTRY}/orw-cli:latest"
docker buildx imagetools inspect "${PLOY_CONTAINER_REGISTRY}/migs-codex:latest"
```
