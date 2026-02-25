#!/usr/bin/env bash
set -Eeuo pipefail

# Build and push mig/build-gate images to the Garage-backed registry.
#
# Behavior:
# - Skips images that already exist in the target registry.
# - Use --force to rebuild/repush everything.
#
# Inputs (env):
#   IMAGE_PREFIX  Registry/repo prefix (default: ${PLOY_CONTAINER_REGISTRY:-localhost:5000/ploy})
#   PLATFORM      Build platforms for buildx (default: linux/amd64)
#   PUSH_TIMEOUT  Per-command timeout in seconds (default: 900)
#   PUSH_RETRIES  Retries on failure (default: 1)
#   REGISTRY_SCHEME Optional override for registry API scheme (http|https)

PLATFORM=${PLATFORM:-linux/amd64}
PUSH_TIMEOUT=${PUSH_TIMEOUT:-900}
PUSH_RETRIES=${PUSH_RETRIES:-1}
IMAGE_PREFIX="${IMAGE_PREFIX:-${PLOY_CONTAINER_REGISTRY:-localhost:5000/ploy}}"
REGISTRY_SCHEME="${REGISTRY_SCHEME:-}"
FORCE=0

usage() {
  cat <<'USAGE'
Usage: deploy/images/garage.sh [--force]

Options:
  --force, -f  Build/push all images even if they already exist in registry
  --help, -h   Show this help
USAGE
}

log() {
  echo "[$(date -u +%H:%M:%S)] $*"
}

need() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "error: missing dependency: $1" >&2
    exit 2
  fi
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --force|-f)
        FORCE=1
        ;;
      --help|-h)
        usage
        exit 0
        ;;
      *)
        echo "error: unknown argument: $1" >&2
        usage >&2
        exit 2
        ;;
    esac
    shift
  done
}

with_timeout() {
  local secs=$1
  shift
  if command -v timeout >/dev/null 2>&1; then
    timeout "${secs}s" "$@"
  elif command -v gtimeout >/dev/null 2>&1; then
    gtimeout "${secs}s" "$@"
  else
    perl -e 'alarm shift @ARGV; exec @ARGV' "$secs" "$@"
  fi
}

run_with_retries() {
  local label="$1"
  shift

  local attempt=0
  while :; do
    attempt=$((attempt + 1))
    log "[attempt ${attempt}/${PUSH_RETRIES}] ${label}"
    if with_timeout "$PUSH_TIMEOUT" "$@"; then
      return 0
    fi
    if (( attempt >= PUSH_RETRIES )); then
      echo "error: failed after ${attempt} attempt(s): ${label}" >&2
      return 1
    fi
    sleep 3
  done
}

registry_scheme_for_host() {
  local host="$1"
  if [[ -n "$REGISTRY_SCHEME" ]]; then
    echo "$REGISTRY_SCHEME"
    return
  fi
  case "$host" in
    localhost|localhost:*|127.0.0.1|127.0.0.1:*|[::1]|[::1]:*)
      echo "http"
      ;;
    *)
      echo "https"
      ;;
  esac
}

image_exists() {
  local ref="$1"
  local host repo tag scheme url code

  # Parse <host>/<repo...>:<tag>; default tag=latest when omitted.
  local repo_with_host last_segment
  last_segment="${ref##*/}"
  if [[ "$last_segment" == *:* ]]; then
    tag="${last_segment##*:}"
    repo_with_host="${ref%:*}"
  else
    tag="latest"
    repo_with_host="$ref"
  fi

  host="${repo_with_host%%/*}"
  repo="${repo_with_host#*/}"
  if [[ -z "$host" || -z "$repo" || "$repo" == "$repo_with_host" ]]; then
    return 1
  fi

  scheme="$(registry_scheme_for_host "$host")"
  url="${scheme}://${host}/v2/${repo}/manifests/${tag}"
  code="$(
    curl --noproxy '*' -sS -o /dev/null -w '%{http_code}' \
      -H 'Accept: application/vnd.oci.image.manifest.v1+json,application/vnd.oci.image.index.v1+json,application/vnd.docker.distribution.manifest.v2+json,application/vnd.docker.distribution.manifest.list.v2+json' \
      "$url" || true
  )"

  if [[ "$code" == "200" ]]; then
    return 0
  fi

  # Some registries may block unauthenticated manifest requests.
  if [[ "$code" == "401" || "$code" == "403" ]]; then
    if docker pull "$ref" >/dev/null 2>&1; then
      return 0
    fi
  fi

  return 1
}

