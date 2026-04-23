#!/usr/bin/env bash
set -Eeuo pipefail

# Build and push runtime/migs images.
#
# Builds and pushes:
# - server  -> server
# - node    -> node
# - amata   -> amata
# - java-17-orw-codex-amata -> java-17-orw-codex-amata
# - sbom runners -> sbom-gradle, sbom-maven
# - gate-gradle -> gate-gradle:jdk11, gate-gradle:jdk17
# - gate-maven  -> maven:3-eclipse-temurin-11, maven:3-eclipse-temurin-17
# - orw/*   -> <dir name> (for example: orw-cli-maven, orw-cli-gradle)
#
# Inputs (env):
#   PLATFORM - Optional: comma list of platforms (default linux/amd64)
#   VERSION - Optional semver tag (default from ./VERSION file, format vX.Y.Z)
#   IMAGE_PREFIX - Optional image prefix (default ghcr.io/iw2rmb/ploy)
#   OUTPUT_MODE - Optional: push|load (default push)
#   PUSH_LATEST - Optional alias toggle for :latest (default 1 for stable releases)
#   PLOY_CA_CERTS - Optional PEM bundle (path or inline content), passed as BuildKit secret id=ploy_ca_certs
#
# CLI options:
#   --build <groups> - Optional comma list from: orw,ploy,amata,sbom,gates (default: all)
#   --help           - Show usage
#
# Examples:
#   images/build-and-push.sh
#   VERSION=v0.1.0 PLATFORM=linux/amd64 images/build-and-push.sh
#   OUTPUT_MODE=load IMAGE_PREFIX=ploy VERSION=v0.1.0 images/build-and-push.sh
#   images/build-and-push.sh --build ploy,orw

PLATFORM=${PLATFORM:-linux/amd64}
IMAGE_PREFIX="${IMAGE_PREFIX:-ghcr.io/iw2rmb/ploy}"
OUTPUT_MODE="${OUTPUT_MODE:-push}"
PUSH_LATEST="${PUSH_LATEST:-1}"
declare -a BUILD_SECRET_ARGS=()
PLOY_CA_CERTS_TMP=""
declare -a BUILD_OUTPUT_ARGS=(--push)
declare -a GROUP_ORDER=("ploy" "amata" "sbom" "gates" "orw")
BUILD_GROUPS_RAW=""
declare -a SELECTED_GROUPS=()

usage() {
  cat <<'USAGE' >&2
Usage: images/build-and-push.sh [--build <groups>]

Options:
  --build <groups>  Comma-separated groups from: orw,ploy,amata,sbom,gates
                    Default: all groups
  -h, --help        Show this help
USAGE
}

contains_value() {
  local needle="$1"
  shift
  local value
  for value in "$@"; do
    if [[ "$value" == "$needle" ]]; then
      return 0
    fi
  done
  return 1
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --build)
        if [[ $# -lt 2 ]]; then
          echo "error: --build requires a value" >&2
          usage
          exit 2
        fi
        BUILD_GROUPS_RAW="$2"
        shift 2
        ;;
      --build=*)
        BUILD_GROUPS_RAW="${1#*=}"
        shift
        ;;
      -h|--help)
        usage
        exit 0
        ;;
      *)
        echo "error: unknown argument: $1" >&2
        usage
        exit 2
        ;;
    esac
  done
}

normalize_selected_groups() {
  local -a requested=()
  if [[ -z "$BUILD_GROUPS_RAW" ]]; then
    SELECTED_GROUPS=("${GROUP_ORDER[@]}")
    return 0
  fi

  local token normalized
  IFS=',' read -r -a requested <<<"$BUILD_GROUPS_RAW"
  if [[ ${#requested[@]} -eq 0 ]]; then
    echo "error: --build must not be empty" >&2
    exit 2
  fi

  local -a uniq_requested=()
  for token in "${requested[@]}"; do
    normalized="$(echo "$token" | xargs)"
    if [[ -z "$normalized" ]]; then
      echo "error: --build contains an empty group value" >&2
      exit 2
    fi
    if ! contains_value "$normalized" "${GROUP_ORDER[@]}"; then
      echo "error: unknown build group '$normalized' (allowed: orw,ploy,amata,sbom,gates)" >&2
      exit 2
    fi
    if ! contains_value "$normalized" "${uniq_requested[@]-}"; then
      uniq_requested+=("$normalized")
    fi
  done

  SELECTED_GROUPS=()
  for token in "${GROUP_ORDER[@]}"; do
    if contains_value "$token" "${uniq_requested[@]-}"; then
      SELECTED_GROUPS+=("$token")
    fi
  done
}

group_selected() {
  local group="$1"
  contains_value "$group" "${SELECTED_GROUPS[@]-}"
}

parse_args "$@"
normalize_selected_groups

if ! command -v docker >/dev/null 2>&1; then
  echo "error: docker CLI not found" >&2
  exit 2
fi
if ! docker buildx version >/dev/null 2>&1; then
  echo "error: docker buildx not available (install docker buildx plugin)" >&2
  exit 2
fi

case "$OUTPUT_MODE" in
  push) BUILD_OUTPUT_ARGS=(--push) ;;
  load) BUILD_OUTPUT_ARGS=(--load) ;;
  *)
    echo "error: OUTPUT_MODE '$OUTPUT_MODE' must be one of: push, load" >&2
    exit 2
    ;;
