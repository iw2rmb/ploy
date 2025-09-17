#!/usr/bin/env bash
set -euo pipefail

# Java 17 NoJib cycle helper
# - Push (async) via controller
# - Fetch enriched builder logs (docker_image, push_verify)
# - Inspect rendered HCL image line via SSH (optional)
# - Long-poll health snapshot via controller

# Env vars:
#   APP               - app name (required)
#   REPO_DIR          - path to repo to push (default: current dir)
#   PLOY_CONTROLLER   - controller base URL (e.g., https://api.dev.ployman.app/v1) [required]
#   PLOY_CMD          - path to ploy CLI (default: builds one at ./bin/ploy)
#   TARGET_HOST       - VPS host for HCL/alloc inspection (optional)
#   LINES             - log lines for builder logs (default: 400)
#   TLS_INSECURE      - if '1', sends curl with -k

err() { echo "[ERR] $*" >&2; }
info() { echo "[INFO] $*"; }
need() { command -v "$1" >/dev/null 2>&1 || { err "missing dependency: $1"; exit 2; }; }

APP=${APP:-}
if [[ -z ${APP} ]]; then err "APP is required"; exit 2; fi
PC=${PLOY_CONTROLLER:-}
if [[ -z ${PC} ]]; then err "PLOY_CONTROLLER is required"; exit 2; fi

REPO_DIR=${REPO_DIR:-.}
LINES=${LINES:-400}
INSECURE=${TLS_INSECURE:-0}
INSECURE_FLAG=""
[[ "${INSECURE}" == "1" ]] && INSECURE_FLAG="-k"

need jq

# Build or resolve CLI
PLOY_CMD=${PLOY_CMD:-}
if [[ -z "${PLOY_CMD}" ]]; then
  if [[ -x ./bin/ploy ]]; then
    PLOY_CMD=./bin/ploy
  else
    info "building ploy CLI"
    mkdir -p bin
    GOCACHE=$(mktemp -d) go build -o ./bin/ploy ./cmd/ploy
    PLOY_CMD=./bin/ploy
  fi
fi

if [[ ! -x "${PLOY_CMD}" ]]; then err "ploy CLI not executable at ${PLOY_CMD}"; exit 2; fi

push_async() {
  local app=$1
  local lane=${2:-E}
  local sha
  sha=$(date +%Y%m%d-%H%M%S)
  info "pushing app=${app} lane=${lane} sha=${sha}"
  (cd "${REPO_DIR}" && PLOY_ASYNC=1 PLOY_TLS_INSECURE=${TLS_INSECURE:-0} "${PLOY_CMD}" push -a "${app}" -lane "${lane}" -sha "${sha}" -main "") | tee /tmp/ploy-push.out >/dev/null
  local id
  id=$(sed -n 's/.*"id"\s*:\s*"\([^"]\+\)".*/\1/p' /tmp/ploy-push.out | tail -n1)
  if [[ -z ${id} ]]; then err "failed to capture build id (async not accepted)"; return 1; fi
  echo "${id}"
}

fetch_builder_logs() {
  local app=$1 id=$2
  info "fetching builder logs: app=${app} id=${id}"
  local url="${PC%/}/apps/${app}/builds/${id}/logs?lines=${LINES}"
  curl -sS ${INSECURE_FLAG} "${url}" | tee ./builder.logs.json >/dev/null
  if [[ ! -s ./builder.logs.json ]]; then err "no logs payload returned"; return 1; fi
  local docker_image
  docker_image=$(jq -r 'try (.docker_image // empty)' ./builder.logs.json)
  local digest
  digest=$(jq -r 'try (.push_verify.digest // empty)' ./builder.logs.json)
  info "docker_image: ${docker_image:-<empty>}"
  info "push_verify.digest: ${digest:-<empty>}"
}

inspect_hcl_image() {
  local app=$1
  local host=${TARGET_HOST:-}
  if [[ -z ${host} ]]; then info "TARGET_HOST not set; skipping HCL inspection"; return 0; fi
  info "inspecting rendered HCL image on ${host}"
  local file
  file=$(ssh -o ConnectTimeout=10 "root@${host}" "ls -1 /opt/ploy/debug/jobs | grep '^${app}-lane-e.*\\.hcl$' | sort | tail -n 1" || true)
  if [[ -z ${file} ]]; then err "no rendered HCL found for ${app}"; return 1; fi
  info "rendered HCL: /opt/ploy/debug/jobs/${file}"
  ssh -o ConnectTimeout=10 "root@${host}" "sed -n '1,160p' /opt/ploy/debug/jobs/${file} | grep -En '^\s*image\s*=|DOCKER_IMAGE|docker_image'" || true
}

watch_health() {
  local app=$1
  local url="${PC%/}/apps/${app}/status/watch?wait=30s&timeout=30s"
  info "watching health snapshot (30s)"
  curl -sS ${INSECURE_FLAG} "${url}" | tee ./health.watch.json >/dev/null || true
}

main() {
  local id
  id=$(push_async "${APP}" "E") || exit 1
  info "build id: ${id}"
  fetch_builder_logs "${APP}" "${id}" || true
  inspect_hcl_image "${APP}" || true
  watch_health "${APP}" || true
  info "done. review: builder.logs.json and health.watch.json"
}

main "$@"

