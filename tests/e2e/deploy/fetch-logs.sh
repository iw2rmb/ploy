#!/usr/bin/env bash
set -euo pipefail

# Fetch logs to inspect issues for an app, plus platform logs; optionally app task logs via SSH.
# Usage:
#   APP_NAME=<name> [LANE=D] [SHA=<sha12>] [LINES=200] [TARGET_HOST=ip] [BUILD_ID=<id>] [FOLLOW=false] [OUT_DIR=<path>] ./tests/e2e/deploy/fetch-logs.sh
# Notes:
# - Keeps stdout output for CI readability. If OUT_DIR is set, also writes files under it.

APP_NAME=${APP_NAME:-}
LANE=${LANE:-D}
SHA=${SHA:-}
LINES=${LINES:-200}
FILTER_MARKERS=${FILTER_MARKERS:-}
START_TS=${START_TS:-}
START_TS_SOURCE=${START_TS_SOURCE:-} # vps|platform|local (default local)
FOLLOW=${FOLLOW:-false}
OUT_DIR=${OUT_DIR:-}
BUILD_ID=${BUILD_ID:-}

if [[ -z "$APP_NAME" ]]; then echo "APP_NAME required" >&2; exit 2; fi

PC=${PLOY_CONTROLLER%/}

# Prepare OUT_DIR if provided
if [[ -n "$OUT_DIR" ]]; then
  mkdir -p "$OUT_DIR" 2>/dev/null || true
fi

