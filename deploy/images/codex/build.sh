#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "$ROOT_DIR"

PLATFORM="${PLATFORM:-linux/amd64}"
IMAGE_PREFIX="${PLOY_CONTAINER_REGISTRY:-127.0.0.1:5000/ploy}"

docker buildx build \
  --platform "$PLATFORM" \
  -f deploy/images/codex/Dockerfile \
  -t "${IMAGE_PREFIX}/codex:latest" \
  --push .
