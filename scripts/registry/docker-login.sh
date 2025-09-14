#!/usr/bin/env bash
set -euo pipefail

# Helper: login to the internal Docker registry used by the VPS
# Usage:
#   REGISTRY_URL=registry.dev.ployman.app \
#   REGISTRY_USERNAME=<user> \
#   REGISTRY_PASSWORD=<pass> \
#   ./scripts/registry/docker-login.sh

REGISTRY_URL=${REGISTRY_URL:-}
REGISTRY_USERNAME=${REGISTRY_USERNAME:-}
REGISTRY_PASSWORD=${REGISTRY_PASSWORD:-}

if [[ -z "${REGISTRY_URL}" || -z "${REGISTRY_USERNAME}" || -z "${REGISTRY_PASSWORD}" ]]; then
  echo "usage: REGISTRY_URL=... REGISTRY_USERNAME=... REGISTRY_PASSWORD=... $0" >&2
  exit 2
fi

echo "Logging into ${REGISTRY_URL} ..."
echo "${REGISTRY_PASSWORD}" | docker login "${REGISTRY_URL}" --username "${REGISTRY_USERNAME}" --password-stdin
echo "Login successful. Docker config: ${HOME}/.docker/config.json"

