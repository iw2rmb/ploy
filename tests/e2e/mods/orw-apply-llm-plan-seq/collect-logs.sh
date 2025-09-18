#!/usr/bin/env bash
set -euo pipefail

# Collects controller/platform logs and referenced SeaweedFS artifacts for a given MOD_ID.
# Usage:
#   Ensure PLOY_CONTROLLER=https://api.dev.ployman.app/v1, then run: ./collect-logs.sh <MOD_ID>
# Optional:
#   PLOY_SEAWEEDFS_URL=http://seaweedfs-filer.storage.ploy.local:8888
#   LINES=800            # number of platform log lines to fetch
#   FOLLOW_SECONDS=0     # if >0, follow SSE for N seconds and save full stream
#   TARGET_HOST=<ip>     # if set, also SSH to VPS and fetch last_job alloc logs via job-manager wrapper
#   COMPRESS=0|1         # if 1, gzip large logs

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MOD_ID="${1:-}"
if [[ -z "${PLOY_CONTROLLER:-}" ]]; then
  echo "PLOY_CONTROLLER must be set (e.g., https://api.dev.ployman.app/v1)" >&2
  exit 1
fi
if [[ -z "$MOD_ID" ]]; then
  echo "Usage: $0 <MOD_ID>" >&2
  exit 1
fi

LINES="${LINES:-800}"
FOLLOW_SECONDS="${FOLLOW_SECONDS:-0}"
OUT_DIR="$ROOT_DIR/logs/$MOD_ID"
mkdir -p "$OUT_DIR"

log() { echo "[collect] $*"; }

# 1) Fetch status snapshot
log "Fetching status JSON"
BUILDER_LOG_KEY=""
BUILDER_LOG_URL=""
if curl -fsS "$PLOY_CONTROLLER/mods/$MOD_ID/status" -o "$OUT_DIR/status_latest.json"; then
  jq -r '{status,phase,error,last_job} | @json' "$OUT_DIR/status_latest.json" 2>/dev/null || true
  if [[ -s "$OUT_DIR/status_latest.json" ]]; then
    # Inspect error field for builder log pointers emitted by the build gate helper.
    ERR_TEXT=$(jq -r '.error // empty' "$OUT_DIR/status_latest.json")
    if [[ -n "$ERR_TEXT" ]]; then
      BUILDER_LOG_KEY=$(printf '%s' "$ERR_TEXT" | rg -o 'build-logs/[A-Za-z0-9_.:-]+\.log' -m1 || true)
      BUILDER_LOG_URL=$(printf '%s' "$ERR_TEXT" | rg -o 'https?://[^ )]+/build-logs/[A-Za-z0-9_.:-]+\.log' -m1 || true)
    fi
    # Fallback to structured fields if status payload exposes them separately.
    if [[ -z "$BUILDER_LOG_KEY" ]]; then
      BUILDER_LOG_KEY=$(jq -r 'try .result.builder_logs_key // .result.builder.logs_key // empty' "$OUT_DIR/status_latest.json" 2>/dev/null || true)
    fi
    if [[ -z "$BUILDER_LOG_URL" ]]; then
      BUILDER_LOG_URL=$(jq -r 'try .result.builder_logs_url // .result.builder.logs_url // empty' "$OUT_DIR/status_latest.json" 2>/dev/null || true)
    fi
  fi
else
  echo "warning: failed to fetch status" >&2
fi

# Best-effort extraction of app name from events or status for API log downloads
APP_NAME=""
if compgen -G "$OUT_DIR/events*.sse" >/dev/null; then
  APP_NAME=$(rg -o 'app=([A-Za-z0-9_.:-]+)' "$OUT_DIR"/events*.sse 2>/dev/null | head -n1 | cut -d= -f2 || true)
fi
if [[ -z "$APP_NAME" && -s "$OUT_DIR/status_latest.json" ]]; then
  APP_NAME=$(jq -r 'try (.steps[] | select(.step == "build-gate") | capture("app=(?<name>[A-Za-z0-9_.:-]+)").name) // empty' "$OUT_DIR/status_latest.json" 2>/dev/null | head -n1)
fi

# 2) Fetch an events snapshot stream (5s sample) and optionally a longer follow capture
log "Fetching events SSE snapshot (5s sample)"
curl -N -sS --max-time 5 "$PLOY_CONTROLLER/mods/$MOD_ID/logs?follow=1" \
  -o "$OUT_DIR/events.sample.sse" || true

