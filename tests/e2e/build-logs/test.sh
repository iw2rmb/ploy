#!/usr/bin/env bash
set -euo pipefail

# One-shot E2E: submit → fetch → summarize

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
APP_NAME=${APP_NAME:-kaniko-fail-app}
PLOY_CONTROLLER=${PLOY_CONTROLLER:-}
TARGET_HOST=${TARGET_HOST:-}

if [[ -z "$PLOY_CONTROLLER" ]]; then
  echo "PLOY_CONTROLLER must be set" >&2
  exit 2
fi
if [[ -z "$TARGET_HOST" ]]; then
  echo "TARGET_HOST must be set" >&2
  exit 2
fi

echo "== Submitting build (Lane E, build_only)"
APP_NAME="$APP_NAME" PLOY_CONTROLLER="$PLOY_CONTROLLER" "$ROOT_DIR/run.sh"

# Resolve last run id
ID=$(ls -1t "$ROOT_DIR/logs" | head -n1)
if [[ -z "$ID" || "$ID" == submission-* ]]; then
  # read from last submission headers if id-specific dir wasn’t created
  if [[ -f "$ROOT_DIR/logs/$ID/headers.txt" ]]; then
    ID=$(grep -i '^x-deployment-id:' "$ROOT_DIR/logs/$ID/headers.txt" | tail -n1 | \
      cut -d':' -f2- | tr -d '\r' | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')
  fi
fi
if [[ -z "$ID" ]]; then
  echo "Unable to resolve BUILD_ID from last submission" >&2
  exit 3
fi
echo "BUILD_ID: $ID"

echo "== Fetching logs"
APP_NAME="$APP_NAME" BUILD_ID="$ID" TARGET_HOST="$TARGET_HOST" PLOY_CONTROLLER="$PLOY_CONTROLLER" "$ROOT_DIR/fetch-logs.sh"

echo "== Summarizing"
APP_NAME="$APP_NAME" BUILD_ID="$ID" "$ROOT_DIR/summarize.sh"

echo "Done. See: $ROOT_DIR/logs/$ID"
