#!/usr/bin/env bash
set -euo pipefail

# OpenRewrite E2E via gridctl
#
# Submits Mods stages (mods-plan, orw-apply) using gridctl, then polls status
# for a bounded duration. Mirrors the simple-openrewrite scenario without ploy.
#
# Requirements:
#   - Go toolchain (for `go run ./cmd/gridctl`)
#   - GRID_ID / PLOY_GRID_ID
#   - GRID_API_KEY / PLOY_GRID_API_KEY
# Optional:
#   - E2E_STAGE_TIMEOUT (default 180s)

GRID_ID=${PLOY_GRID_ID:-${GRID_ID:-}}
GRID_API_KEY=${PLOY_GRID_API_KEY:-${GRID_API_KEY:-}}
TIMEBOX=${E2E_STAGE_TIMEOUT:-180}
WORKFLOW_ID="smoke"
TICKET=${TICKET:-ticket-orw-$(date +%s)}

if [[ -z "${GRID_ID}" || -z "${GRID_API_KEY}" ]]; then
  echo "error: GRID_ID/GRID_API_KEY (or PLOY_*) must be set" >&2
  exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# Prefer adjacent grid repo (sibling of ploy)
if [[ -d "${SCRIPT_DIR}/../../../grid" ]]; then
  GRIDCTL_DIR="$(cd "${SCRIPT_DIR}/../../../grid" && pwd)"
else
  GRIDCTL_DIR="$(cd "${SCRIPT_DIR}/../../grid" && pwd 2>/dev/null || true)"
fi
if [[ -z "${GRIDCTL_DIR}" || ! -d "${GRIDCTL_DIR}" ]]; then
  echo "error: could not locate grid source tree next to this repo" >&2
  exit 1
fi
gridctl() {
  ( cd "${GRIDCTL_DIR}" && GRID_ID="${GRID_ID}" GRID_API_KEY="${GRID_API_KEY}" go run ./cmd/gridctl "$@" )
}

submit_stage() {
  local stage_name="$1"; shift
  local lane="$1"; shift
  local image="$1"; shift
  local -a cmd=("$@")

  echo "==> Submit ${stage_name} (lane=${lane})"
  local out
  if ! out=$(gridctl workflow submit \
      --workflow "${WORKFLOW_ID}" \
      --image "${image}" \
      $(printf -- '--cmd %q ' "${cmd[@]}") \
      --idempotency "${TICKET}:${stage_name}" \
      --label "stage=${stage_name}" \
      --label "lane=${lane}" \
      --label "ticket_id=${TICKET}" \
      --label "manifest_name=smoke" \
      --label "manifest_version=2025-09-26" \
      --label "priority=standard" \
      --label "resources.cpu=2000m" \
      --label "resources.memory=4Gi" ); then
    echo "error: submit failed for ${stage_name}" >&2
    return 1
  fi
  echo "    submit output: ${out}"
  local run_id
  run_id=$(printf "%s" "${out}" | awk '/^Run:/ {print $2; exit}')
  if [[ -z "${run_id}" ]]; then
    # fallback: try to parse JSON if printed
    run_id=$(printf "%s" "${out}" | sed -n 's/.*"run_id"[[:space:]]*:[[:space:]]*"\([^"]\+\)".*/\1/p')
  fi
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
  while true; do
    local out
    if ! out=$(gridctl workflow status "${run_id}"); then
      echo "error: status fetch failed for ${run_id}" >&2
      break
    fi
    local status
    status=$(printf "%s" "${out}" | sed -n 's/.*Status:[[:space:]]*\([^[:space:]]\+\).*/\1/p')
    printf "    status=%s\n" "${status}"
    if [[ "${status}" == "succeeded" || "${status}" == "failed" || "${status}" == "canceled" ]]; then
      return 0
    fi
    local now=$(date +%s)
    if (( now - start_ts > TIMEBOX )); then
      echo "    timeout waiting for ${label} to complete (status=${status}); canceling"
      gridctl workflow cancel "${run_id}" --reason "e2e-timeout" >/dev/null || true
      return 1
    fi
    sleep 5
  done
}

# Stages to execute (min viable ORW path)
submit_stage "mods-plan" "mods-plan" "registry.dev/ploy/mods-plan:latest" "mods-plan" "--run" || true
submit_stage "orw-apply" "mods-java" "registry.dev/ploy/mods-openrewrite:latest" "mods-orw" "--apply" || true

echo "==> Done (gridctl)"