if [[ "$FOLLOW_SECONDS" =~ ^[1-9][0-9]*$ ]]; then
  log "Following events SSE for ${FOLLOW_SECONDS}s"
  curl -N -sS --max-time "$FOLLOW_SECONDS" "$PLOY_CONTROLLER/mods/$MOD_ID/logs?follow=1" \
    -o "$OUT_DIR/events.sse" || true
fi

# If a longer events.sse exists from run.sh, preserve and point to it
if [[ -s "$OUT_DIR/events.sse" ]]; then
  log "Found existing events.sse (from run), keeping it alongside sample"
fi

# 3) Fetch platform logs (API and Traefik) for recent lines
log "Fetching platform API logs ($LINES lines)"
curl -fsS "$PLOY_CONTROLLER/platform/api/logs?lines=${LINES}" -o "$OUT_DIR/platform_api.log" || true
log "Fetching Traefik logs ($LINES lines)"
curl -fsS "$PLOY_CONTROLLER/platform/traefik/logs?lines=${LINES}" -o "$OUT_DIR/traefik.log" || true

# 4) Filter events for quick diagnosis
log "Filtering errors and key-step events"
{
  echo "# Errors"
  grep -E 'level\":\"error\"' "$OUT_DIR"/events*.sse 2>/dev/null || true
  echo
  echo "# Planner/LLM/Reducer/Apply steps"
  grep -E 'step\":\"(planner|llm-exec|reducer|apply)' "$OUT_DIR"/events*.sse 2>/dev/null || true
} > "$OUT_DIR/events.filtered.txt" || true

# 5) Extract artifact keys from events and download from SeaweedFS (if URL set)
ART_KEYS_FILE="$OUT_DIR/artifact_keys.txt"
grep -E 'uploaded (plan|diff) to ' "$OUT_DIR"/events*.sse 2>/dev/null \
 | sed -E 's/.*uploaded (plan|diff) to ([^"} ]+).*/\2/' \
 | sort -u > "$ART_KEYS_FILE" || true

if [[ -s "$ART_KEYS_FILE" ]]; then
  log "Found $(wc -l <"$ART_KEYS_FILE") artifact keys in events"
else
  log "No artifact keys found in events (ok if uploads did not occur)"
fi

# Persist builder log pointers if present for downstream download helpers
if [[ -n "$BUILDER_LOG_KEY" ]]; then
  echo "$BUILDER_LOG_KEY" > "$OUT_DIR/builder_logs.key"
  if [[ -n "$BUILDER_LOG_URL" ]]; then
    echo "$BUILDER_LOG_URL" > "$OUT_DIR/builder_logs.url"
  fi
  log "Builder logs pointer detected: $BUILDER_LOG_KEY"
fi

BUILDER_JOB=""
if [[ -n "$BUILDER_LOG_KEY" ]]; then
  BUILDER_JOB=${BUILDER_LOG_KEY##*/}
  BUILDER_JOB=${BUILDER_JOB%.log}
  if [[ -z "$APP_NAME" ]]; then
    case "$BUILDER_JOB" in
      *-c-build-*) APP_NAME="${BUILDER_JOB%%-c-build-*}" ;;
      *-e-build-*) APP_NAME="${BUILDER_JOB%%-e-build-*}" ;;
      *-lane-*)    APP_NAME="${BUILDER_JOB%%-lane-*}" ;;
    esac
  fi
fi

if [[ -n "${PLOY_SEAWEEDFS_URL:-}" && -s "$ART_KEYS_FILE" ]]; then
  log "Downloading artifacts from SeaweedFS"
  while IFS= read -r KEY; do
    # Mirror path under logs: logs/<MOD_ID>/seaweedfs/mods/.../file
    DEST="$OUT_DIR/seaweedfs/$KEY"
    mkdir -p "$(dirname "$DEST")"
    URL="${PLOY_SEAWEEDFS_URL%/}/artifacts/${KEY}"
    if curl -fsS "$URL" -o "$DEST"; then
      log "Downloaded $KEY"
    else
      echo "warning: failed to download $KEY from $URL" >&2
    fi
  done < "$ART_KEYS_FILE"
else
  if [[ -z "${PLOY_SEAWEEDFS_URL:-}" ]]; then
    log "PLOY_SEAWEEDFS_URL not set; skipping SeaweedFS artifact download"
  fi
fi

