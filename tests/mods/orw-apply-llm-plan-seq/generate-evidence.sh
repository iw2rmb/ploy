#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MOD_DIR="${1:-}"

if [[ -z "$MOD_DIR" ]]; then
  # Pick most recent mod-*/ by mtime
  MOD_DIR=$(ls -td "$ROOT_DIR"/logs/mod-* 2>/dev/null | head -n1 || true)
fi

if [[ -z "$MOD_DIR" || ! -d "$MOD_DIR" ]]; then
  echo "No logs/mod-*/ directory found. Run ./run.sh first." >&2
  exit 1
fi

EVID="$MOD_DIR/evidence.txt"
EVS="$MOD_DIR/events.sse"
STAT="$MOD_DIR/status_last.json"
DIFF="$MOD_DIR/diff.patch"

{
  echo "Evidence for $(basename "$MOD_DIR")"
  echo
  echo "[1] ORW apply → build gate context"
  rg -n "\\b(orw-apply|diff-found|diff-apply-started|build-gate-start|build-gate-failed|build-gate-succeeded)\\b" "$EVS" -n -S || true
  echo
  echo "[2] Planner/LLM inputs prepared (prompt/error context)"
  rg -n "prepared inputs.json|prompt enriched" "$EVS" -n -S || true
  echo
  echo "[3] Build error (from status_last.json)"
  if [[ -s "$STAT" ]]; then
    jq -r '.error // empty' "$STAT" | sed -n '1,120p'
  fi
  echo
  echo "[3a] Builder logs pointer"
  if [[ -f "$MOD_DIR/builder_logs.key" ]]; then
    printf 'Key: '
    cat "$MOD_DIR/builder_logs.key"
    if [[ -f "$MOD_DIR/builder_logs.url" ]]; then
      printf 'URL: '
      cat "$MOD_DIR/builder_logs.url"
    fi
    BUILDER_JOB=$(basename "$(cat "$MOD_DIR/builder_logs.key")")
    BUILDER_JOB=${BUILDER_JOB%.log}
    APP_NAME=""
    if [[ -s "$STAT" ]]; then
      APP_NAME=$(jq -r 'try (.steps[] | select(.step == "build-gate") | capture("app=(?<name>[A-Za-z0-9_.:-]+)").name) // empty' "$STAT" 2>/dev/null | head -n1)
    fi
    if [[ -z "$APP_NAME" ]]; then
      case "$BUILDER_JOB" in
        *-c-build-*) APP_NAME="${BUILDER_JOB%%-c-build-*}" ;;
        *-e-build-*) APP_NAME="${BUILDER_JOB%%-e-build-*}" ;;
      esac
    fi
    if [[ -n "$APP_NAME" && -n "$BUILDER_JOB" ]]; then
      echo "API route: /v1/apps/${APP_NAME}/builds/${BUILDER_JOB}/logs/download"
    fi
  else
    echo "(no builder logs pointer detected)"
  fi
  echo
  echo "[4] LLM diff (first 120 lines if exists)"
  if [[ -s "$DIFF" ]]; then
    sed -n '1,120p' "$DIFF"
  else
    echo "(no diff.patch captured by controller)"
  fi
  echo
  echo "[5] LLM/Reducer artifact events"
  rg -n "llm-exec|reducer.*download|uploaded diff|branches/.*/steps/.*/diff.patch|plan.json|next.json|bytes=" "$EVS" -n -S || true
} > "$EVID"

echo "Wrote $EVID"