esac

cleanup() {
  if [[ -n "${PLOY_CA_CERTS_TMP}" && -f "${PLOY_CA_CERTS_TMP}" ]]; then
    rm -f "${PLOY_CA_CERTS_TMP}"
  fi
}
trap cleanup EXIT

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

if [[ -n "${PLOY_CA_CERTS:-}" ]]; then
  if [[ -f "${PLOY_CA_CERTS}" ]]; then
    BUILD_SECRET_ARGS+=(--secret "id=ploy_ca_certs,src=${PLOY_CA_CERTS}")
    echo "==> Using build CA bundle from file path in PLOY_CA_CERTS"
  else
    PLOY_CA_CERTS_TMP="$(mktemp)"
    printf '%s' "${PLOY_CA_CERTS}" >"${PLOY_CA_CERTS_TMP}"
    BUILD_SECRET_ARGS+=(--secret "id=ploy_ca_certs,src=${PLOY_CA_CERTS_TMP}")
    echo "==> Using build CA bundle from inline PEM content in PLOY_CA_CERTS"
  fi
fi

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
    "${BUILD_SECRET_ARGS[@]}" \
    -f "${dockerfile}" \
    "${tag_args[@]}" \
    "${BUILD_OUTPUT_ARGS[@]}" "${context}" --progress=plain
}

build_push_orw() {
  local dir="$1"
  local image_name
  image_name="$(basename "$dir")"
  local dockerfile="images/orw/${image_name}/Dockerfile"

  # Current ORW images share repo-root context because Dockerfiles copy shared files.
  build_push "$image_name" "$dockerfile" "."
}

build_push_fixed_tag() {
  local image_name="$1"
  local dockerfile="$2"
  local context="$3"
  local tag="$4"
  local ref="${IMAGE_PREFIX}/${image_name}:${tag}"

  echo "==> Building ${ref} (df=${dockerfile}, ctx=${context}, platform=${PLATFORM})"
  docker buildx build \
    --platform "${PLATFORM}" \
    --provenance=false --sbom=false --pull \
    --label "org.opencontainers.image.version=${VERSION}" \
    --label "org.opencontainers.image.revision=${GIT_COMMIT}" \
    "${BUILD_SECRET_ARGS[@]}" \
    -f "${dockerfile}" \
    -t "${ref}" \
    "${BUILD_OUTPUT_ARGS[@]}" "${context}" --progress=plain
}

echo "==> Selected build groups: ${SELECTED_GROUPS[*]}"

if group_selected "ploy"; then
  make build PLOY_BUILD_PLATFORMS="${PLATFORM}"

  # server
  build_push server images/server/Dockerfile .

  # node
  build_push node images/node/Dockerfile .
fi

if group_selected "amata"; then
  PLATFORM="${PLATFORM}" bash images/amata/build-amata.sh
  build_push amata images/amata/Dockerfile .
  build_push java-17-orw-codex-amata images/java-17-orw-codex-amata/Dockerfile .
fi

if group_selected "sbom"; then
  # sbom runners
  build_push sbom-gradle images/sbom/gradle/Dockerfile .
  build_push sbom-maven images/sbom/maven/Dockerfile .
fi

if group_selected "gates"; then
  # build gate (gradle)
  build_push_fixed_tag gate-gradle images/gates/gradle/Dockerfile.jdk11 . jdk11
  build_push_fixed_tag gate-gradle images/gates/gradle/Dockerfile.jdk17 . jdk17

  # build gate (maven)
  build_push_fixed_tag maven images/gates/maven/Dockerfile.jdk11 . 3-eclipse-temurin-11
  build_push_fixed_tag maven images/gates/maven/Dockerfile.jdk17 . 3-eclipse-temurin-17
fi

if group_selected "orw"; then
  # orw
  orw_dirs=()
  while IFS= read -r d; do
    orw_dirs+=("$d")
  done < <(
    find images/orw -mindepth 1 -maxdepth 1 -type d \
      -exec test -f "{}/Dockerfile" \; -print | sort
  )
  if [[ ${#orw_dirs[@]} -eq 0 ]]; then
    echo "error: no ORW image directories with Dockerfile found under images/orw" >&2
    exit 1
  fi
  for d in "${orw_dirs[@]}"; do
    build_push_orw "$d"
  done
fi

if [[ "$OUTPUT_MODE" == "push" ]]; then
  echo "All images pushed under ${IMAGE_PREFIX} for ${VERSION}"
else
  echo "All images loaded into local Docker image store under ${IMAGE_PREFIX} for ${VERSION}"
fi
