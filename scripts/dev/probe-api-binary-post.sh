#!/usr/bin/env bash
set -euo pipefail

# Probes binary-bodied POST behavior against the Dev API controller.
# Sends small tar payload via application/x-tar and multipart/form-data, and runs a JSON probe.
#
# Usage:
#   scripts/dev/probe-api-binary-post.sh
#
# Env:
#   BASE_URL   Base API URL (default: https://api.dev.ployman.app)
#   APP_NAME   App name for path construction (default: probe-hello)
#   LANE       Target lane (default: E)
#   ENV_NAME   Target env (default: dev)

BASE_URL="${BASE_URL:-https://api.dev.ployman.app}"
APP_NAME="${APP_NAME:-probe-hello}"
LANE="${LANE:-E}"
ENV_NAME="${ENV_NAME:-dev}"

WORKDIR="$(mktemp -d)"
trap 'rm -rf "${WORKDIR}"' EXIT

echo "Working in ${WORKDIR}"
echo "Target: ${BASE_URL} (app=${APP_NAME} lane=${LANE} env=${ENV_NAME})"

# Create a tiny tarball
echo "hello" > "${WORKDIR}/file.txt"
tar -cf "${WORKDIR}/small.tar" -C "${WORKDIR}" file.txt
ls -l "${WORKDIR}/small.tar"

post_binary() {
  local url="$1"; shift
  echo "\n==> POST (binary) to: ${url}" >&2
  curl --http2 -sS -o /dev/null -w "HTTP:%{http_code} TIME:%{time_total} SIZE:%{size_upload}\n" \
    -X POST -H "Content-Type: application/x-tar" \
    --data-binary @"${WORKDIR}/small.tar" \
    "$url" || true
}

post_multipart() {
  local url="$1"; shift
  echo "\n==> POST (multipart) to: ${url}" >&2
  curl --http2 -sS -o /dev/null -w "HTTP:%{http_code} TIME:%{time_total} SIZE:%{size_upload}\n" \
    -X POST -F "file=@${WORKDIR}/small.tar;type=application/x-tar" \
    "$url" || true
}

post_json_probe() {
  local url="$1"; shift
  echo "\n==> POST (json probe) to: ${url}" >&2
  curl --http2 -sS -o /dev/null -w "HTTP:%{http_code} TIME:%{time_total}\n" \
    -X POST -H "Content-Type: application/json" \
    -d '{"ping":"pong"}' \
    "$url" || true
}

UPLOAD_URL="${BASE_URL}/v1/apps/${APP_NAME}/upload?lane=${LANE}&env=${ENV_NAME}&debug=true"
BUILDS_URL="${BASE_URL}/v1/apps/${APP_NAME}/builds?lane=${LANE}&env=${ENV_NAME}&debug=true"
PROBE_URL="${BASE_URL}/v1/apps/${APP_NAME}/builds/probe?lane=${LANE}&env=${ENV_NAME}"

post_binary  "$UPLOAD_URL"
post_multipart "$UPLOAD_URL"

post_binary  "$BUILDS_URL"
post_multipart "$BUILDS_URL"

post_json_probe "$PROBE_URL"

echo "\nDone."

