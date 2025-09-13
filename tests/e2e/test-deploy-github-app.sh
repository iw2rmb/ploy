#!/usr/bin/env bash
# E2E: Deploy a GitHub app with ploy CLI, verify HTTPS, then destroy
# Requirements (VPS Dev API): set PLOY_CONTROLLER=https://api.dev.ployman.app/v1

set -euo pipefail

BLUE='\033[0;34m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; RED='\033[0;31m'; NC='\033[0m'
info(){ echo -e "${BLUE}$*${NC}"; }
ok(){ echo -e "${GREEN}$*${NC}"; }
warn(){ echo -e "${YELLOW}$*${NC}"; }
err(){ echo -e "${RED}$*${NC}"; }

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/../.." && pwd)
REPO_URL=${HELLO_APP_REPO:-}
APP_NAME=${APP_NAME:-}
BRANCH=${BRANCH:-main}
ENV_NAME=${ENV_NAME:-dev}

if [[ -z "$REPO_URL" ]]; then
  err "HELLO_APP_REPO is required (e.g., https://github.com/iw2rmb/ploy-scala-hello.git)"
  exit 1
fi

if [[ -z "$APP_NAME" ]]; then
  # Derive from repo name
  APP_NAME=$(basename -s .git "$REPO_URL" | tr 'A-Z' 'a-z' | tr -c 'a-z0-9-' '-')
fi

if [[ -z "${PLOY_CONTROLLER:-}" ]]; then
  warn "PLOY_CONTROLLER not set; defaulting to https://api.dev.ployman.app/v1"
  export PLOY_CONTROLLER=https://api.dev.ployman.app/v1
fi

WORKDIR=$(mktemp -d)
trap 'rm -rf "$WORKDIR"' EXIT

info "Cloning $REPO_URL (branch: $BRANCH)"
git clone --depth 1 --branch "$BRANCH" "$REPO_URL" "$WORKDIR/app"

pushd "$WORKDIR/app" >/dev/null
info "Deploying app with ploy push: app=$APP_NAME"
PLOY_CMD=${PLOY_CMD:-}
if [[ -z "$PLOY_CMD" ]]; then
  if command -v ploy >/dev/null 2>&1; then
    PLOY_CMD=ploy
  elif [[ -x "$REPO_ROOT/bin/ploy" ]]; then
    PLOY_CMD="$REPO_ROOT/bin/ploy"
  else
    PLOY_CMD=ploy
  fi
fi
EXTRA_FLAGS=()
[[ -n "${LANE:-}" ]] && EXTRA_FLAGS+=("-lane" "$LANE")
[[ -n "${MAIN:-}" ]] && EXTRA_FLAGS+=("-main" "$MAIN")
START_TS=$(date +%s)
attempt_push() {
  local out rc
  out=$("$PLOY_CMD" push -a "$APP_NAME" "${EXTRA_FLAGS[@]:-}" 2>&1)
  rc=$?
  echo "$out"
  if [[ $rc -ne 0 ]] || echo "$out" | rg -qi '("error"\s*:|failed|^❌)'; then
    return 1
  fi
  return 0
}

if ! attempt_push; then
  warn "First push attempt failed; retrying once in 5s..."
  sleep 5
  if ! attempt_push; then
    err "ploy push reported failure"
    # Show logs for diagnostics
    if command -v jq >/dev/null 2>&1; then
      APP_NAME="$APP_NAME" PLOY_CONTROLLER="$PLOY_CONTROLLER" "${REPO_ROOT}/tests/lanes/check-app-logs.sh" || true
    fi
    exit 1
  fi
fi
ok "ploy push triggered"

# Determine expected URL
# Prefer preview router using commit SHA to trigger run, else allow override
GIT_SHA=$(git rev-parse --short=12 HEAD 2>/dev/null || echo "")
URL_OVERRIDE=${URL_OVERRIDE:-}
if [[ -n "$URL_OVERRIDE" ]]; then
  URL="$URL_OVERRIDE"
elif [[ -n "$GIT_SHA" ]]; then
  DOMAIN_SUFFIX="ployd.app"
  if [[ "${ENV_NAME}" == "dev" ]]; then
    DOMAIN_SUFFIX="dev.ployd.app"
  fi
  URL="https://${GIT_SHA}.${APP_NAME}.${DOMAIN_SUFFIX}"
else
  URL="https://${APP_NAME}.ployd.app"
fi

HEALTH_PATH=${HEALTH_PATH:-/healthz}
TIMEOUT=${TIMEOUT:-300}
SLEEP=${SLEEP:-5}
# Adjust remaining time budget after push
NOW_TS=$(date +%s)
SPENT=$((NOW_TS - START_TS))
REMAIN=$((TIMEOUT - SPENT))
if (( REMAIN <= 0 )); then
  err "No time left after push (spent ${SPENT}s of ${TIMEOUT}s)"
  exit 1
fi
ELAPSED=0
info "Waiting for app health at ${URL}${HEALTH_PATH} (timeout ${TIMEOUT}s)"
set +e
while (( ELAPSED < REMAIN )); do
  if curl -sf "${URL}${HEALTH_PATH}" >/dev/null; then
    ok "App is responding over HTTPS: ${URL}${HEALTH_PATH}"
    READY=1; break
  fi
  sleep "$SLEEP"; ELAPSED=$((ELAPSED + SLEEP))
done
set -e

if [[ -z "${READY:-}" ]]; then
  err "App failed to become healthy within ${REMAIN}s (total ${TIMEOUT}s)"
  # Fetch logs for diagnostics when available
  if command -v jq >/dev/null 2>&1; then
    APP_NAME="$APP_NAME" PLOY_CONTROLLER="$PLOY_CONTROLLER" "${REPO_ROOT}/tests/lanes/check-app-logs.sh" || true
  else
    echo "Tip: install jq for prettier logs" >&2
  fi
  exit 1
fi

popd >/dev/null

info "Destroying app via ploy apps destroy --name $APP_NAME --force"
ploy apps destroy --name "$APP_NAME" --force
ok "Destroy request sent"

info "Verifying app status returns 404"
STATUS_CODE=$(curl -s -o /dev/null -w "%{http_code}" "${PLOY_CONTROLLER%/}/apps/${APP_NAME}/status")
if [[ "$STATUS_CODE" == "404" ]]; then
  ok "App status is 404 after destroy"
else
  warn "Expected 404; got $STATUS_CODE"
fi

ok "E2E complete for $APP_NAME from $REPO_URL"
