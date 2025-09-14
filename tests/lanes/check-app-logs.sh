#!/usr/bin/env bash
# Retrieve app logs via API (preferred) or VPS job manager if TARGET_HOST is set.

set -euo pipefail

APP_NAME=${APP_NAME:-}
LINES=${LINES:-200}
FOLLOW=${FOLLOW:-false}

if [[ -z "$APP_NAME" ]]; then
  echo "APP_NAME is required" >&2
  exit 1
fi

if [[ -n "${PLOY_CONTROLLER:-}" ]]; then
  APP_URL="${PLOY_CONTROLLER%/}/apps/${APP_NAME}/logs?lines=${LINES}&follow=${FOLLOW}"
  echo "Fetching app logs via API: $APP_URL" >&2
  curl -sf "$APP_URL" | jq -r '.logs // .message // .error'
  # Also attempt to fetch Traefik platform logs when available (Traefik runs as Nomad job)
  TRAEFIK_URL="${PLOY_CONTROLLER%/}/platform/traefik/logs?lines=${LINES}&follow=false"
  echo "---" >&2
  echo "Fetching Traefik logs via API (if supported): $TRAEFIK_URL" >&2
  curl -sf "$TRAEFIK_URL" | jq -r '.logs // .message // .error' || true
  exit 0
fi

if [[ -n "${TARGET_HOST:-}" ]]; then
  echo "Fetching recent task logs via VPS job manager (ssh root@${TARGET_HOST})" >&2
  # Best-effort: use job-manager helper to get last alloc logs for typical task name 'web'
  # Customize as needed for lane-specific task names
  ssh -o ConnectTimeout=30 "root@${TARGET_HOST}" \
    "su - ploy -c '/opt/hashicorp/bin/nomad-job-manager.sh tail-app --app ${APP_NAME} --lines ${LINES}'"
  exit 0
fi

echo "No PLOY_CONTROLLER or TARGET_HOST set; cannot fetch logs" >&2
exit 2
