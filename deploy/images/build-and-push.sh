#!/usr/bin/env bash
set -Eeuo pipefail

# Build and push runtime/migs images.
#
# Builds and pushes:
# - server  -> ploy-server
# - node    -> ploy-node
# - codex   -> migs-codex
# - amata   -> migs-amata
# - shell   -> migs-shell
# - orw/*   -> <dir name> (for example: orw-cli-maven, orw-cli-gradle)
#
# Inputs (env):
#   PLATFORM - Optional: comma list of platforms (default linux/amd64)
#   VERSION - Optional semver tag (default from ./VERSION file, format vX.Y.Z)
#   IMAGE_PREFIX - Optional image prefix (default ghcr.io/iw2rmb)
#   PUSH_LATEST - Optional alias toggle for :latest (default 1 for stable releases)
#
# Examples:
#   deploy/images/build-and-push.sh
#   VERSION=v0.1.0 PLATFORM=linux/amd64 deploy/images/build-and-push.sh

PLATFORM=${PLATFORM:-linux/amd64}
IMAGE_PREFIX="${IMAGE_PREFIX:-ghcr.io/iw2rmb/ploy}"
PUSH_LATEST="${PUSH_LATEST:-1}"

if ! command -v docker >/dev/null 2>&1; then
  echo "error: docker CLI not found" >&2
  exit 2
fi
if ! docker buildx version >/dev/null 2>&1; then
  echo "error: docker buildx not available (install docker buildx plugin)" >&2
  exit 2
fi

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

build_push() {
  local name="$1"
  local dockerfile="$2"
  local context="$3"
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

build_push_orw() {
  local dir="$1"
  local image_name
  image_name="$(basename "$dir")"
  local dockerfile="deploy/images/orw/${image_name}/Dockerfile"

  # Current ORW images share repo-root context because Dockerfiles copy shared files.
  build_push "$image_name" "$dockerfile" "."
}

# server
build_push ploy-server deploy/images/server/Dockerfile .

# node
build_push ploy-node deploy/images/node/Dockerfile .

# codex
build_push migs-codex deploy/images/codex/Dockerfile .

# amata
bash deploy/images/amata/build-amata.sh
build_push migs-amata deploy/images/amata/Dockerfile .

# shell
build_push migs-shell deploy/images/shell/Dockerfile deploy/images/shell

# orw
mapfile -t orw_dirs < <(
  find deploy/images/orw -mindepth 1 -maxdepth 1 -type d \
    -exec test -f "{}/Dockerfile" \; -print | sort
)
if [[ ${#orw_dirs[@]} -eq 0 ]]; then
  echo "error: no ORW image directories with Dockerfile found under deploy/images/orw" >&2
  exit 1
fi
for d in "${orw_dirs[@]}"; do
  build_push_orw "$d"
done

echo "All images pushed under ${IMAGE_PREFIX} for ${VERSION}"
