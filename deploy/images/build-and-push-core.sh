#!/usr/bin/env bash
set -Eeuo pipefail

# Build and push core images (server, node, garage-init, db) to GHCR.
#
# Inputs (env):
#   PLATFORM - Optional: comma list of platforms (default linux/amd64)
#   VERSION - Optional semver tag (default from ./VERSION file, format vX.Y.Z)
#   PUSH_LATEST - Optional alias toggle for :latest (default 1 for stable releases)
#
# Examples:
#   deploy/images/build-and-push-core.sh
#   VERSION=v0.1.0 PLATFORM=linux/amd64 deploy/images/build-and-push-core.sh

PLATFORM=${PLATFORM:-linux/amd64}
IMAGE_PREFIX="ghcr.io/iw2rmb"
PUSH_LATEST="${PUSH_LATEST:-1}"

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
  local -a refs=("${IMAGE_PREFIX}/${name}:${VERSION}")
  local -a tag_args=()
  local ref

  if [[ "$VERSION" != *-* ]]; then
    refs+=("${IMAGE_PREFIX}/${name}:v${SEMVER_MAJOR}")
    refs+=("${IMAGE_PREFIX}/${name}:v${SEMVER_MAJOR}.${SEMVER_MINOR}")
    case "$PUSH_LATEST" in
      1|true|TRUE|yes|YES|on|ON) refs+=("${IMAGE_PREFIX}/${name}:latest") ;;
    esac
  fi

  for ref in "${refs[@]}"; do
    tag_args+=(-t "$ref")
  done

  echo "==> Building ${refs[0]} (df=${dockerfile}, ctx=${context}, platform=${PLATFORM})"
  echo "    Tags: ${refs[*]}"
  docker buildx build \
    --platform "${PLATFORM}" \
    --provenance=false --sbom=false --pull \
    --label "org.opencontainers.image.version=${VERSION}" \
    --label "org.opencontainers.image.revision=${GIT_COMMIT}" \
    -f "${dockerfile}" \
    "${tag_args[@]}" \
    --push "${context}" --progress=plain
}

ROOT=$(git rev-parse --show-toplevel 2>/dev/null || pwd)
cd "$ROOT"
GIT_COMMIT="$(git rev-parse --short=12 HEAD 2>/dev/null || echo unknown)"

if [[ -z "${VERSION:-}" ]]; then
  if [[ -f "$ROOT/VERSION" ]]; then
    VERSION="$(tr -d '[:space:]' < "$ROOT/VERSION")"
  fi
fi
if [[ -z "${VERSION:-}" ]]; then
  echo "error: VERSION is required (set VERSION env var or create ./VERSION)" >&2
  exit 2
fi
if [[ ! "$VERSION" =~ ^v([0-9]+)\.([0-9]+)\.([0-9]+)(-[0-9A-Za-z][0-9A-Za-z.-]*)?$ ]]; then
  echo "error: VERSION '$VERSION' must match vX.Y.Z or vX.Y.Z-prerelease" >&2
  exit 2
fi
SEMVER_MAJOR="${BASH_REMATCH[1]}"
SEMVER_MINOR="${BASH_REMATCH[2]}"

# server
build_push ploy-server deploy/images/server/Dockerfile .

# node
build_push ploy-node deploy/images/node/Dockerfile .

# runtime garage bootstrap helper
build_push ploy-garage-init deploy/local/garage/Dockerfile .

# db
build_push ploy-db deploy/images/db/Dockerfile .

echo "All core images pushed under ${IMAGE_PREFIX} for ${VERSION}"
