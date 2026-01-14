#!/usr/bin/env bash
set -Eeuo pipefail

# Build and push core images (server, node, db) to GitHub Container Registry.
#
# Inputs (env):
#   PLATFORM - Optional: comma list of platforms (default linux/amd64)
#
# Examples:
#   scripts/docker/build-and-push-core-ghcr.sh
#   PLATFORM=linux/amd64,linux/arm64 scripts/docker/build-and-push-core-ghcr.sh

PLATFORM=${PLATFORM:-linux/amd64}
IMAGE_PREFIX="ghcr.io/iw2rmb"

if ! command -v docker >/dev/null 2>&1; then
  echo "error: docker CLI not found" >&2; exit 2
fi
if ! docker buildx version >/dev/null 2>&1; then
  echo "error: docker buildx not available (install docker buildx plugin)" >&2; exit 2
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
