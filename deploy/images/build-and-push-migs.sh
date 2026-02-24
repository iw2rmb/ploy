#!/usr/bin/env bash
set -Eeuo pipefail

# Build and push Migs Docker images to Docker Hub.
# Requires: docker buildx

PLATFORM=${PLATFORM:-linux/amd64}
PUSH_TIMEOUT=${PUSH_TIMEOUT:-900}        # seconds (default 15m)
PUSH_RETRIES=${PUSH_RETRIES:-1}          # number of retries on failure
IMAGE_PREFIX="ghcr.io/iw2rmb"

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
  local root="deploy/images/migs"
  [[ -d "$root" ]] || return 0
  find "$root" -mindepth 1 -maxdepth 1 -type d -print | while read -r d; do basename "$d"; done | sort
}

images=( $(discover_images) )
if [[ ${#images[@]} -eq 0 ]]; then
  echo "no images discovered under migs" >&2
  exit 1
fi

for name in "${images[@]}"; do
  # Map directory name to repo image name.
  image_name="$name"
  case "$name" in
    orw-maven)
      image_name="migs-orw-maven"
      ;;
    orw-gradle)
      image_name="migs-orw-gradle"
      ;;
    mig-*)
      image_name="migs-${name#mig-}"
      ;;
  esac

  ref="${IMAGE_PREFIX}/${image_name}:latest"

  # Build context rules:
  # - Default: use "deploy/images/migs/<dir>"
  # - Special-case mig-codex: Dockerfile expects repo-root context (COPY go.mod, internal/ ...)
  build_args=("docker" "buildx" "build" "--platform" "$PLATFORM" "--provenance=false" "--sbom=false" "--pull" "-t" "$ref" "--push")
  if [[ "$name" == "mig-codex" ]]; then
    context="."
    build_args+=("-f" "deploy/images/migs/mig-codex/Dockerfile" "$context")
  else
    context="deploy/images/migs/${name}"
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

echo "All Migs images pushed to ${IMAGE_PREFIX}"
