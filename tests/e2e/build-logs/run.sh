#!/usr/bin/env bash
set -euo pipefail

# Submits a Lane E build-only tar for the sample app and saves response and headers.

: "${PLOY_CONTROLLER:?PLOY_CONTROLLER must be set (e.g., https://api.dev.ployman.app/v1)}"
APP_NAME=${APP_NAME:-kaniko-fail-app}
SHA=${SHA:-$(date +%s)}
LINES=${LINES:-800}
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUT_DIR="$ROOT_DIR/logs"
mkdir -p "$OUT_DIR"

TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

# Tar the app directory
tar -cf "$TMP_DIR/src.tar" -C "$ROOT_DIR/app" .

# Submit build (Lane E, build_only=true)
BUILD_URL="${PLOY_CONTROLLER%/}/apps/${APP_NAME}/builds?sha=${SHA}&lane=E&env=dev&build_only=true"
RUN_DIR="$OUT_DIR/submission-$(date +%Y%m%d-%H%M%S)"
mkdir -p "$RUN_DIR"

curl -sS -D "$RUN_DIR/headers.txt" -o "$RUN_DIR/response.json" \
  -H 'Content-Type: application/x-tar' \
  --data-binary @"$TMP_DIR/src.tar" \
  "$BUILD_URL" || true

# Extract X-Deployment-ID (if any, header casing may vary)
BUILD_ID=$(grep -i '^x-deployment-id:' "$RUN_DIR/headers.txt" | tail -n1 | \
  cut -d':' -f2- | tr -d '\r' | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')

echo "Submission complete"
echo "  APP_NAME: $APP_NAME"
echo "  SHA:      $SHA"
echo "  BUILD_ID: ${BUILD_ID:-<none>} (see $RUN_DIR)"

# If we have an id, create an id-specific folder and copy artifacts
if [[ -n "$BUILD_ID" ]]; then
  ID_DIR="$OUT_DIR/$BUILD_ID"
  mkdir -p "$ID_DIR"
  cp "$RUN_DIR/headers.txt" "$ID_DIR/headers.txt"
  cp "$RUN_DIR/response.json" "$ID_DIR/response.json"
  echo "$BUILD_ID" > "$ID_DIR/build_id.txt"
  echo "BUILD_ID saved under $ID_DIR"
else
  echo "No X-Deployment-ID header found (the build may have failed before acceptance)."
fi