if [[ -n "${PC:-}" ]]; then
  # Optionally resolve START_TS from VPS or platform logs to avoid timezone skew
  if [[ -z "$START_TS" && -n "$START_TS_SOURCE" ]]; then
    case "$START_TS_SOURCE" in
      vps)
        if [[ -n "${TARGET_HOST:-}" ]]; then
          START_TS=$(ssh -o ConnectTimeout=10 "root@$TARGET_HOST" "date '+%Y-%m-%d %H:%M:%S'" 2>/dev/null || true)
        fi
        ;;
      platform)
        # Pull a tiny snapshot and use the last bracketed timestamp as START_TS
        SNAP=$(curl -sf "$PC/platform/api/logs?lines=20" 2>/dev/null || true)
        if [[ -n "$SNAP" ]]; then
          TS=$(printf '%s\n' "$SNAP" | awk 'match($0,/^\[([0-9-]{10} [0-9:]{8})\]/,m){ts=m[1]} END{print ts}')
          if [[ -n "$TS" ]]; then START_TS="$TS"; fi
        fi
        ;;
    esac
    if [[ -n "$START_TS" ]]; then echo "== START_TS resolved: $START_TS (source: $START_TS_SOURCE)" >&2; fi
  fi

  echo "== App status" >&2
  if [[ -n "$OUT_DIR" ]]; then
    curl -sf "$PC/apps/$APP_NAME/status" | tee "$OUT_DIR/app.status.json" || true; echo
  else
    curl -sf "$PC/apps/$APP_NAME/status" || true; echo
  fi
  echo "== App logs ($LINES)" >&2
  if [[ -n "$OUT_DIR" ]]; then
    curl -sf "$PC/apps/$APP_NAME/logs?lines=$LINES" | tee "$OUT_DIR/app.logs.json" || true; echo
  else
    curl -sf "$PC/apps/$APP_NAME/logs?lines=$LINES" || true; echo
  fi
  echo "== Platform API logs ($LINES, follow=$FOLLOW)" >&2
  if [[ -n "$OUT_DIR" ]]; then
    curl -sf "$PC/platform/api/logs?lines=$LINES&follow=$FOLLOW" | tee "$OUT_DIR/platform_api.log" || true; echo
    # Optional: time-based slicing — keep only lines at or after START_TS if provided
    if [[ -n "$START_TS" ]]; then
      echo "== Platform API logs (sliced since $START_TS)" >&2
      awk -v start="$START_TS" '
        BEGIN { printed=0 }
        {
          ts="";
          if ($0 ~ /^\[/) {
            # Timestamp formatted as [YYYY-MM-DD HH:MM:SS]
            ts=substr($0, 2, 19);
          }
          if (ts=="" && printed==1) { print; next }
          if (ts!="" && ts >= start) { printed=1; print; next }
        }' "$OUT_DIR/platform_api.log" | tee "$OUT_DIR/platform_api.sliced.log" >/dev/null || true; echo
    fi
    if [[ -n "$FILTER_MARKERS" ]]; then
      echo "== Platform API logs (filtered)" >&2
      SRC_FILE="$OUT_DIR/platform_api.log"
      if [[ -f "$OUT_DIR/platform_api.sliced.log" ]]; then SRC_FILE="$OUT_DIR/platform_api.sliced.log"; fi
      rg -n "$FILTER_MARKERS" "$SRC_FILE" | tee "$OUT_DIR/platform_api.filtered.log" || true; echo
    fi
  else
    curl -sf "$PC/platform/api/logs?lines=$LINES&follow=$FOLLOW" || true; echo
  fi
  echo "== Traefik logs ($LINES, follow=$FOLLOW)" >&2
  if [[ -n "$OUT_DIR" ]]; then
    curl -sf "$PC/platform/traefik/logs?lines=$LINES&follow=$FOLLOW" | tee "$OUT_DIR/traefik.log" || true; echo
  else
    curl -sf "$PC/platform/traefik/logs?lines=$LINES&follow=$FOLLOW" || true; echo
  fi
fi

# Optional: fetch builder logs via API when BUILD_ID is provided
if [[ -n "${PC:-}" && -n "${BUILD_ID:-}" ]]; then
  echo "== Builder logs via API (build id: $BUILD_ID)" >&2
  if [[ -n "$OUT_DIR" ]]; then
    curl -sf "$PC/apps/$APP_NAME/builds/$BUILD_ID/logs?lines=$LINES" | tee "$OUT_DIR/builder.logs.json" || true; echo
  else
    curl -sf "$PC/apps/$APP_NAME/builds/$BUILD_ID/logs?lines=$LINES" || true; echo
  fi
fi

# Optional: fetch builder logs via SSH when TARGET_HOST is provided (not applicable for Lane D host builds)
if [[ -n "${TARGET_HOST:-}" ]]; then
  # App job allocs + alloc status + task logs (Lane D only)
  L_LOWER=$(printf '%s' "$LANE" | tr '[:upper:]' '[:lower:]')
  CANDIDATES=("$APP_NAME-lane-$L_LOWER")
  for JOB in "${CANDIDATES[@]}"; do
    echo "== SSH: app job allocs ($JOB)" >&2
    ssh -o ConnectTimeout=10 "root@$TARGET_HOST" "su - ploy -c '/opt/hashicorp/bin/nomad-job-manager.sh allocs --job $JOB --format human'" || true
    ALLOC_ID=$(ssh -o ConnectTimeout=10 "root@$TARGET_HOST" "su - ploy -c '/opt/hashicorp/bin/nomad-job-manager.sh running-alloc --job $JOB'" 2>/dev/null | tail -n1 || true)
    if [[ -n "$ALLOC_ID" ]]; then
      echo "== SSH: app alloc status ($ALLOC_ID)" >&2
      ssh -o ConnectTimeout=10 "root@$TARGET_HOST" "su - ploy -c '/opt/hashicorp/bin/nomad-job-manager.sh alloc-status --alloc-id $ALLOC_ID'" || true
      echo "== SSH: app task logs (docker-runtime stdout+stderr)" >&2
      if [[ -n "$OUT_DIR" ]]; then
        ssh -o ConnectTimeout=10 "root@$TARGET_HOST" "su - ploy -c '/opt/hashicorp/bin/nomad-job-manager.sh logs --alloc-id $ALLOC_ID --task docker-runtime --both --lines $LINES'" | tee "$OUT_DIR/app-$JOB.docker.log" || true
      else
        ssh -o ConnectTimeout=10 "root@$TARGET_HOST" "su - ploy -c '/opt/hashicorp/bin/nomad-job-manager.sh logs --alloc-id $ALLOC_ID --task docker-runtime --both --lines $LINES'" || true
      fi
      break
    fi
  done
fi

echo "Done." >&2
