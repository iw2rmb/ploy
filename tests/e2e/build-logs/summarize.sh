#!/usr/bin/env bash
set -euo pipefail

# Creates a summary.txt for the given BUILD_ID, reporting API tail and full log pointer.

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
APP_NAME=${APP_NAME:-docker-fail-app}
: "${BUILD_ID:?BUILD_ID must be set}"

OUT_DIR="$ROOT_DIR/logs/$BUILD_ID"
SUMMARY="$OUT_DIR/summary.txt"

API_TAIL_FILE="$OUT_DIR/builder.logs.json"
FULL_LOG_FILE="$OUT_DIR/builder.full.log"
LOGS_URL=$(jq -r 'try .logs_url // .builder.logs_url // empty' "$API_TAIL_FILE" 2>/dev/null || echo "")
if [[ -z "$LOGS_URL" ]]; then
  LOGS_URL="http://seaweedfs-filer.storage.ploy.local:8888/artifacts/build-logs/${BUILD_ID}.log"
fi

BYTES=0; [[ -s "$FULL_LOG_FILE" ]] && BYTES=$(wc -c < "$FULL_LOG_FILE" | tr -d ' ')

{
  echo "Builder Logs E2E Summary"
  echo "APP_NAME: $APP_NAME"
  echo "BUILD_ID: $BUILD_ID"
  echo "logs_url: $LOGS_URL"
  echo "builder_full_log: $(basename "$FULL_LOG_FILE") bytes=$BYTES"
  echo
  echo "API logs tail (first 20 lines of .logs):"
  jq -r 'try .logs // empty' "$API_TAIL_FILE" 2>/dev/null | sed -n '1,20p' || echo "<none>"
  echo
  echo "Full logs tail (last 40 lines):"
  if [[ -s "$FULL_LOG_FILE" ]]; then
    tail -n 40 "$FULL_LOG_FILE"
  else
    echo "<none>"
  fi
} > "$SUMMARY"

echo "Wrote summary: $SUMMARY"
