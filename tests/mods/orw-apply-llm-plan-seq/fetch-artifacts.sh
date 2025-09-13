#!/usr/bin/env bash
set -euo pipefail

# Usage: ./fetch-artifacts.sh <EXEC_ID>
# Downloads known artifacts (plan_json, next_json, diff_patch) into logs/<EXEC_ID>/

if [[ $# -lt 1 ]]; then
  echo "Usage: $0 <EXEC_ID>" >&2
  exit 1
fi

EXEC_ID="$1"
API_BASE="${PLOY_CONTROLLER:-}"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LOG_DIR="$ROOT_DIR/logs/$EXEC_ID"

mkdir -p "$LOG_DIR"

if [[ -z "$API_BASE" ]]; then
  echo "PLOY_CONTROLLER must be set (e.g., https://api.dev.ployman.app/v1)" >&2
  exit 1
fi

ARTS_JSON=$(curl -sS "$API_BASE/mods/$EXEC_ID/artifacts" || true)
echo "$ARTS_JSON" > "$LOG_DIR/artifacts_index.json"

download() {
  local name="$1"
  local out="$LOG_DIR/$2"
  echo "Fetching $name → $out"
  if [[ -z "$2" ]]; then
    echo "  (skip: no output filename provided)"; return 0; fi
  curl -fsS "$API_BASE/mods/$EXEC_ID/artifacts/$name" -o "$out" || {
    echo "  (missing $name)"; return 0; }
}

download plan_json plan.json
download next_json next.json
download diff_patch diff.patch

echo "Artifacts written to $LOG_DIR"
