#!/usr/bin/env bash

set -euo pipefail

# Build and optionally push the OpenRewrite JVM runner image
# Defaults target ghcr.io if REGISTRY not provided. Use -p to push.

REGISTRY=${REGISTRY:-"ghcr.io"}
OWNER_LOWER=${OWNER_LOWER:-"$(echo "${GITHUB_REPOSITORY_OWNER:-$(git config --get user.name || echo unknown)}" | tr '[:upper:]' '[:lower:]' | tr -cd '[:alnum:]-')"}
IMAGE_NAME=${IMAGE_NAME:-"openrewrite-jvm"}
IMAGE_TAG=${IMAGE_TAG:-"latest"}
PUSH=${PUSH:-"false"}

while getopts ":p" opt; do
  case $opt in
    p) PUSH="true" ;;
  esac
done

IMAGE_REF="${REGISTRY}/${OWNER_LOWER}/${IMAGE_NAME}:${IMAGE_TAG}"
IMAGE_REF_SHA="${REGISTRY}/${OWNER_LOWER}/${IMAGE_NAME}:sha-$(git rev-parse --short HEAD)"

echo "[ORW] Building image: ${IMAGE_REF}"
docker build -t "${IMAGE_REF}" -t "${IMAGE_REF_SHA}" -f services/openrewrite-jvm/Dockerfile services/openrewrite-jvm

docker images "${IMAGE_REF}" --format 'size={{.Size}}' || true

if [[ "${PUSH}" == "true" ]]; then
  echo "[ORW] Pushing image tags: ${IMAGE_REF} and ${IMAGE_REF_SHA}"
  docker push "${IMAGE_REF}"
  docker push "${IMAGE_REF_SHA}"
else
  echo "[ORW] Skipping push (use -p or PUSH=true to push)"
fi

echo "[ORW] Done."

