#!/usr/bin/env bash
set -Eeuo pipefail

# Build and push Migs Docker images to an OCI registry.
# Requires: docker buildx plugin.

PLATFORM=${PLATFORM:-linux/amd64}
PUSH_TIMEOUT=${PUSH_TIMEOUT:-900}        # seconds (default 15m)
PUSH_RETRIES=${PUSH_RETRIES:-1}          # number of retries on failure
IMAGE_PREFIX="${IMAGE_PREFIX:-${PLOY_CONTAINER_REGISTRY:-127.0.0.1:5000/ploy}}"

if ! command -v docker >/dev/null 2>&1; then
  echo "error: docker CLI not found" >&2
  exit 2
fi
if ! docker buildx version >/dev/null 2>&1; then
  echo "error: docker buildx not available (install docker buildx plugin)" >&2
  exit 2
fi

workdir=$(mktemp -d)
trap 'rm -rf "$workdir"' EXIT

with_timeout() {
  # usage: with_timeout <seconds> <command...>
  local secs=$1; shift
  if command -v timeout >/dev/null 2>&1; then
    timeout "${secs}s" "$@"
  elif command -v gtimeout >/dev/null 2>&1; then
    gtimeout "${secs}s" "$@"
  else
    # portable fallback using perl alarm; works on macOS/Linux
    perl -e 'alarm shift @ARGV; exec @ARGV' "$secs" "$@"
  fi
}

discover_images() {
  local root_migs="deploy/images/migs"
  local root_mig="deploy/images/mig"
  {
    if [[ -d "$root_migs" ]]; then
      find "$root_migs" -mindepth 1 -maxdepth 1 -type d -print | while read -r d; do
        name="$(basename "$d")"
        printf 'migs/%s\n' "$name"
      done
    fi

    if [[ -d "$root_mig" ]]; then
      find "$root_mig" -mindepth 1 -maxdepth 1 -type d -print | while read -r d; do
        name="$(basename "$d")"
        printf 'mig/%s\n' "$name"
      done
    fi
  } | sort
}

images=( $(discover_images) )
if [[ ${#images[@]} -eq 0 ]]; then
  echo "no images discovered under deploy/images/migs or deploy/images/mig" >&2
  exit 1
fi

for entry in "${images[@]}"; do
  name="${entry##*/}"
  source_group="${entry%%/*}"

  # Map directory name to repo image name.
  image_name="$name"
  case "$name" in
    orw-cli)
      image_name="orw-cli"
      ;;
    mig-*)
      image_name="migs-${name#mig-}"
      ;;
  esac

  ref="${IMAGE_PREFIX}/${image_name}:latest"

  # Build context rules:
  # - Default: use deploy/images/<group>/<dir>
  # - Special-case mig-codex: Dockerfile expects repo-root context (COPY go.* and internal/ ...)
  build_args=("docker" "buildx" "build" "--platform" "$PLATFORM" "--provenance=false" "--sbom=false" "--pull" "--progress=plain" "-t" "$ref" "--push")
  if [[ "$source_group" == "migs" && "$name" == "mig-codex" ]]; then
    context="."
    build_args+=("-f" "deploy/images/migs/mig-codex/Dockerfile" "$context")
  else
    context="deploy/images/${source_group}/${name}"
    build_args+=("$context")
  fi

  echo "==> Building and pushing ${ref} (context: ${context})"
  attempt=0
  while :; do
    attempt=$((attempt+1))
    echo "[attempt ${attempt}/${PUSH_RETRIES}] ${build_args[*]}"
    if with_timeout "$PUSH_TIMEOUT" "${build_args[@]}"; then
      echo "OK: ${ref}"
      break
    fi
    if (( attempt >= PUSH_RETRIES )); then
      echo "error: push failed for ${ref} after ${attempt} attempt(s) within ${PUSH_TIMEOUT}s timeout" >&2
      exit 1
    fi
    echo "warn: push failed for ${ref}; retrying..." >&2
    sleep 3
  done
done

echo "All Migs images pushed to ${IMAGE_PREFIX}"
