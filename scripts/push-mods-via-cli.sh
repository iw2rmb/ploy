#!/usr/bin/env bash
set -euo pipefail

# Build Mods Docker contexts to OCI layouts then push to the Ploy registry via CLI.
# Requires: docker buildx, jq, tar

REPO_PREFIX=${PLOY_E2E_MODS_REPO_PREFIX:-ploy}
PLATFORM=${PLATFORM:-linux/amd64}

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
  # Map directory name to registry repo name for backward compatibility:
  # mod-foo (dir) -> mods-foo (registry repo)
  repo_name="$name"
  if [[ "$repo_name" == mod-* ]]; then
    repo_name="mods-${repo_name#mod-}"
  fi
  echo "==> Building $name as OCI layout"
  out_tar="$workdir/${name}.oci.tar"
  docker buildx build --platform "$PLATFORM" --output type=oci,dest="$out_tar" "$context"
  out_dir="$workdir/${name}.oci"
  mkdir -p "$out_dir"
  tar -C "$out_dir" -xf "$out_tar"

  manifest_digest="sha256:$(jq -r '.manifests[0].digest' "$out_dir/index.json" | sed 's/^sha256://')"
  mhex=${manifest_digest#sha256:}
  manifest_path="$out_dir/blobs/sha256/${mhex}"
  if [[ ! -s "$manifest_path" ]]; then
    echo "error: manifest blob not found for $name" >&2; exit 2
  fi

  mf="$manifest_path"
  # push config
  cfg_d=$(jq -r '.config.digest' "$mf")
  cfg_media=$(jq -r '.config.mediaType' "$mf")
  cfg_path="$out_dir/blobs/sha256/${cfg_d#sha256:}"
  echo "--> Pushing config $cfg_d ($cfg_media)"
  dist/ploy registry push-blob --repo "${REPO_PREFIX}/${repo_name}" --media-type "$cfg_media" "$cfg_path" >/dev/null

  # push layers
  lcount=$(jq '.layers|length' "$mf")
  for ((i=0; i<lcount; i++)); do
    l_d=$(jq -r ".layers[$i].digest" "$mf")
    l_media=$(jq -r ".layers[$i].mediaType" "$mf")
    l_path="$out_dir/blobs/sha256/${l_d#sha256:}"
    echo "--> Pushing layer $((i+1))/$lcount $l_d ($l_media)"
    dist/ploy registry push-blob --repo "${REPO_PREFIX}/${repo_name}" --media-type "$l_media" "$l_path" >/dev/null
  done

  echo "--> Putting manifest as :latest"
  dist/ploy registry put-manifest --repo "${REPO_PREFIX}/${repo_name}" --reference latest "$mf" >/dev/null
  echo "OK: ${REPO_PREFIX}/${repo_name}:latest"
done

echo "All mods images pushed via CLI"
