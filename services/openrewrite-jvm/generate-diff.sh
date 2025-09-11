#!/usr/bin/env bash
set -euo pipefail

# Usage: generate-diff.sh <before_dir> <after_dir> <output_patch>
if [ "$#" -lt 3 ]; then
  echo "usage: $0 <before_dir> <after_dir> <output_patch>" >&2
  exit 2
fi

BEFORE_DIR="$1"
AFTER_DIR="$2"
OUT_PATCH="$3"

mkdir -p "$(dirname "$OUT_PATCH")"

# Create temp directories to stage trees with consistent prefixes for unified diff
TMP_BEFORE=$(mktemp -d 2>/dev/null || mktemp -d -t diffbefore)
TMP_AFTER=$(mktemp -d 2>/dev/null || mktemp -d -t diffafter)
trap 'rm -rf "$TMP_BEFORE" "$TMP_AFTER"' EXIT

# Copy trees (best-effort) excluding build caches
rsync -a --delete \
  --exclude '.m2' \
  --exclude 'target' \
  --exclude '.gradle' \
  --exclude 'build' \
  --exclude '.git' \
  "$BEFORE_DIR"/ "$TMP_BEFORE"/ 2>/dev/null || true
rsync -a --delete \
  --exclude '.m2' \
  --exclude 'target' \
  --exclude '.gradle' \
  --exclude 'build' \
  --exclude '.git' \
  "$AFTER_DIR"/ "$TMP_AFTER"/ 2>/dev/null || true

## Generate unified diff reliably using diff(1)
diff -ruN "$TMP_BEFORE" "$TMP_AFTER" > "$OUT_PATCH" 2>/dev/null || true

# Ensure file exists even if empty
touch "$OUT_PATCH"

exit 0
