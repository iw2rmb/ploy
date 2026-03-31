Publish Migs Images to a Garage-Backed Registry

Overview
- Migs images live under `deploy/images/migs/` and `deploy/images/mig/`:
  - `orw-cli-maven` (`deploy/images/mig/orw-cli-maven`) -> `orw-cli-maven`
  - `orw-cli-gradle` (`deploy/images/mig/orw-cli-gradle`) -> `orw-cli-gradle`
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

Publish all Migs images
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

Custom CA support is runtime-only. Do not inject corporate certs during image build.
Use `PLOY_CA_CERTS` at deployment/runtime so the same bundle is mounted into runtime
containers and propagated as `CA_CERTS_PEM_BUNDLE`.

Publish a single Migs image (example: orw-cli-maven)
```bash
IMAGE_PREFIX="${PLOY_CONTAINER_REGISTRY:-127.0.0.1:5000/ploy}" \
  docker buildx build --platform linux/amd64 \
  -f deploy/images/mig/orw-cli-maven/Dockerfile \
  -t "${IMAGE_PREFIX}/orw-cli-maven:latest" \
  --push .
```

Publish `migs-codex` (manual one-off)

`migs-codex` embeds a locally built `amata` binary. Build it before the Docker
image build — the Dockerfile copies the staged binary; no in-image compilation occurs.

```bash
# Step 1: build and stage the amata binary (requires ../amata source sibling repo)
PLATFORM=linux/amd64 deploy/images/migs/mig-codex/build-amata.sh

# Step 2: build and push the migs-codex image
IMAGE_PREFIX="${PLOY_CONTAINER_REGISTRY:-127.0.0.1:5000/ploy}"
docker buildx build \
  --platform linux/amd64 \
  -f deploy/images/migs/mig-codex/Dockerfile \
  -t "${IMAGE_PREFIX}/migs-codex:latest" \
  --push .
```

`build-amata.sh` expects the `amata` repository to be a sibling of `ploy` at
`../amata`. It fails fast with a clear error when the source or build output is
missing.

Stack-aware image mapping example
```yaml
image:
  default: ${PLOY_CONTAINER_REGISTRY}/orw-cli-maven:latest
  java-maven: ${PLOY_CONTAINER_REGISTRY}/orw-cli-maven:latest
  java-gradle: ${PLOY_CONTAINER_REGISTRY}/orw-cli-gradle:latest
```

Notes
- Directory mapping:
  - `mig-foo` -> `migs-foo`
  - `orw-cli-maven` -> `orw-cli-maven`
  - `orw-cli-gradle` -> `orw-cli-gradle`
- To use a different registry/namespace, override:
  - `IMAGE_PREFIX=... deploy/images/build-and-push-migs.sh`

Multi-arch push
```bash
PLATFORM=linux/amd64 deploy/images/build-and-push-migs.sh
```

Verification
```bash
docker buildx imagetools inspect "${PLOY_CONTAINER_REGISTRY}/orw-cli-maven:latest"
docker buildx imagetools inspect "${PLOY_CONTAINER_REGISTRY}/orw-cli-gradle:latest"
docker buildx imagetools inspect "${PLOY_CONTAINER_REGISTRY}/migs-codex:latest"
```
