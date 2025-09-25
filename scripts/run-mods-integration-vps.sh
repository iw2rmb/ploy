#!/usr/bin/env bash
set -euo pipefail

readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
readonly JOB_TEMPLATE="${PROJECT_ROOT}/tests/nomad-jobs/mods-integration.nomad.hcl"

log() {
  printf '[%s] %s\n' "$(date '+%Y-%m-%d %H:%M:%S')" "$*"
}

require_env() {
  local name="$1"
  if [[ -z "${!name:-}" ]]; then
    printf 'Missing required environment variable: %s\n' "$name" >&2
    exit 1
  fi
}

if [[ ! -f "${JOB_TEMPLATE}" ]]; then
  printf 'Nomad job template missing at %s\n' "${JOB_TEMPLATE}" >&2
  exit 1
fi

require_env TARGET_HOST
require_env PLOY_CONTROLLER
require_env PLOY_SEAWEEDFS_URL
require_env PLOY_GITLAB_PAT

if [[ -z "${GITHUB_PLOY_DEV_USERNAME:-}" || -z "${GITHUB_PLOY_DEV_PAT:-}" ]]; then
  printf 'GITHUB_PLOY_DEV_USERNAME and GITHUB_PLOY_DEV_PAT must be set for repository access.\n' >&2
  exit 1
fi

export MODS_INTEGRATION_JOB_NAME="${MODS_INTEGRATION_JOB_NAME:-mods-integration-tests}"
export MODS_INTEGRATION_DC="${MODS_INTEGRATION_DC:-${NOMAD_DC:-dc1}}"
export MODS_INTEGRATION_IMAGE="${MODS_INTEGRATION_IMAGE:-registry.dev.ployman.app/library/golang:1.24}"
export MODS_INTEGRATION_WORKDIR="${MODS_INTEGRATION_WORKDIR:-/workspace/mods}"
export MODS_INTEGRATION_REPO="${MODS_INTEGRATION_REPO:-https://github.com/iw2rmb/ploy.git}"
export MODS_INTEGRATION_REF="${MODS_INTEGRATION_REF:-main}"
export MODS_INTEGRATION_SHA="${MODS_INTEGRATION_SHA:-}"
export MODS_INTEGRATION_TIMEOUT="${MODS_INTEGRATION_TIMEOUT:-30m}"
export PLOY_GITLAB_PAT="${PLOY_GITLAB_PAT}"
export MODS_INTEGRATION_CPU="${MODS_INTEGRATION_CPU:-2000}"
export MODS_INTEGRATION_MEMORY="${MODS_INTEGRATION_MEMORY:-4096}"
export MODS_INTEGRATION_WAIT="${MODS_INTEGRATION_WAIT:-600}"
export MODS_INTEGRATION_LOG_LINES="${MODS_INTEGRATION_LOG_LINES:-400}"
export GOFLAGS="${GOFLAGS:--mod=readonly}"
export MODS_SEAWEED_MASTER="${MODS_SEAWEED_MASTER:-seaweedfs-master.storage.ploy.local:9333}"
export MODS_SEAWEED_FALLBACKS="${MODS_SEAWEED_FALLBACKS:-}"
export MODS_ALLOW_PARTIAL_ORW="${MODS_ALLOW_PARTIAL_ORW:-false}"
export MODS_REGISTRY="${MODS_REGISTRY:-registry.dev.ployman.app}"
export MODS_PLANNER_IMAGE="${MODS_PLANNER_IMAGE:-registry.dev.ployman.app/langgraph-runner:latest}"
export MODS_REDUCER_IMAGE="${MODS_REDUCER_IMAGE:-$MODS_PLANNER_IMAGE}"
export MODS_LLM_EXEC_IMAGE="${MODS_LLM_EXEC_IMAGE:-$MODS_PLANNER_IMAGE}"
export MODS_ORW_APPLY_IMAGE="${MODS_ORW_APPLY_IMAGE:-registry.dev.ployman.app/openrewrite-jvm:latest}"
export NOMAD_ADDR="${NOMAD_ADDR:-http://nomad.control.ploy.local:4646}"
export CONSUL_HTTP_ADDR="${CONSUL_HTTP_ADDR:-http://consul.service.consul:8500}"
export SEAWEEDFS_FILER="${SEAWEEDFS_FILER:-http://seaweedfs-filer.storage.ploy.local:8888}"
export SEAWEEDFS_MASTER="${SEAWEEDFS_MASTER:-seaweedfs-master.storage.ploy.local:9333}"
export SEAWEEDFS_COLLECTION="${SEAWEEDFS_COLLECTION:-artifacts}"
export TARGET_HOST="${TARGET_HOST}"

