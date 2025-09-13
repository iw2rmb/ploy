#!/usr/bin/env bash
set -euo pipefail

# End-to-end runner for the orw-apply → build fail → llm-plan → llm-exec scenario
# - Posts scenario.yaml to /v1/mods/run
# - Streams SSE events
# - Polls status until terminal
# - Downloads artifacts

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCENARIO_YAML="${SCENARIO_YAML:-$ROOT_DIR/scenario.yaml}"
API_BASE="${PLOY_CONTROLLER:-}"

if [[ -z "${API_BASE}" ]]; then
  echo "PLOY_CONTROLLER env must point to controller base with /v1 (e.g., https://api.dev.ployman.app/v1)" >&2
  exit 1
fi

if [[ ! -f "$SCENARIO_YAML" ]]; then
  echo "scenario.yaml not found at $SCENARIO_YAML" >&2
  exit 1
fi

mkdir -p "$ROOT_DIR/logs"

echo "Submitting transflow run…"
CONFIG_ESCAPED=$(jq -Rs . < "$SCENARIO_YAML")
BODY=$(cat <<EOF
{
  "config": $CONFIG_ESCAPED,
  "test_mode": false
}
EOF
)

RESP=$(curl -sS -w "\nHTTP_CODE:%{http_code}" -H "Content-Type: application/json" \
  -X POST "$API_BASE/transflow/run" -d "$BODY")

HTTP_CODE=$(echo "$RESP" | awk -FHTTP_CODE: 'END{print $2}')
JSON=$(echo "$RESP" | sed '/HTTP_CODE:/d')
if [[ "$HTTP_CODE" != "202" && "$HTTP_CODE" != "200" && "$HTTP_CODE" != "201" ]]; then
  echo "Run submission failed: HTTP $HTTP_CODE" >&2
  echo "$JSON" | jq . >&2 || echo "$JSON" >&2
  exit 1
fi

EXEC_ID=$(echo "$JSON" | jq -r '.execution_id')
if [[ -z "$EXEC_ID" || "$EXEC_ID" == "null" ]]; then
  echo "No execution_id in response:" >&2
  echo "$JSON" | jq . >&2 || echo "$JSON" >&2
  exit 1
fi

LOG_DIR="$ROOT_DIR/logs/$EXEC_ID"
mkdir -p "$LOG_DIR"
echo "$JSON" > "$LOG_DIR/run_response.json"
echo "EXEC_ID: $EXEC_ID"

echo "Streaming events to $LOG_DIR/events.sse …"
(
  set +e
  curl -sN "$API_BASE/transflow/logs/$EXEC_ID?follow=1" \
    | tee "$LOG_DIR/events.sse"
) &
SSE_PID=$!

# Poll status
echo "Polling status…"
TERM_STATUS=""
MR_URL=""
START_TS=$(date +%s)
TIMEOUT_SEC=${TIMEOUT_SEC:-3600}
while :; do
  ST_JSON=$(curl -sS "$API_BASE/transflow/status/$EXEC_ID" || true)
  if [[ -n "$ST_JSON" ]]; then
    echo "$ST_JSON" > "$LOG_DIR/status_last.json"
    TERM_STATUS=$(echo "$ST_JSON" | jq -r '.status // empty')
    MR_URL=$(echo "$ST_JSON" | jq -r '.result.mr_url // empty')
    PHASE=$(echo "$ST_JSON" | jq -r '.phase // empty')
    echo "status=$TERM_STATUS phase=$PHASE"
  fi
  if [[ "$TERM_STATUS" == "completed" || "$TERM_STATUS" == "failed" || "$TERM_STATUS" == "cancelled" ]]; then
    break
  fi
  NOW=$(date +%s)
  if (( NOW - START_TS > TIMEOUT_SEC )); then
    echo "Timed out waiting for terminal status after ${TIMEOUT_SEC}s" >&2
    break
  fi
  sleep 5
done

echo "Stopping SSE (pid=$SSE_PID)…"
kill "$SSE_PID" >/dev/null 2>&1 || true

echo "Fetching artifacts…"
"$ROOT_DIR/fetch-artifacts.sh" "$EXEC_ID" || true

echo "Summary:"
echo "  EXEC_ID: $EXEC_ID"
echo "  STATUS:  $TERM_STATUS"
echo "  MR_URL:  ${MR_URL:-<none>}"
echo "Logs and artifacts under: $LOG_DIR"

if [[ "$TERM_STATUS" != "completed" ]]; then
  echo "Run did not complete successfully (status=$TERM_STATUS). Check $LOG_DIR for details." >&2
  exit 2
fi

exit 0

