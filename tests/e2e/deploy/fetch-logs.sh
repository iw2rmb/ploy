#!/usr/bin/env bash
set -euo pipefail

# Fetch logs to inspect issues for an app, plus platform logs; optionally builder job logs via API and app task logs via SSH
# Usage:
#   APP_NAME=<name> [LANE=<A-G>] [SHA=<sha12>] [LINES=200] [TARGET_HOST=ip] [BUILD_ID=<id>] [FOLLOW=false] [OUT_DIR=<path>] ./tests/e2e/deploy/fetch-logs.sh
# Notes:
# - Keeps stdout output for CI readability. If OUT_DIR is set, also writes files under it.

APP_NAME=${APP_NAME:-}
LANE=${LANE:-}
SHA=${SHA:-}
LINES=${LINES:-200}
FILTER_MARKERS=${FILTER_MARKERS:-}
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
    if [[ -n "$FILTER_MARKERS" ]]; then
      echo "== Platform API logs (filtered)" >&2
      rg -n "$FILTER_MARKERS" "$OUT_DIR/platform_api.log" | tee "$OUT_DIR/platform_api.filtered.log" || true; echo
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

# Optional: fetch builder logs via SSH when TARGET_HOST is provided
if [[ -n "${TARGET_HOST:-}" ]]; then
  if [[ -n "$LANE" && -n "$SHA" ]]; then
    case "$LANE" in
      E) BJ="$APP_NAME-e-build-$SHA" ;;
      C) BJ="$APP_NAME-c-build-$SHA" ;;
      *) BJ="" ;;
    esac
    if [[ -n "$BJ" ]]; then
      echo "== SSH: builder job logs ($BJ)" >&2
      # Resolve running allocation ID for the builder job, then fetch logs
      ALLOC_ID=$(ssh -o ConnectTimeout=10 "root@$TARGET_HOST" "su - ploy -c '/opt/hashicorp/bin/nomad-job-manager.sh running-alloc --job $BJ'" 2>/dev/null || true)
      if [[ -n "$ALLOC_ID" ]]; then
        if [[ -n "$OUT_DIR" ]]; then
          ssh -o ConnectTimeout=10 "root@$TARGET_HOST" "su - ploy -c '/opt/hashicorp/bin/nomad-job-manager.sh logs --alloc-id $ALLOC_ID --task kaniko --both --lines $LINES'" | tee "$OUT_DIR/builder.ssh.log" || true
        else
          ssh -o ConnectTimeout=10 "root@$TARGET_HOST" "su - ploy -c '/opt/hashicorp/bin/nomad-job-manager.sh logs --alloc-id $ALLOC_ID --task kaniko --both --lines $LINES'" || true
        fi
      else
        # Fallback: list allocs in human form
        if [[ -n "$OUT_DIR" ]]; then
          ssh -o ConnectTimeout=10 "root@$TARGET_HOST" "su - ploy -c '/opt/hashicorp/bin/nomad-job-manager.sh allocs --job $BJ --format human'" | tee "$OUT_DIR/builder.allocs.txt" || true
        else
          ssh -o ConnectTimeout=10 "root@$TARGET_HOST" "su - ploy -c '/opt/hashicorp/bin/nomad-job-manager.sh allocs --job $BJ --format human'" || true
        fi
      fi
    fi
  fi

  # App job allocs + alloc status + task logs (prefer provided LANE; else scan)
  if [[ -n "$LANE" ]]; then
    L_LOWER=$(printf '%s' "$LANE" | tr '[:upper:]' '[:lower:]')
    CANDIDATES=("$APP_NAME-lane-$L_LOWER")
  else
    CANDIDATES=("$APP_NAME-lane-e" "$APP_NAME-lane-c" "$APP_NAME-lane-a" "$APP_NAME-lane-b" "$APP_NAME-lane-d" "$APP_NAME-lane-f" "$APP_NAME-lane-g")
  fi
  for JOB in "${CANDIDATES[@]}"; do
    echo "== SSH: app job allocs ($JOB)" >&2
    ssh -o ConnectTimeout=10 "root@$TARGET_HOST" "su - ploy -c '/opt/hashicorp/bin/nomad-job-manager.sh allocs --job $JOB --format human'" || true
    ALLOC_ID=$(ssh -o ConnectTimeout=10 "root@$TARGET_HOST" "su - ploy -c '/opt/hashicorp/bin/nomad-job-manager.sh running-alloc --job $JOB'" 2>/dev/null | tail -n1 || true)
    if [[ -n "$ALLOC_ID" ]]; then
      echo "== SSH: app alloc status ($ALLOC_ID)" >&2
      ssh -o ConnectTimeout=10 "root@$TARGET_HOST" "su - ploy -c '/opt/hashicorp/bin/nomad-job-manager.sh alloc-status --alloc-id $ALLOC_ID'" || true
      echo "== SSH: app task logs (oci-kontain stdout+stderr)" >&2
      if [[ -n "$OUT_DIR" ]]; then
        ssh -o ConnectTimeout=10 "root@$TARGET_HOST" "su - ploy -c '/opt/hashicorp/bin/nomad-job-manager.sh logs --alloc-id $ALLOC_ID --task oci-kontain --both --lines $LINES'" | tee "$OUT_DIR/app-$JOB.oci-kontain.log" || true
      else
        ssh -o ConnectTimeout=10 "root@$TARGET_HOST" "su - ploy -c '/opt/hashicorp/bin/nomad-job-manager.sh logs --alloc-id $ALLOC_ID --task oci-kontain --both --lines $LINES'" || true
      fi
      break
    fi
  done
fi

echo "Done." >&2