# Fetch builder logs from SeaweedFS when the pointer is available.
if [[ -n "$BUILDER_LOG_KEY" && -n "${PLOY_SEAWEEDFS_URL:-}" ]]; then
  BUILDER_DEST="$OUT_DIR/seaweedfs/$BUILDER_LOG_KEY"
  mkdir -p "$(dirname "$BUILDER_DEST")"
  BUILDER_URL_FULL="${PLOY_SEAWEEDFS_URL%/}/artifacts/${BUILDER_LOG_KEY}"
  if curl -fsS "$BUILDER_URL_FULL" -o "$BUILDER_DEST"; then
    log "Downloaded builder logs to $BUILDER_DEST"
  else
    echo "warning: failed to download builder logs from $BUILDER_URL_FULL" >&2
    rm -f "$BUILDER_DEST" || true
  fi
else
  if [[ -z "${PLOY_SEAWEEDFS_URL:-}" ]]; then
    log "PLOY_SEAWEEDFS_URL not set in current shell; relying on SSH fallback if TARGET_HOST is provided"
  fi
fi

BUILDER_API_DEST=""
if [[ -n "$BUILDER_JOB" && -n "$APP_NAME" && -n "${PLOY_CONTROLLER:-}" ]]; then
  DEPLOY_ID="$BUILDER_JOB"
  BUILDER_API_URL="${PLOY_CONTROLLER%/}/apps/${APP_NAME}/builds/${DEPLOY_ID}/logs/download"
  BUILDER_API_DEST="$OUT_DIR/builder_logs.api.log"
  if curl -fsS "$BUILDER_API_URL" -o "$BUILDER_API_DEST"; then
    log "Downloaded builder logs via API to $BUILDER_API_DEST"
  else
    echo "warning: failed to download builder logs via API $BUILDER_API_URL" >&2
    BUILDER_API_DEST=""
  fi
fi

# 5b) Optional: fetch last_job allocation logs via SSH job-manager wrapper
if [[ -n "${TARGET_HOST:-}" ]]; then
  log "Attempting SSH fetch of last_job allocation logs"
  LAST_ALLOC_ID=$(jq -r 'try .last_job.alloc_id // .last_job.AllocID // empty' "$OUT_DIR/status_latest.json" 2>/dev/null || true)
  LAST_JOB_NAME=$(jq -r 'try .last_job.job_name // .last_job.JobName // empty' "$OUT_DIR/status_latest.json" 2>/dev/null || true)
  # Derive a --since timestamp from first SSE event (ISO8601 -> "YYYY-MM-DD HH:MM:SS"); fallback to VPS/platform clock
  SINCE_FMT=""
  if grep -hqo '"time":"' "$OUT_DIR"/events*.sse 2>/dev/null; then
    SINCE_RAW=$(grep -hEo '"time":"[^"]+"' "$OUT_DIR"/events*.sse 2>/dev/null | head -n1 | sed -E 's/.*"time":"([^\"]+)".*/\1/' || true)
    if [[ -n "$SINCE_RAW" ]]; then
      SINCE_FMT="${SINCE_RAW:0:10} ${SINCE_RAW:11:8}"
    fi
  fi
  if [[ -z "$SINCE_FMT" && -n "${START_TS:-}" ]]; then
    SINCE_FMT="$START_TS"
  fi
  if [[ -z "$SINCE_FMT" && -n "${START_TS_SOURCE:-}" ]]; then
    case "$START_TS_SOURCE" in
      vps)
        VPS_TS=$(ssh -o ConnectTimeout=10 "root@$TARGET_HOST" "date '+%Y-%m-%d %H:%M:%S'" 2>/dev/null || true)
        if [[ -n "$VPS_TS" ]]; then SINCE_FMT="$VPS_TS"; fi
        ;;
      platform)
        SNAP=$(curl -sf "${PLOY_CONTROLLER%/}/platform/api/logs?lines=20" 2>/dev/null || true)
        if [[ -n "$SNAP" ]]; then
          TS=$(printf '%s\n' "$SNAP" | awk 'match($0,/^\[([0-9-]{10} [0-9:]{8})\]/,m){ts=m[1]} END{print ts}')
          if [[ -n "$TS" ]]; then SINCE_FMT="$TS"; fi
        fi
        ;;
    esac
  fi
  if [[ -n "$SINCE_FMT" ]]; then
    log "Using log since timestamp: $SINCE_FMT"
  fi
  if [[ -n "$LAST_ALLOC_ID" ]]; then
    log "Fetching logs for alloc=$LAST_ALLOC_ID job=$LAST_JOB_NAME"
    ssh -o ConnectTimeout=10 "root@$TARGET_HOST" \
      "su - ploy -c '/opt/hashicorp/bin/nomad-job-manager.sh logs --alloc-id $LAST_ALLOC_ID --both --lines $LINES ${SINCE_FMT:+--since "$SINCE_FMT"}'" \
      | sed -e "1s/^/[alloc:$LAST_ALLOC_ID] /" \
      > "$OUT_DIR/last_job.logs" || true
  else
    log "No last_job.alloc_id in status; skipping SSH logs"
  fi
