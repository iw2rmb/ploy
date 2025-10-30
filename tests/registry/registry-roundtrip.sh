#!/usr/bin/env bash
set -euo pipefail

# E2E: Build a tiny OCI image (config + 1 layer), push via CLI to Ploy registry,
# fetch it back, and delete it (tag, manifest, blobs).

if ! command -v jq >/dev/null 2>&1; then
  echo "jq required" >&2; exit 1
fi

repo=${PLOY_E2E_REGISTRY_REPO:-e2e/mods-sample}
tag=${PLOY_E2E_TAG:-e2e-$(date -u +%Y%m%d%H%M%S)}
workdir=$(mktemp -d)
cleanup() { rm -rf "$workdir"; }
trap cleanup EXIT

mkdir -p "$workdir"

# Create a tiny layer tar.gz
mkdir -p "$workdir/layer"
echo "hello from e2e" > "$workdir/layer/hello.txt"
tar -C "$workdir/layer" -czf "$workdir/layer.tar.gz" .

# Config JSON (minimal)
printf '{}' > "$workdir/config.json"

sha256() { sha256sum "$1" 2>/dev/null | awk '{print $1}'; }
if ! command -v sha256sum >/dev/null 2>&1; then
  sha256() { shasum -a 256 "$1" | awk '{print $1}'; }
fi

size_of() { stat -f%z "$1" 2>/dev/null || stat -c%s "$1"; }

layer_size=$(size_of "$workdir/layer.tar.gz")
config_size=$(size_of "$workdir/config.json")
layer_digest="sha256:$(sha256 "$workdir/layer.tar.gz")"
config_digest="sha256:$(sha256 "$workdir/config.json")"

# Push blobs via CLI
DigestLayer=$(dist/ploy registry push-blob --repo "$repo" --media-type application/vnd.oci.image.layer.v1.tar+gzip "$workdir/layer.tar.gz" | awk '/^Digest:/ {print $2}')
DigestConfig=$(dist/ploy registry push-blob --repo "$repo" --media-type application/vnd.oci.image.config.v1+json "$workdir/config.json" | awk '/^Digest:/ {print $2}')

test "$DigestLayer" = "$layer_digest" || { echo "layer digest mismatch" >&2; exit 2; }
test "$DigestConfig" = "$config_digest" || { echo "config digest mismatch" >&2; exit 2; }

# Create manifest referencing the blobs
cat > "$workdir/manifest.json" << JSON
{
  "schemaVersion": 2,
  "mediaType": "application/vnd.oci.image.manifest.v1+json",
  "config": { "mediaType": "application/vnd.oci.image.config.v1+json", "digest": "$config_digest", "size": $config_size },
  "layers": [
    { "mediaType": "application/vnd.oci.image.layer.v1.tar+gzip", "digest": "$layer_digest", "size": $layer_size }
  ]
}
JSON

Manifest=$(dist/ploy registry put-manifest --repo "$repo" --reference "$tag" "$workdir/manifest.json" | awk '/^Manifest:/ {print $2}')
test -n "$Manifest" || { echo "missing manifest digest" >&2; exit 3; }

# Verify presence
dist/ploy registry get-manifest --repo "$repo" --reference "$tag" --output "$workdir/manifest.out.json"
diff -u <(jq -S . "$workdir/manifest.json") <(jq -S . "$workdir/manifest.out.json")

# Download layer back and verify digest
dist/ploy registry get-blob --repo "$repo" --digest "$layer_digest" --output "$workdir/layer.pull.tgz"
GotLayer="sha256:$(sha256 "$workdir/layer.pull.tgz")"
test "$GotLayer" = "$layer_digest" || { echo "roundtrip digest mismatch" >&2; exit 4; }

# Delete tag, manifest, blobs
dist/ploy registry rm-manifest --repo "$repo" --reference "$tag"
dist/ploy registry rm-manifest --repo "$repo" --reference "$Manifest"
dist/ploy registry rm-blob --repo "$repo" --digest "$config_digest"
dist/ploy registry rm-blob --repo "$repo" --digest "$layer_digest"

echo "OK: registry roundtrip ($repo:$tag)"