should_push() {
  local ref="$1"
  if (( FORCE )); then
    return 0
  fi
  if image_exists "$ref"; then
    log "SKIP exists: $ref"
    return 1
  fi
  return 0
}

discover_mig_dirs() {
  local root="deploy/images/migs"
  [[ -d "$root" ]] || return 0
  find "$root" -mindepth 1 -maxdepth 1 -type d -print | while read -r d; do basename "$d"; done | sort
}

mig_repo_name() {
  local dir="$1"
  case "$dir" in
    orw-maven) echo "migs-orw-maven" ;;
    orw-gradle) echo "migs-orw-gradle" ;;
    mig-*) echo "migs-${dir#mig-}" ;;
    *) echo "$dir" ;;
  esac
}

build_push_mig_image() {
  local dir="$1"
  local image_name ref context
  image_name="$(mig_repo_name "$dir")"
  ref="${IMAGE_PREFIX}/${image_name}:latest"

  if ! should_push "$ref"; then
    return 0
  fi

  if [[ "$dir" == "mig-codex" ]]; then
    context="."
    run_with_retries \
      "buildx push ${ref} (context=${context}, dockerfile=deploy/images/migs/mig-codex/Dockerfile)" \
      docker buildx build \
      --platform "$PLATFORM" \
      --provenance=false --sbom=false --pull --progress=plain \
      -f deploy/images/migs/mig-codex/Dockerfile \
      -t "$ref" \
      --push \
      "$context"
  else
    context="deploy/images/migs/${dir}"
    run_with_retries \
      "buildx push ${ref} (context=${context})" \
      docker buildx build \
      --platform "$PLATFORM" \
      --provenance=false --sbom=false --pull --progress=plain \
      -t "$ref" \
      --push \
      "$context"
  fi
}

build_push_gate_gradle_image() {
  local jdk="$1"
  local ref="${IMAGE_PREFIX}/ploy-gate-gradle:jdk${jdk}"
  local dockerfile="deploy/images/gates/gradle/Dockerfile.jdk${jdk}"

  if ! should_push "$ref"; then
    return 0
  fi

  run_with_retries \
    "buildx push ${ref} (dockerfile=${dockerfile})" \
    docker buildx build \
    --platform "$PLATFORM" \
    --provenance=false --sbom=false --pull --progress=plain \
    -f "$dockerfile" \
    -t "$ref" \
    --push \
    deploy/images/gates/gradle
}

mirror_image_if_missing() {
  local source_ref="$1"
  local target_ref="$2"

  if ! should_push "$target_ref"; then
    return 0
  fi

  run_with_retries "pull ${source_ref}" docker pull "$source_ref"
  run_with_retries "tag ${source_ref} -> ${target_ref}" docker tag "$source_ref" "$target_ref"
  run_with_retries "push ${target_ref}" docker push "$target_ref"
}

main() {
  parse_args "$@"

  need docker
  need curl
  if ! docker buildx version >/dev/null 2>&1; then
    echo "error: docker buildx not available (install docker buildx plugin)" >&2
    exit 2
  fi

  local root
  root="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
  cd "$root"

  log "Target image prefix: ${IMAGE_PREFIX}"
  if (( FORCE )); then
    log "Force mode enabled: existing registry images will be rebuilt/re-pushed"
  fi

  log "Syncing mig images..."
  local mig_dirs_raw
  mig_dirs_raw="$(discover_mig_dirs)"
  if [[ -z "$mig_dirs_raw" ]]; then
    echo "error: no mig image directories found under deploy/images/migs" >&2
    exit 1
  fi
  local d
  for d in $mig_dirs_raw; do
    build_push_mig_image "$d"
  done

  log "Syncing build-gate gradle images..."
  build_push_gate_gradle_image 11
  build_push_gate_gradle_image 17

  log "Syncing mirrored upstream build-gate base images..."
  mirror_image_if_missing "maven:3-eclipse-temurin-11" "${IMAGE_PREFIX}/maven:3-eclipse-temurin-11"
  mirror_image_if_missing "maven:3-eclipse-temurin-17" "${IMAGE_PREFIX}/maven:3-eclipse-temurin-17"
  mirror_image_if_missing "golang:1.22" "${IMAGE_PREFIX}/golang:1.22"
  mirror_image_if_missing "rust:1.76" "${IMAGE_PREFIX}/rust:1.76"

  log "Garage image sync complete"
}

main "$@"