fi

# 5c) SSH fallback to fetch SeaweedFS artifacts when URL not set
if [[ -z "${PLOY_SEAWEEDFS_URL:-}" && -n "${TARGET_HOST:-}" && -s "$ART_KEYS_FILE" ]]; then
  log "Downloading artifacts from SeaweedFS via SSH"
  while IFS= read -r KEY; do
    DEST="$OUT_DIR/seaweedfs/$KEY"
    mkdir -p "$(dirname "$DEST")"
    ssh -o ConnectTimeout=10 "root@$TARGET_HOST" \
      "curl -fsS 'http://seaweedfs-filer.storage.ploy.local:8888/artifacts/${KEY}'" \
      > "$DEST" 2>/dev/null || {
        echo "warning: failed to fetch $KEY via SSH" >&2
        rm -f "$DEST" || true
      }
  done < "$ART_KEYS_FILE"
fi

if [[ -z "${PLOY_SEAWEEDFS_URL:-}" && -n "${TARGET_HOST:-}" && -n "$BUILDER_LOG_KEY" ]]; then
  log "Fetching builder logs from SeaweedFS via SSH"
  DEST="$OUT_DIR/seaweedfs/$BUILDER_LOG_KEY"
  mkdir -p "$(dirname "$DEST")"
  ssh -o ConnectTimeout=10 "root@$TARGET_HOST" \
    "curl -fsS 'http://seaweedfs-filer.storage.ploy.local:8888/artifacts/${BUILDER_LOG_KEY}'" \
    > "$DEST" 2>/dev/null || {
      echo "warning: failed to fetch builder logs via SSH" >&2
      rm -f "$DEST" || true
    }
fi

