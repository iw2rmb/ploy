#!/usr/bin/env bash
set -euo pipefail

# Fetch logs to inspect issues for an app, plus platform logs; optionally builder job logs via SSH
# Usage:
#   APP_NAME=<name> [LANE=<A-G>] [SHA=<sha12>] [LINES=200] [TARGET_HOST=ip] ./tests/e2e/deploy/fetch-logs.sh

APP_NAME=${APP_NAME:-}
LANE=${LANE:-}
SHA=${SHA:-}
LINES=${LINES:-200}

if [[ -z "$APP_NAME" ]]; then echo "APP_NAME required" >&2; exit 2; fi

PC=${PLOY_CONTROLLER%/}
if [[ -n "${PC:-}" ]]; then
  echo "== App status" >&2
  curl -sf "$PC/apps/$APP_NAME/status" || true; echo
  echo "== App logs ($LINES)" >&2
  curl -sf "$PC/apps/$APP_NAME/logs?lines=$LINES" || true; echo
  echo "== Platform API logs ($LINES)" >&2
  curl -sf "$PC/platform/api/logs?lines=$LINES" || true; echo
  echo "== Traefik logs ($LINES)" >&2
  curl -sf "$PC/platform/traefik/logs?lines=$LINES" || true; echo
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
        ssh -o ConnectTimeout=10 "root@$TARGET_HOST" "su - ploy -c '/opt/hashicorp/bin/nomad-job-manager.sh logs --alloc-id $ALLOC_ID --lines $LINES'" || true
      else
        # Fallback: list allocs in human form
        ssh -o ConnectTimeout=10 "root@$TARGET_HOST" "su - ploy -c '/opt/hashicorp/bin/nomad-job-manager.sh allocs --job $BJ --format human'" || true
      fi
    fi
  fi
fi

echo "Done." >&2