log "Rendering Nomad job for ${MODS_INTEGRATION_JOB_NAME}"
TMP_SPEC=$(mktemp "/tmp/${MODS_INTEGRATION_JOB_NAME}.XXXXXX.hcl")
trap 'rm -f "${TMP_SPEC}"' EXIT

envsubst < "${JOB_TEMPLATE}" > "${TMP_SPEC}"

REMOTE_SPEC="/tmp/${MODS_INTEGRATION_JOB_NAME}.nomad.hcl"
log "Uploading rendered job to ${TARGET_HOST}:${REMOTE_SPEC}"
scp -o ConnectTimeout=10 "${TMP_SPEC}" "root@${TARGET_HOST}:${REMOTE_SPEC}" >/dev/null
ssh -o ConnectTimeout=10 "root@${TARGET_HOST}" "chown ploy:ploy ${REMOTE_SPEC}"

log "Validating Nomad job"
ssh -o ConnectTimeout=10 "root@${TARGET_HOST}" \
  "su - ploy -c '/opt/hashicorp/bin/nomad-job-manager.sh validate --file ${REMOTE_SPEC}'"

log "Stopping any existing job with the same name"
ssh -o ConnectTimeout=10 "root@${TARGET_HOST}" \
  "su - ploy -c '/opt/hashicorp/bin/nomad-job-manager.sh stop --job ${MODS_INTEGRATION_JOB_NAME}'" >/dev/null || true

log "Submitting job ${MODS_INTEGRATION_JOB_NAME}"
ssh -o ConnectTimeout=10 "root@${TARGET_HOST}" \
  "su - ploy -c '/opt/hashicorp/bin/nomad-job-manager.sh run --job ${MODS_INTEGRATION_JOB_NAME} --file ${REMOTE_SPEC}'"

log "Waiting up to ${MODS_INTEGRATION_WAIT}s for allocations"
ssh -o ConnectTimeout=10 "root@${TARGET_HOST}" \
  "su - ploy -c '/opt/hashicorp/bin/nomad-job-manager.sh wait --job ${MODS_INTEGRATION_JOB_NAME} --timeout ${MODS_INTEGRATION_WAIT}'" >/dev/null || true

log "Fetching allocation summary"
alloc_json=$(ssh -o ConnectTimeout=10 "root@${TARGET_HOST}" \
  "su - ploy -c '/opt/hashicorp/bin/nomad-job-manager.sh allocs --job ${MODS_INTEGRATION_JOB_NAME} --format json'" 2>/dev/null || true)

ALLOC_ID=""
CLIENT_STATUS=""
TASK_STATUS=""
EXIT_CODE=""
if [[ -n "${alloc_json}" ]]; then
  alloc_summary=$(printf '%s' "${alloc_json}" | python3 - <<'PY'
import json, sys
try:
    data = json.load(sys.stdin)
except json.JSONDecodeError:
    sys.exit(0)
if not data:
    sys.exit(0)
alloc = data[-1]
state = alloc.get("TaskStates", {}).get("mods-integration", {})
print("{} {} {} {}".format(
    alloc.get("ID", ""),
    alloc.get("ClientStatus", ""),
    state.get("State", ""),
    state.get("ExitCode", "")
))
PY
  ) || true
  if [[ -n "${alloc_summary}" ]]; then
    read -r ALLOC_ID CLIENT_STATUS TASK_STATUS EXIT_CODE <<<"${alloc_summary}"
  fi
fi

if [[ -n "${ALLOC_ID}" ]]; then
  log "Streaming logs for allocation ${ALLOC_ID}"
  ssh -o ConnectTimeout=10 "root@${TARGET_HOST}" \
    "su - ploy -c '/opt/hashicorp/bin/nomad-job-manager.sh logs --alloc-id ${ALLOC_ID} --both --lines ${MODS_INTEGRATION_LOG_LINES}'" || true
else
  log "No allocation information available"
fi

if [[ -n "${CLIENT_STATUS}" ]]; then
  log "Allocation status: client=${CLIENT_STATUS} task=${TASK_STATUS} exit=${EXIT_CODE}"
  if [[ "${CLIENT_STATUS}" != "complete" ]] || [[ "${TASK_STATUS}" != "dead" ]] || [[ "${EXIT_CODE}" != "0" ]]; then
    printf '\nNomad allocation indicates failure (status=%s task=%s exit=%s)\n' "${CLIENT_STATUS}" "${TASK_STATUS}" "${EXIT_CODE}" >&2
    exit 1
  fi
else
  log "Unable to determine allocation status"
fi

log "Mods integration job completed successfully"
