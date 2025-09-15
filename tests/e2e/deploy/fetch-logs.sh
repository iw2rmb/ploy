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

  # Optional: fetch app task logs by scanning lanes for running alloc
  echo "== SSH: app task logs ($APP_NAME, lanes a-g)" >&2
  for L in a b c d e f g; do
    JOB="$APP_NAME-lane-$L"
    ALLOC_ID=$(ssh -o ConnectTimeout=10 "root@$TARGET_HOST" "su - ploy -c '/opt/hashicorp/bin/nomad-job-manager.sh running-alloc --job $JOB'" 2>/dev/null || true)
    if [[ -n "$ALLOC_ID" ]]; then
      if [[ -n "$OUT_DIR" ]]; then
        ssh -o ConnectTimeout=10 "root@$TARGET_HOST" "su - ploy -c '/opt/hashicorp/bin/nomad-job-manager.sh logs --alloc-id $ALLOC_ID --both --lines $LINES'" | tee "$OUT_DIR/app-$JOB.log" || true
      else
        ssh -o ConnectTimeout=10 "root@$TARGET_HOST" "su - ploy -c '/opt/hashicorp/bin/nomad-job-manager.sh logs --alloc-id $ALLOC_ID --both --lines $LINES'" || true
      fi
      break
    fi
  done
fi

echo "Done." >&2
