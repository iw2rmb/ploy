#!/usr/bin/env bash
set -euo pipefail

# OpenRewrite E2E via Grid HTTP API (curl)
#
# Submits the key Mods stages (mods-plan, orw-apply) directly to Grid's
# Workflow RPC using curl, then polls for bounded status. This mirrors the
# simple-openrewrite scenario without relying on the ploy runner.
#
# Requirements (env):
#   - GRID_ID / PLOY_GRID_ID
#   - GRID_API_KEY / PLOY_GRID_API_KEY
#   - PLOY_E2E_TENANT (defaults to acme)
# Optional:
#   - GRID_API_IP (IP used with --resolve for ${GRID_ID}.grid)
#   - E2E_STAGE_TIMEOUT (default 180s)
#
# Notes:
# - Uses --insecure to skip TLS validation and --resolve to target a known node.
# - Cancels any run that does not leave queued/running within the timebox.

TENANT=${PLOY_E2E_TENANT:-${TENANT:-acme}}
GRID_ID=${PLOY_GRID_ID:-${GRID_ID:-}}
GRID_API_KEY=${PLOY_GRID_API_KEY:-${GRID_API_KEY:-}}
GRID_API_IP=${GRID_API_IP:-}
API_BASE="https://${GRID_ID}.grid"
TIMEBOX=${E2E_STAGE_TIMEOUT:-180}
WORKFLOW_ID="smoke"
TICKET=${TICKET:-ticket-orw-$(date +%s)}

if [[ -z "${GRID_ID}" || -z "${GRID_API_KEY}" ]]; then
  echo "error: GRID_ID/GRID_API_KEY (or PLOY_*) must be set" >&2
  exit 1
fi

curl_args=(-sS -k -H "Authorization: Bearer ${GRID_API_KEY}" -H "Content-Type: application/json")
if [[ -n "${GRID_API_IP}" ]]; then
  curl_args+=(--resolve "${GRID_ID}.grid:443:${GRID_API_IP}")
fi

submit_stage() {
  local stage_name="$1"; shift
  local lane="$1"; shift
  local image="$1"; shift
  local -a cmd=("$@")

  local idem_key="${TICKET}:${stage_name}"
  local payload
  payload=$(cat <<JSON
{
  "tenant": "${TENANT}",
  "workflow_id": "${WORKFLOW_ID}",
  "idempotency_key": "${idem_key}",
  "labels": {
    "stage": "${stage_name}",
    "lane": "${lane}",
    "ticket_id": "${TICKET}",
    "manifest_name": "smoke",
    "manifest_version": "2025-09-26"
  },
  "job": {
    "image": "${image}",
    "command": [$(printf '"%s",' "${cmd[@]}" | sed 's/,$//')],
    "env": {},
    "resources": {},
    "metadata": {
      "lane": "${lane}",
      "priority": "standard",
      "resources.cpu": "2000m",
      "resources.memory": "4Gi",
      "manifest_name": "smoke",
      "manifest_version": "2025-09-26"
    }
  }
}
JSON
)

  echo "==> Submit ${stage_name} (lane=${lane})"
  local resp
  if ! resp=$(curl "${curl_args[@]}" -X POST --data-binary @- "${API_BASE}/v1/workflows/rpc/runs" <<<"${payload}"); then
    echo "error: submit failed for ${stage_name}" >&2
    return 1
  fi
  echo "    submit response: ${resp}" | sed -e 's/\s\+/ /g'
  local run_id
  run_id=$(printf "%s" "${resp}" | awk -F '"run_id":"' '{ if (NF>1) { split($2,a,"\""); print a[1]; } }')
  if [[ -z "${run_id}" ]]; then
    echo "error: could not parse run_id for ${stage_name}" >&2
    return 1
  fi
  poll_status "${run_id}" "${stage_name}"
}

poll_status() {
  local run_id="$1"; shift
  local label="$1"; shift
  echo "==> Poll ${label} (run=${run_id})"
  local start_ts=$(date +%s)
  local status
  while true; do
    local meta
    if ! meta=$(curl "${curl_args[@]}" -X GET "${API_BASE}/v1/workflows/rpc/runs/${run_id}?tenant=${TENANT}"); then
      echo "error: status fetch failed for ${run_id}" >&2
      break
    fi
    status=$(printf "%s" "${meta}" | awk -F '"status":"' '{ if (NF>1) { split($2,a,"\""); print a[1]; } }')
    printf "    status=%s\n" "${status}"
    if [[ "${status}" == "succeeded" || "${status}" == "failed" || "${status}" == "canceled" ]]; then
      return 0
    fi
    local now=$(date +%s)
    if (( now - start_ts > TIMEBOX )); then
      echo "    timeout waiting for ${label} to complete (status=${status}); canceling"
      cancel_run "${run_id}"
      return 1
    fi
    sleep 5
  done
}

cancel_run() {
  local run_id="$1"
  curl "${curl_args[@]}" -X POST --data-binary '{"reason":"e2e-timeout"}' "${API_BASE}/v1/workflows/rpc/runs/${run_id}:cancel?tenant=${TENANT}" >/dev/null || true
}

# Stages to execute (min viable ORW path)
submit_stage "mods-plan" "mods-plan" "registry.dev/ploy/mods-plan:latest" "mods-plan" "--run" || true
submit_stage "orw-apply" "mods-java" "registry.dev/ploy/mods-openrewrite:latest" "mods-orw" "--apply" || true

echo "==> Done (API)"
