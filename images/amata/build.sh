#!/usr/bin/env bash
set -euo pipefail

cd $HOME/@iw2rmb/ploy

PLATFORM="${PLATFORM:-linux/amd64}"
IMAGE_PREFIX="${PLOY_CONTAINER_REGISTRY:-ghcr.io/iw2rmb/ploy}"

PLATFORM="$PLATFORM" images/amata/build-amata.sh
docker buildx build \
  --platform "$PLATFORM" \
  -f images/amata/Dockerfile \
  -t "${IMAGE_PREFIX}/amata:latest" \
  --push .
