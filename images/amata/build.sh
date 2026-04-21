#!/usr/bin/env bash
set -euo pipefail

cd $HOME/@iw2rmb/ploy

PLATFORM="${PLATFORM:-linux/amd64,linux/arm64}"
IMAGE_PREFIX="${PLOY_CONTAINER_REGISTRY:-ghcr.io/iw2rmb/ploy}"
BUILD_SECRET_ARGS=()
PLOY_CA_CERTS_TMP=""

cleanup() {
  if [[ -n "${PLOY_CA_CERTS_TMP}" && -f "${PLOY_CA_CERTS_TMP}" ]]; then
    rm -f "${PLOY_CA_CERTS_TMP}"
  fi
}
trap cleanup EXIT

if [[ -n "${PLOY_CA_CERTS:-}" ]]; then
  if [[ -f "${PLOY_CA_CERTS}" ]]; then
    BUILD_SECRET_ARGS+=(--secret "id=ploy_ca_certs,src=${PLOY_CA_CERTS}")
  else
    PLOY_CA_CERTS_TMP="$(mktemp)"
    printf '%s' "${PLOY_CA_CERTS}" >"${PLOY_CA_CERTS_TMP}"
    BUILD_SECRET_ARGS+=(--secret "id=ploy_ca_certs,src=${PLOY_CA_CERTS_TMP}")
  fi
fi

PLATFORM="$PLATFORM" images/amata/build-amata.sh
docker buildx build \
  --platform "$PLATFORM" \
  "${BUILD_SECRET_ARGS[@]}" \
  -f images/amata/Dockerfile \
  -t "${IMAGE_PREFIX}/amata:latest" \
  --push .
