#!/usr/bin/env bash
set -Eeuo pipefail

# Build and push Mods Docker images to Docker Hub.
# Requires: docker buildx

PLATFORM=${PLATFORM:-linux/amd64}
PUSH_TIMEOUT=${PUSH_TIMEOUT:-900}        # seconds (default 15m)
PUSH_RETRIES=${PUSH_RETRIES:-1}          # number of retries on failure
DOCKERHUB_USERNAME=${DOCKERHUB_USERNAME:-}
IMAGE_PREFIX=${MODS_IMAGE_PREFIX:-}

if [[ -z "$DOCKERHUB_USERNAME" && -z "$IMAGE_PREFIX" ]]; then
  echo "error: DOCKERHUB_USERNAME or MODS_IMAGE_PREFIX must be set" >&2
  echo "hint: export DOCKERHUB_USERNAME in your shell (e.g., via ~/.zshenv)" >&2
  exit 2
fi

if ! command -v docker >/dev/null 2>&1; then
  echo "error: docker CLI not found" >&2; exit 2
fi
if ! docker buildx version >/dev/null 2>&1; then
  echo "error: docker buildx not available (install docker buildx plugin)" >&2; exit 2
fi

if [[ -n "${DOCKERHUB_PAT:-}" && -n "$DOCKERHUB_USERNAME" ]]; then
  echo "Logging in to Docker Hub as $DOCKERHUB_USERNAME"
  if ! printf '%s' "$DOCKERHUB_PAT" | docker login -u "$DOCKERHUB_USERNAME" --password-stdin >/dev/null 2>&1; then
    echo "error: docker login failed; aborting push" >&2
    exit 2
  fi
fi

if [[ -z "$IMAGE_PREFIX" ]]; then
  IMAGE_PREFIX="docker.io/${DOCKERHUB_USERNAME}"
fi

discover_images() {
  local root="docker/mods"
  [[ -d "$root" ]] || return 0
  find "$root" -mindepth 1 -maxdepth 1 -type d -print | while read -r d; do basename "$d"; done | sort
}

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
  local root="docker/mods"
  [[ -d "$root" ]] || return 0
  find "$root" -mindepth 1 -maxdepth 1 -type d -print | while read -r d; do basename "$d"; done | sort
}

images=( $(discover_images) )
if [[ ${#images[@]} -eq 0 ]]; then
  echo "no images discovered under mods" >&2
  exit 1
fi

for name in "${images[@]}"; do
  # Skip deprecated mod-orw; use orw-maven/orw-gradle instead.
  if [[ "$name" == "mod-orw" ]]; then
    echo "==> Skipping deprecated mod-orw (use orw-maven/orw-gradle)"
    continue
  fi

  # Map directory name to repo image name.
  image_name="$name"
  case "$name" in
    orw-maven)
      image_name="mods-orw-maven"
      ;;
    orw-gradle)
      image_name="mods-orw-gradle"
      ;;
    mod-*)
      image_name="mods-${name#mod-}"
      ;;
  esac

  ref="${IMAGE_PREFIX}/${image_name}:latest"

  # Build context rules:
  # - Default: use "docker/mods/<dir>"
  # - Special-case mod-codex: Dockerfile expects repo-root context (COPY go.mod, internal/ ...)
  build_args=("docker" "buildx" "build" "--platform" "$PLATFORM" "--provenance=false" "--sbom=false" "--pull" "-t" "$ref" "--push")
  if [[ "$name" == "mod-codex" ]]; then
    context="."
    build_args+=("-f" "docker/mods/mod-codex/Dockerfile" "$context")
  else
    context="docker/mods/${name}"
    build_args+=("$context")
  fi

  echo "==> Building and pushing ${ref} (context: ${context})"
  attempt=0
  while :; do
    attempt=$((attempt+1))
    echo "[attempt ${attempt}/${PUSH_RETRIES}] ${build_args[*]}"
    if with_timeout "$PUSH_TIMEOUT" "${build_args[@]}" --progress=plain; then
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

echo "All Mods images pushed to ${IMAGE_PREFIX}"
