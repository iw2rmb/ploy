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
  local root_orw="deploy/images/orw"
  local root_shell="deploy/images/shell"
  local root_codex="deploy/images/codex"
  local root_amata="deploy/images/amata"
  {
    if [[ -d "$root_migs" ]]; then
      find "$root_migs" -mindepth 1 -maxdepth 1 -type d -print | while read -r d; do
        name="$(basename "$d")"
        printf 'migs/%s\n' "$name"
      done
    fi

    if [[ -d "$root_orw" ]]; then
      find "$root_orw" -mindepth 1 -maxdepth 1 -type d -print | while read -r d; do
        name="$(basename "$d")"
        printf 'orw/%s\n' "$name"
      done
    fi

    if [[ -d "$root_shell" ]]; then
      printf 'shell/shell\n'
    fi

    if [[ -d "$root_codex" ]]; then
      printf 'codex/codex\n'
    fi

    if [[ -d "$root_amata" ]]; then
      printf 'amata/amata\n'
    fi
  } | sort
}

images=( $(discover_images) )
if [[ ${#images[@]} -eq 0 ]]; then
  echo "no images discovered under deploy/images/{migs,orw,codex,amata,shell}" >&2
  exit 1
fi

for entry in "${images[@]}"; do
  name="${entry##*/}"
  source_group="${entry%%/*}"

  # Map directory name to repo image name.
  image_name="$name"
  case "$name" in
    mig-*)
      image_name="migs-${name#mig-}"
      ;;
    shell)
      image_name="migs-shell"
      ;;
    codex)
      image_name="migs-codex"
      ;;
    amata)
      image_name="migs-amata"
      ;;
  esac

  ref="${IMAGE_PREFIX}/${image_name}:latest"

  # Build context rules:
  # - Default: use deploy/images/<group>/<dir>
  # - Special-case codex/amata: Dockerfile expects repo-root context.
  # - Special-case orw-cli-gradle/orw-cli-maven: Dockerfile expects repo-root context (shared runner src)
  build_args=("docker" "buildx" "build" "--platform" "$PLATFORM" "--provenance=false" "--sbom=false" "--pull" "--progress=plain" "-t" "$ref" "--push")
  if [[ "$source_group" == "codex" && "$name" == "codex" ]]; then
    context="."
    build_args+=("-f" "deploy/images/codex/Dockerfile" "$context")
  elif [[ "$source_group" == "amata" && "$name" == "amata" ]]; then
    bash deploy/images/amata/build-amata.sh
    context="."
    build_args+=("-f" "deploy/images/amata/Dockerfile" "$context")
  elif [[ "$source_group" == "orw" && ( "$name" == "orw-cli-gradle" || "$name" == "orw-cli-maven" ) ]]; then
    context="."
    build_args+=("-f" "deploy/images/orw/${name}/Dockerfile" "$context")
  elif [[ "$source_group" == "shell" && "$name" == "shell" ]]; then
    context="deploy/images/shell"
    build_args+=("-f" "deploy/images/shell/Dockerfile" "$context")
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
