#!/usr/bin/env bash
set -euo pipefail

# Fetch builder logs via API and full logs via SeaweedFS (SSH) for a given BUILD_ID

: "${PLOY_CONTROLLER:?PLOY_CONTROLLER must be set}"
: "${APP_NAME:?APP_NAME must be set}"
: "${BUILD_ID:?BUILD_ID must be set}"
: "${TARGET_HOST:?TARGET_HOST must be set for SSH-based SeaweedFS fetch}"

LINES=${LINES:-800}
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUT_DIR="$ROOT_DIR/logs/$BUILD_ID"
mkdir -p "$OUT_DIR"

echo "Fetching API builder logs for id=$BUILD_ID (lines=$LINES)"
curl -sS "${PLOY_CONTROLLER%/}/apps/${APP_NAME}/builds/${BUILD_ID}/logs?lines=${LINES}" \
  | tee "$OUT_DIR/builder.logs.json" >/dev/null || true

# Attempt to resolve logs_url from API response or derive from deployment id
LOGS_URL=$(jq -r 'try .logs_url // .builder.logs_url // empty' "$OUT_DIR/builder.logs.json" 2>/dev/null || echo "")
if [[ -z "$LOGS_URL" ]]; then
  LOGS_URL="http://seaweedfs-filer.storage.ploy.local:8888/artifacts/build-logs/${BUILD_ID}.log"
fi

echo "Fetching full builder log via SSH: $LOGS_URL"
ssh -o ConnectTimeout=10 "root@$TARGET_HOST" "curl -fsS '$LOGS_URL'" > "$OUT_DIR/builder.full.log" 2>/dev/null || true

BYTES=0; [[ -s "$OUT_DIR/builder.full.log" ]] && BYTES=$(wc -c < "$OUT_DIR/builder.full.log" | tr -d ' ')
echo "Full log bytes: $BYTES"

