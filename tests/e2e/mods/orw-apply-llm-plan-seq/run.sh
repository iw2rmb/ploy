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

echo "Submitting mods run…"
# Quick debug: show the base_ref and target_branch being sent
if command -v grep >/dev/null 2>&1; then
  echo "Scenario base_ref: $(grep -E '^base_ref:' -m1 "$SCENARIO_YAML" || echo '<none>')"
  echo "Scenario target_branch: $(grep -E '^target_branch:' -m1 "$SCENARIO_YAML" || echo '<none>')"
fi
CONFIG_ESCAPED=$(jq -Rs . < "$SCENARIO_YAML")
BODY=$(cat <<EOF
{
  "config": $CONFIG_ESCAPED,
  "test_mode": false
}
EOF
)

RESP=$(curl -sS -w "\nHTTP_CODE:%{http_code}" -H "Content-Type: application/json" \
  -X POST "$API_BASE/mods" -d "$BODY")

HTTP_CODE=$(echo "$RESP" | awk -FHTTP_CODE: 'END{print $2}')
JSON=$(echo "$RESP" | sed '/HTTP_CODE:/d')
if [[ "$HTTP_CODE" != "202" && "$HTTP_CODE" != "200" && "$HTTP_CODE" != "201" ]]; then
  echo "Run submission failed: HTTP $HTTP_CODE" >&2
  # Pretty-print JSON if possible; otherwise show raw body without jq noise
  if echo "$JSON" | jq . 1>&2 2>/dev/null; then :; else echo "$JSON" >&2; fi
  exit 1
fi

MOD_ID=$(echo "$JSON" | jq -r '.mod_id // .execution_id // .id')
if [[ -z "$MOD_ID" || "$MOD_ID" == "null" ]]; then
  echo "No mod_id in response:" >&2
  echo "$JSON" | jq . >&2 || echo "$JSON" >&2
  exit 1
fi

LOG_DIR="$ROOT_DIR/logs/$MOD_ID"
mkdir -p "$LOG_DIR"
echo "$JSON" > "$LOG_DIR/run_response.json"
echo "MOD_ID: $MOD_ID"

echo "Streaming events to $LOG_DIR/events.sse …"
(
  set +e
  curl -sN "$API_BASE/mods/$MOD_ID/logs?follow=1" \
    | tee "$LOG_DIR/events.sse"
) &
SSE_PID=$!

# Start an event-driven watcher to detect terminal status from SSE and signal early exit
# Creates $LOG_DIR/terminated.flag when a meta event includes status completed/failed/cancelled
(
  set +e
  for i in $(seq 1 720); do # up to ~12 minutes
    if rg -n '"status":"(completed|failed|cancelled)"' -S "$LOG_DIR/events.sse" >/dev/null 2>&1; then
      touch "$LOG_DIR/terminated.flag"
      break
    fi
    sleep 1
  done
) &
WATCH_PID=$!

# Poll status
echo "Polling status…"
TERM_STATUS=""
MR_URL=""
START_TS=$(date +%s)
TIMEOUT_SEC=${TIMEOUT_SEC:-3600}
while :; do
  ST_JSON=$(curl -sS "$API_BASE/mods/$MOD_ID/status" || true)
  if [[ -n "$ST_JSON" ]]; then
    echo "$ST_JSON" > "$LOG_DIR/status_last.json"
    if [[ "${ST_JSON:0:1}" == "{" ]]; then
      TERM_STATUS=$(echo "$ST_JSON" | jq -r '.status // empty' 2>/dev/null || echo "")
      MR_URL=$(echo "$ST_JSON" | jq -r '.result.mr_url // empty' 2>/dev/null || echo "")
      PHASE=$(echo "$ST_JSON" | jq -r '.phase // empty' 2>/dev/null || echo "")
      echo "status=$TERM_STATUS phase=$PHASE"
    else
      echo "non-json status response (len=${#ST_JSON})" >&2
    fi
  fi
  # Event-driven short-circuit: if watcher detected terminal in SSE, break after one final status fetch
  if [[ -f "$LOG_DIR/terminated.flag" && -n "$TERM_STATUS" ]]; then
    break
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
# Gracefully terminate children of the SSE subshell to avoid noisy 'Terminated: 15' messages
if ps -p "$SSE_PID" >/dev/null 2>&1; then
  pkill -TERM -P "$SSE_PID" >/dev/null 2>&1 || true
  wait "$SSE_PID" >/dev/null 2>&1 || true
fi
if ps -p "$WATCH_PID" >/dev/null 2>&1; then
  kill "$WATCH_PID" >/dev/null 2>&1 || true
  wait "$WATCH_PID" >/dev/null 2>&1 || true
fi

echo "Fetching artifacts…"
"$ROOT_DIR/fetch-artifacts.sh" "$MOD_ID" || true

echo "Summary:"
echo "  MOD_ID: $MOD_ID"
echo "  STATUS:  $TERM_STATUS"
echo "  MR_URL:  ${MR_URL:-<none>}"
echo "Logs and artifacts under: $LOG_DIR"

if [[ "$TERM_STATUS" != "completed" ]]; then
  echo "Run did not complete successfully (status=$TERM_STATUS). Check $LOG_DIR for details." >&2
  # Always collect logs (controller/platform) and referenced SeaweedFS artifacts for diagnosis
  if [[ -x "$ROOT_DIR/collect-logs.sh" ]]; then
    echo "Collecting logs and artifacts via collect-logs.sh …"
    PLOY_CONTROLLER="$API_BASE" PLOY_SEAWEEDFS_URL="${PLOY_SEAWEEDFS_URL:-}" "$ROOT_DIR/collect-logs.sh" "$MOD_ID" || true
  fi
  exit 2
fi

# On success also collect logs for traceability (optional, non-fatal)
if [[ -x "$ROOT_DIR/collect-logs.sh" ]]; then
  echo "Collecting logs and artifacts via collect-logs.sh …"
  PLOY_CONTROLLER="$API_BASE" PLOY_SEAWEEDFS_URL="${PLOY_SEAWEEDFS_URL:-}" "$ROOT_DIR/collect-logs.sh" "$MOD_ID" || true
fi

exit 0
