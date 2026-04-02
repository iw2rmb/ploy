#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "$ROOT_DIR"

PLATFORM="${PLATFORM:-linux/amd64}"
IMAGE_PREFIX="${PLOY_CONTAINER_REGISTRY:-ghcr.io/iw2rmb/ploy}"

PLATFORM="$PLATFORM" images/amata/build-amata.sh
docker buildx build \
  --platform "$PLATFORM" \
  -f images/amata/Dockerfile \
  -t "${IMAGE_PREFIX}/amata:latest" \
  --push .
