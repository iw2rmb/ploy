#!/usr/bin/env bash
set -euo pipefail

# Usage: ./check-steps.sh <EXEC_ID>
# Validates that critical steps occurred in the expected order for the scenario.

if [[ $# -lt 1 ]]; then
  echo "Usage: $0 <EXEC_ID>" >&2
  exit 1
fi

EXEC_ID="$1"
API_BASE="${PLOY_CONTROLLER:-}"
if [[ -z "$API_BASE" ]]; then
  echo "PLOY_CONTROLLER must be set (e.g., https://api.dev.ployman.app/v1)" >&2
  exit 1
fi

ST_JSON=$(curl -sS "$API_BASE/mods/$EXEC_ID/status")

req() {
  local field="$1"; shift
  local expect="$1"; shift
  if ! echo "$ST_JSON" | jq -e "$field | tostring | test($expect)" >/dev/null 2>&1; then
    echo "Missing expected pattern: $field ~= $expect" >&2
    exit 2
  fi
}

# Ensure step records exist
COUNT=$(echo "$ST_JSON" | jq '.steps | length')
if [[ "$COUNT" -eq 0 ]]; then
  echo "No steps recorded for $EXEC_ID" >&2
  exit 3
fi

# Look for key steps (best-effort regex over concatenated messages)
ALL=$(echo "$ST_JSON" | jq -r '.steps[] | "\(.phase):\(.step):\(.level):\(.message)"' | tr '\n' ' ')
echo "Steps (flattened): $ALL"

grep -Eq 'apply:diff-found:info' <<<"$ALL" || { echo "missing diff-found" >&2; exit 4; }
grep -Eq 'build:build-gate-start:info' <<<"$ALL" || { echo "missing build-gate-start" >&2; exit 4; }
grep -Eq 'build:build-gate-failed:error' <<<"$ALL" || { echo "missing build-gate-failed" >&2; exit 4; }
grep -Eq 'planner:planner:info:job started' <<<"$ALL" || { echo "missing planner start" >&2; exit 4; }
grep -Eq 'planner:planner:info:job completed' <<<"$ALL" || { echo "missing planner completed" >&2; exit 4; }
grep -Eq 'llm-exec:llm-exec:info:job started' <<<"$ALL" || { echo "missing llm-exec start" >&2; exit 4; }
grep -Eq 'llm-exec:llm-exec:info:job completed' <<<"$ALL" || { echo "missing llm-exec completed" >&2; exit 4; }
grep -Eq 'reducer:reducer:info:job completed' <<<"$ALL" || { echo "missing reducer completed" >&2; exit 4; }

echo "All expected steps observed."
