#!/usr/bin/env bash
set -euo pipefail

# Collects controller/platform logs and referenced SeaweedFS artifacts for a given MOD_ID.
# Usage:
#   Ensure PLOY_CONTROLLER=https://api.dev.ployman.app/v1, then run: ./collect-logs.sh <MOD_ID>
# Optional:
#   PLOY_SEAWEEDFS_URL=http://seaweedfs-filer.service.consul:8888
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
if curl -fsS "$PLOY_CONTROLLER/mods/$MOD_ID/status" -o "$OUT_DIR/status_latest.json"; then
  jq -r '{status,phase,error,last_job} | @json' "$OUT_DIR/status_latest.json" 2>/dev/null || true
else
  echo "warning: failed to fetch status" >&2
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

# 5b) Optional: fetch last_job allocation logs via SSH job-manager wrapper
if [[ -n "${TARGET_HOST:-}" ]]; then
  log "Attempting SSH fetch of last_job allocation logs"
  LAST_ALLOC_ID=$(jq -r 'try .last_job.alloc_id // .last_job.AllocID // empty' "$OUT_DIR/status_latest.json" 2>/dev/null || true)
  LAST_JOB_NAME=$(jq -r 'try .last_job.job_name // .last_job.JobName // empty' "$OUT_DIR/status_latest.json" 2>/dev/null || true)
  if [[ -n "$LAST_ALLOC_ID" ]]; then
    log "Fetching logs for alloc=$LAST_ALLOC_ID job=$LAST_JOB_NAME"
    ssh -o ConnectTimeout=10 "root@$TARGET_HOST" \
      "su - ploy -c '/opt/hashicorp/bin/nomad-job-manager.sh logs --alloc-id $LAST_ALLOC_ID --both --lines $LINES'" \
      | sed -e "1s/^/[alloc:$LAST_ALLOC_ID] /" \
      > "$OUT_DIR/last_job.logs" || true
  else
    log "No last_job.alloc_id in status; skipping SSH logs"
  fi
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
} > "$SUMMARY"

log "Done. Collected logs in $OUT_DIR"

# Optional compression to reduce artifact size
if [[ "${COMPRESS:-0}" == "1" ]]; then
  find "$OUT_DIR" -type f \( -name '*.log' -o -name '*.sse' -o -name '*.txt' \) -size +256k -print0 \
    | xargs -0 -n1 gzip -9 -f || true
  log "Compressed large logs with gzip"
fi