# 6) Summarize
SUMMARY="$OUT_DIR/summary.txt"
{
  echo "MOD_ID: $MOD_ID"
  echo "Controller: $PLOY_CONTROLLER"
  echo "SeaweedFS: ${PLOY_SEAWEEDFS_URL:-<unset>}"
  echo
  echo "Status:"
  if [[ -s "$OUT_DIR/status_latest.json" ]]; then
    jq -r '{status,phase,error,last_job} | .status + ", phase=" + .phase + (if .error then "\n"+.error else "" end) + (if .last_job and .last_job.job_name then "\nlast_job="+.last_job.job_name else "" end)' "$OUT_DIR/status_latest.json" 2>/dev/null || true
  else
    echo "<missing>"
  fi
  echo
  echo "Events (errors) → events.filtered.txt"
  echo "Platform logs → platform_api.log, traefik.log"
  if [[ -s "$ART_KEYS_FILE" ]]; then
    echo "Artifact keys (from events):"
    cat "$ART_KEYS_FILE"
  fi
  if [[ -n "$BUILDER_LOG_KEY" ]]; then
    echo
    echo "Builder logs key: $BUILDER_LOG_KEY"
    if [[ -n "$BUILDER_LOG_URL" ]]; then
      echo "Builder logs URL: $BUILDER_LOG_URL"
    fi
    if [[ -f "$OUT_DIR/seaweedfs/$BUILDER_LOG_KEY" ]]; then
      echo "Builder logs saved to: $OUT_DIR/seaweedfs/$BUILDER_LOG_KEY"
    elif [[ -n "$BUILDER_API_DEST" ]]; then
      echo "Builder logs saved via API to: $BUILDER_API_DEST"
    fi
    if [[ -n "$APP_NAME" && -n "$BUILDER_JOB" ]]; then
      echo "Builder logs API route: ${PLOY_CONTROLLER%/}/apps/${APP_NAME}/builds/${BUILDER_JOB}/logs/download"
    fi
  fi
  # Try to extract planner and llm-exec RUN_IDs from events to fetch context inputs.json (if SeaweedFS URL is set)
  if [[ -n "${PLOY_SEAWEEDFS_URL:-}" ]]; then
    # Planner RUN_ID: from uploaded plan key mods/<MOD_ID>/planner/<RUN_ID>/plan.json
    PLANNER_RUN_ID=$(grep -Eo 'planner/[A-Za-z0-9_.:-]+/plan.json' "$OUT_DIR"/events*.sse 2>/dev/null | sed -E 's#planner/([^/]+)/plan\.json#\1#' | head -n1)
    # LLM RUN_ID: from steps/<RUN_ID>/diff.patch in llm-exec events
    LLM_RUN_ID=$(grep -Eo 'steps/[A-Za-z0-9_.:-]+/diff\.patch' "$OUT_DIR"/events*.sse 2>/dev/null | sed -E 's#steps/([^/]+)/diff\.patch#\1#' | head -n1)
    if [[ -n "$PLANNER_RUN_ID" ]]; then
      echo "Planner RUN_ID: $PLANNER_RUN_ID"
    fi
    if [[ -n "$LLM_RUN_ID" ]]; then
      echo "LLM RUN_ID: $LLM_RUN_ID"
    fi
    # Download and extract contexts where possible
    for RID in $PLANNER_RUN_ID $LLM_RUN_ID; do
      [[ -z "$RID" ]] && continue
      KEY_CTX="mods/${MOD_ID}/contexts/${RID}.tar"
      URL_CTX="${PLOY_SEAWEEDFS_URL%/}/artifacts/${KEY_CTX}"
      DEST_CTX_TAR="$OUT_DIR/${RID}.context.tar"
      DEST_CTX_DIR="$OUT_DIR/${RID}.context"
      if curl -fsS "$URL_CTX" -o "$DEST_CTX_TAR"; then
        mkdir -p "$DEST_CTX_DIR"
        tar -xf "$DEST_CTX_TAR" -C "$DEST_CTX_DIR" 2>/dev/null || true
        if [[ -s "$DEST_CTX_DIR/inputs.json" ]]; then
          echo "Context inputs.json for $RID:" >> "$SUMMARY"
          # Save full inputs.json alongside for inspection
          cp "$DEST_CTX_DIR/inputs.json" "$OUT_DIR/inputs.$RID.json" 2>/dev/null || true
          # Compact summary: first_error_file/line and errors[] count
          if command -v jq >/dev/null 2>&1; then
            fe_file=$(jq -r 'try .first_error_file // empty' "$OUT_DIR/inputs.$RID.json")
            fe_line=$(jq -r 'try .first_error_line // empty' "$OUT_DIR/inputs.$RID.json")
            errs_cnt=$(jq -r 'try (.errors | length) // 0' "$OUT_DIR/inputs.$RID.json")
          else
            fe_file=$(sed -n 's/.*"first_error_file"[[:space:]]*:[[:space:]]*"\([^"\n]*\)".*/\1/p' "$OUT_DIR/inputs.$RID.json" | head -n1)
            fe_line=$(sed -n 's/.*"first_error_line"[[:space:]]*:[[:space:]]*\([0-9][0-9]*\).*/\1/p' "$OUT_DIR/inputs.$RID.json" | head -n1)
            # rough count of errors entries
            errs_cnt=$(grep -c '"file"[[:space:]]*:' "$OUT_DIR/inputs.$RID.json" 2>/dev/null || echo 0)
          fi
          echo "  first_error_file: ${fe_file:-<none>}" >> "$SUMMARY"
          echo "  first_error_line: ${fe_line:-<none>}" >> "$SUMMARY"
          echo "  errors_count: ${errs_cnt:-0}" >> "$SUMMARY"
          # Also append a brief note into events.filtered.txt for quick scan
          {
            echo "# Context summary ($RID)"
            echo "first_error_file=${fe_file:-<none>} first_error_line=${fe_line:-<none>} errors_count=${errs_cnt:-0}"
          } >> "$OUT_DIR/events.filtered.txt"
        else
          echo "Context for $RID downloaded, but inputs.json not found" >> "$SUMMARY"
        fi
      else
        echo "warning: failed to download context tar for $RID from $URL_CTX" >> "$SUMMARY"
      fi
    done
  fi
} > "$SUMMARY"

log "Done. Collected logs in $OUT_DIR"

# Optional compression to reduce artifact size
if [[ "${COMPRESS:-0}" == "1" ]]; then
  find "$OUT_DIR" -type f \( -name '*.log' -o -name '*.sse' -o -name '*.txt' \) -size +256k -print0 \
    | xargs -0 -n1 gzip -9 -f || true
  log "Compressed large logs with gzip"
fi
