#!/usr/bin/env bash
set -Eeuo pipefail

# Build and push core images (server, node, db) to Docker Hub (or custom registry prefix).
#
# Inputs (env):
#   DOCKERHUB_USERNAME   - Docker Hub username/namespace (used when IMAGE_PREFIX unset)
#   DOCKERHUB_PAT        - Optional: PAT for non-interactive docker login
#   IMAGE_PREFIX         - Optional: absolute registry prefix (e.g., docker.io/org, ghcr.io/org)
#   PLATFORM             - Optional: comma list of platforms (default linux/amd64)
#
# Examples:
#   DOCKERHUB_USERNAME=iw2rmb DOCKERHUB_PAT=*** scripts/docker/build-and-push-core.sh
#   IMAGE_PREFIX=ghcr.io/acme PLATFORM=linux/amd64,linux/arm64 scripts/docker/build-and-push-core.sh

PLATFORM=${PLATFORM:-linux/amd64}
DOCKERHUB_USERNAME=${DOCKERHUB_USERNAME:-}
IMAGE_PREFIX=${IMAGE_PREFIX:-}

if [[ -z "$IMAGE_PREFIX" ]]; then
  if [[ -z "$DOCKERHUB_USERNAME" ]]; then
    echo "error: set DOCKERHUB_USERNAME or IMAGE_PREFIX" >&2
    exit 2
  fi
  IMAGE_PREFIX="docker.io/${DOCKERHUB_USERNAME}"
fi

if ! command -v docker >/dev/null 2>&1; then
  echo "error: docker CLI not found" >&2; exit 2
fi
if ! docker buildx version >/dev/null 2>&1; then
  echo "error: docker buildx not available (install docker buildx plugin)" >&2; exit 2
fi

if [[ -n "${DOCKERHUB_PAT:-}" && "$IMAGE_PREFIX" == docker.io/* ]]; then
  echo "Logging in to Docker Hub as ${DOCKERHUB_USERNAME}"
  printf '%s' "$DOCKERHUB_PAT" | docker login -u "$DOCKERHUB_USERNAME" --password-stdin >/dev/null 2>&1 || {
    echo "error: docker login failed" >&2; exit 2; }
fi

build_push() {
  local name="$1"; shift
  local dockerfile="$1"; shift
  local context="$1"; shift
  local ref="${IMAGE_PREFIX}/${name}:latest"
  echo "==> Building ${ref} (df=${dockerfile}, ctx=${context}, platform=${PLATFORM})"
  docker buildx build \
    --platform "${PLATFORM}" \
    --provenance=false --sbom=false --pull \
    -f "${dockerfile}" -t "${ref}" --push "${context}" --progress=plain
}

ROOT=$(git rev-parse --show-toplevel 2>/dev/null || pwd)
cd "$ROOT"

# server
build_push ploy-server docker/server/Dockerfile .

# node
build_push ploy-node docker/node/Dockerfile .

# db
build_push ploy-db docker/db/Dockerfile .

echo "All core images pushed under ${IMAGE_PREFIX}"

