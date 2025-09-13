#!/usr/bin/env bash
set -euo pipefail

# Build and optionally push the LangGraph runner image (planner/reducer/llm-exec stub)

REGISTRY=${REGISTRY:-"ghcr.io"}
OWNER_LOWER=${OWNER_LOWER:-"$(echo "${GITHUB_REPOSITORY_OWNER:-$(git config --get user.name || echo unknown)}" | tr '[:upper:]' '[:lower:]' | tr -cd '[:alnum:]-')"}
IMAGE_NAME=${IMAGE_NAME:-"langgraph-runner"}
IMAGE_TAG=${IMAGE_TAG:-"py-0.1.0"}
PUSH=${PUSH:-"false"}

while getopts ":p" opt; do
  case $opt in
    p) PUSH="true" ;;
  esac
done

IMAGE_REF="${REGISTRY}/${OWNER_LOWER}/${IMAGE_NAME}:${IMAGE_TAG}"
IMAGE_REF_SHA="${REGISTRY}/${OWNER_LOWER}/${IMAGE_NAME}:sha-$(git rev-parse --short HEAD)"

echo "[LG] Building image: ${IMAGE_REF}"
docker build -t "${IMAGE_REF}" -t "${IMAGE_REF_SHA}" -f services/langgraph-runner/Dockerfile services/langgraph-runner

docker images "${IMAGE_REF}" --format 'size={{.Size}}' || true

if [[ "${PUSH}" == "true" ]]; then
  echo "[LG] Pushing image tags: ${IMAGE_REF} and ${IMAGE_REF_SHA}"
  docker push "${IMAGE_REF}"
  docker push "${IMAGE_REF_SHA}"
else
  echo "[LG] Skipping push (use -p or PUSH=true to push)"
fi

echo "[LG] Done."

