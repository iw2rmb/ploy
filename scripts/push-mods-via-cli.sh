#!/usr/bin/env bash
set -euo pipefail

#!/usr/bin/env bash
set -euo pipefail

# Build and push Mods Docker images to Docker Hub.
# Requires: docker buildx

PLATFORM=${PLATFORM:-linux/amd64}
DOCKERHUB_USERNAME=${DOCKERHUB_USERNAME:-}
IMAGE_PREFIX=${MODS_IMAGE_PREFIX:-}

if [[ -z "$DOCKERHUB_USERNAME" && -z "$IMAGE_PREFIX" ]]; then
  echo "error: DOCKERHUB_USERNAME or MODS_IMAGE_PREFIX must be set" >&2
  echo "hint: export DOCKERHUB_USERNAME in your shell (e.g., via ~/.zshenv)" >&2
  exit 2
fi

if [[ -n "${DOCKERHUB_PAT:-}" && -n "$DOCKERHUB_USERNAME" ]]; then
  echo "Logging in to Docker Hub as $DOCKERHUB_USERNAME"
  if ! printf '%s' "$DOCKERHUB_PAT" | docker login -u "$DOCKERHUB_USERNAME" --password-stdin >/dev/null 2>&1; then
    echo "warning: docker login failed; continuing (images may be public)" >&2
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

images=( $(discover_images) )
if [[ ${#images[@]} -eq 0 ]]; then
  echo "no images discovered under docker/mods" >&2
  exit 1
fi

for name in "${images[@]}"; do
  context="docker/mods/${name}"
  # Map directory name to repo image name
  image_name="$name"
  if [[ "$image_name" == mod-* ]]; then
    image_name="mods-${image_name#mod-}"
  fi
  if [[ "$name" == "mod-orw" ]]; then
    image_name="mods-openrewrite"
  fi
  ref="${IMAGE_PREFIX}/${image_name}:latest"
  echo "==> Building and pushing ${ref}"
  docker buildx build --platform "$PLATFORM" -t "$ref" --push "$context"
  echo "OK: ${ref}"
done

echo "All Mods images pushed to ${IMAGE_PREFIX}"
