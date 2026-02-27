#!/usr/bin/env bash
set -euo pipefail

# E2E: Prep lifecycle happy path.
# Validates:
# - PrepPending -> PrepRunning -> PrepReady
# - Jobs are not materialized before PrepReady

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
export PLOY_CONFIG_HOME="${PLOY_CONFIG_HOME:-$REPO_ROOT/deploy/local/cli}"
source "$REPO_ROOT/tests/e2e/lib/ensure_local_descriptor.sh"
ensure_local_descriptor "$REPO_ROOT" "$PLOY_CONFIG_HOME"

TS=$(date +%y%m%d%H%M%S)
ARTIFACT_BASE=${PLOY_E2E_ARTIFACT_BASE:-./tmp/migs/prep-ready}
ARTIFACT_DIR=${PLOY_E2E_ARTIFACT_DIR:-${ARTIFACT_BASE}/${TS}}
mkdir -p "${ARTIFACT_DIR}"

REPO_URL=${PLOY_E2E_REPO_OVERRIDE:-https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git}
BASE_REF=${PLOY_E2E_BASE_REF:-main}
TARGET_REF=${PLOY_E2E_TARGET_REF:-main}

resolve_descriptor_path() {
  local marker="${PLOY_CONFIG_HOME}/clusters/default"
  local clusters_dir="${PLOY_CONFIG_HOME}/clusters"
  if [[ -L "$marker" ]]; then
    local target
    target="$(readlink "$marker")"
    if [[ "$target" = /* ]]; then
      printf '%s\n' "$target"
      return
    fi
    printf '%s\n' "${clusters_dir}/${target}"
    return
  fi
  printf '%s\n' "$marker"
}

DESCRIPTOR_PATH="$(resolve_descriptor_path)"
SERVER_URL="$(jq -r '.address // empty' "$DESCRIPTOR_PATH")"
API_TOKEN="$(jq -r '.token // empty' "$DESCRIPTOR_PATH")"
if [[ -z "$SERVER_URL" || -z "$API_TOKEN" ]]; then
  echo "error: failed to resolve server address/token from ${DESCRIPTOR_PATH}" >&2
  exit 1
fi

api_get() {
  local path="$1"
  curl -fsS \
    -H "Authorization: Bearer ${API_TOKEN}" \
    "${SERVER_URL}${path}"
}

RUN_ID=$("$REPO_ROOT/dist/ploy" mig run --json \
  --repo-url "$REPO_URL" \
  --repo-base-ref "$BASE_REF" \
  --repo-target-ref "$TARGET_REF" \
  --job-image alpine:3.20 \
  --job-command 'echo "[prep-ready] step start"; sleep 1; echo "[prep-ready] step done"' \
  | tee "${ARTIFACT_DIR}/run-submit.json" | jq -r '.run_id')

if [[ -z "${RUN_ID:-}" || "$RUN_ID" == "null" ]]; then
  echo "error: run_id is empty" >&2
  exit 1
fi

REPO_ID=""
for _ in $(seq 1 30); do
  REPO_ID="$("$REPO_ROOT/dist/ploy" run status --json "$RUN_ID" \
    | jq -r '.repos[0].repo_id // empty')"
  if [[ -n "$REPO_ID" ]]; then
    break
  fi
  sleep 1
done

if [[ -z "$REPO_ID" ]]; then
  echo "error: failed to resolve repo_id for run ${RUN_ID}" >&2
  exit 1
fi

PREP_JSON="$(api_get "/v1/repos/${REPO_ID}/prep" | tee "${ARTIFACT_DIR}/prep-initial.json")"
INITIAL_STATUS="$(printf '%s' "$PREP_JSON" | jq -r '.prep_status')"
if [[ "$INITIAL_STATUS" != "PrepPending" ]]; then
  echo "error: expected initial prep_status=PrepPending, got=${INITIAL_STATUS}" >&2
  exit 1
fi

seen_running=0
prep_status="$INITIAL_STATUS"
deadline=$((SECONDS + 180))
: > "${ARTIFACT_DIR}/prep-status.log"
while (( SECONDS < deadline )); do
  PREP_JSON="$(api_get "/v1/repos/${REPO_ID}/prep")"
  JOBS_JSON="$(api_get "/v1/runs/${RUN_ID}/repos/${REPO_ID}/jobs")"

  prep_status="$(printf '%s' "$PREP_JSON" | jq -r '.prep_status')"
  jobs_count="$(printf '%s' "$JOBS_JSON" | jq '.jobs | length')"
  printf '%s prep_status=%s jobs=%s\n' "$(date -u +%FT%TZ)" "$prep_status" "$jobs_count" >> "${ARTIFACT_DIR}/prep-status.log"

  if [[ "$prep_status" != "PrepReady" ]] && (( jobs_count > 0 )); then
    echo "error: jobs were created before prep reached PrepReady (status=${prep_status}, jobs=${jobs_count})" >&2
    exit 1
  fi

  case "$prep_status" in
    PrepPending)
      ;;
    PrepRunning)
      seen_running=1
      ;;
    PrepReady)
      break
      ;;
    PrepFailed|PrepRetryScheduled)
      echo "error: unexpected prep status during happy path: ${prep_status}" >&2
      exit 1
      ;;
    *)
      echo "error: unknown prep status: ${prep_status}" >&2
      exit 1
      ;;
  esac
  sleep 1
done

if [[ "$prep_status" != "PrepReady" ]]; then
  echo "error: timed out waiting for PrepReady (last_status=${prep_status})" >&2
  exit 1
fi
if (( seen_running == 0 )); then
  echo "error: expected to observe PrepRunning transition before PrepReady" >&2
  exit 1
fi

jobs_count=0
for _ in $(seq 1 60); do
  JOBS_JSON="$(api_get "/v1/runs/${RUN_ID}/repos/${REPO_ID}/jobs")"
  jobs_count="$(printf '%s' "$JOBS_JSON" | jq '.jobs | length')"
  if (( jobs_count > 0 )); then
    printf '%s\n' "$JOBS_JSON" > "${ARTIFACT_DIR}/jobs-after-ready.json"
    break
  fi
  sleep 1
done

if (( jobs_count == 0 )); then
  echo "error: expected jobs to be created after PrepReady" >&2
  exit 1
fi

"$REPO_ROOT/dist/ploy" run status "$RUN_ID" > "${ARTIFACT_DIR}/run-status.txt" 2>&1 || true
echo "OK: prep-ready scenario"
echo "Run: ${RUN_ID}"
echo "Repo: ${REPO_ID}"
echo "Artifacts saved to: ${ARTIFACT_DIR}"
