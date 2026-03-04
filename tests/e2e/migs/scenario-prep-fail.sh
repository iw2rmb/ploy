#!/usr/bin/env bash
set -euo pipefail

# E2E: Prep lifecycle failure path.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=tests/e2e/lib/harness.sh
source "${SCRIPT_DIR}/../lib/harness.sh"

e2e_init "${BASH_SOURCE[0]}"
e2e_artifacts_init "$REPO_ROOT/tmp/migs/prep-fail"

REPO_URL="${PLOY_E2E_REPO_OVERRIDE:-https://github.com/octocat/Hello-World.git}"
BASE_REF="${PLOY_E2E_BASE_REF:-prep-fail-trigger}"
TARGET_REF="${PLOY_E2E_TARGET_REF:-master}"
SPEC_FILE="${E2E_ARTIFACT_DIR}/prep-fail-spec.json"

cat > "$SPEC_FILE" <<'JSON'
{
  "version": "0.2.0",
  "env": {},
  "steps": [
    {
      "image": "alpine:3.20",
      "command": "echo \"[prep-fail] this step should stay gated\""
    }
  ]
}
JSON

RUN_ID="$("$PLOY_BIN" run --json \
  --repo "$REPO_URL" \
  --base-ref "$BASE_REF" \
  --target-ref "$TARGET_REF" \
  --spec "$SPEC_FILE" \
  | tee "${E2E_ARTIFACT_DIR}/run-submit.json" | jq -r '.run_id')"

if [[ -z "${RUN_ID:-}" || "$RUN_ID" == "null" ]]; then
  echo "error: run_id is empty" >&2
  exit 1
fi

REPO_ID=""
for _ in $(seq 1 30); do
  REPO_ID="$(e2e_api_get "/v1/runs/${RUN_ID}/repos" | jq -r '.repos[0].repo_id // empty')"
  if [[ -n "$REPO_ID" ]]; then
    break
  fi
  sleep 1
done

if [[ -z "$REPO_ID" ]]; then
  echo "error: failed to resolve repo_id for run ${RUN_ID}" >&2
  exit 1
fi

PREP_JSON="$(e2e_api_get "/v1/repos/${REPO_ID}/prep" | tee "${E2E_ARTIFACT_DIR}/prep-initial.json")"
INITIAL_STATUS="$(printf '%s' "$PREP_JSON" | jq -r '.prep_status')"
if [[ "$INITIAL_STATUS" != "PrepPending" ]]; then
  echo "error: expected initial prep_status=PrepPending, got=${INITIAL_STATUS}" >&2
  exit 1
fi

prep_status="$INITIAL_STATUS"
deadline=$((SECONDS + 180))
: > "${E2E_ARTIFACT_DIR}/prep-status.log"
while (( SECONDS < deadline )); do
  PREP_JSON="$(e2e_api_get "/v1/repos/${REPO_ID}/prep")"
  JOBS_JSON="$(e2e_api_get "/v1/runs/${RUN_ID}/repos/${REPO_ID}/jobs")"

  prep_status="$(printf '%s' "$PREP_JSON" | jq -r '.prep_status')"
  jobs_count="$(printf '%s' "$JOBS_JSON" | jq '.jobs | length')"
  printf '%s prep_status=%s jobs=%s\n' "$(date -u +%FT%TZ)" "$prep_status" "$jobs_count" >> "${E2E_ARTIFACT_DIR}/prep-status.log"

  if (( jobs_count > 0 )); then
    echo "error: jobs were created even though prep has not reached PrepReady (status=${prep_status}, jobs=${jobs_count})" >&2
    exit 1
  fi

  case "$prep_status" in
    PrepPending|PrepRunning)
      ;;
    PrepFailed)
      printf '%s\n' "$PREP_JSON" > "${E2E_ARTIFACT_DIR}/prep-final.json"
      break
      ;;
    PrepReady|PrepRetryScheduled)
      echo "error: unexpected prep status during failure scenario: ${prep_status}" >&2
      exit 1
      ;;
    *)
      echo "error: unknown prep status: ${prep_status}" >&2
      exit 1
      ;;
  esac
  sleep 1
done

if [[ "$prep_status" != "PrepFailed" ]]; then
  echo "error: timed out waiting for PrepFailed (last_status=${prep_status})" >&2
  exit 1
fi

FAILURE_CODE="$(printf '%s' "$PREP_JSON" | jq -r '.prep_failure_code // empty')"
if [[ -z "$FAILURE_CODE" ]]; then
  echo "error: expected prep_failure_code to be set for PrepFailed" >&2
  exit 1
fi

RUN_ROWS="$(printf '%s' "$PREP_JSON" | jq '.runs | length')"
if (( RUN_ROWS < 1 )); then
  echo "error: expected prep runs evidence rows for PrepFailed" >&2
  exit 1
fi
LATEST_RUN_STATUS="$(printf '%s' "$PREP_JSON" | jq -r '.runs[0].status // empty')"
if [[ "$LATEST_RUN_STATUS" != "PrepFailed" ]]; then
  echo "error: expected latest prep run status PrepFailed, got=${LATEST_RUN_STATUS}" >&2
  exit 1
fi

JOBS_JSON="$(e2e_api_get "/v1/runs/${RUN_ID}/repos/${REPO_ID}/jobs" | tee "${E2E_ARTIFACT_DIR}/jobs-final.json")"
jobs_count="$(printf '%s' "$JOBS_JSON" | jq '.jobs | length')"
if (( jobs_count != 0 )); then
  echo "error: expected zero jobs when prep is failed, got=${jobs_count}" >&2
  exit 1
fi

"$PLOY_BIN" run status "$RUN_ID" > "${E2E_ARTIFACT_DIR}/run-status.txt" 2>&1 || true
echo "OK: prep-fail scenario"
echo "Run: ${RUN_ID}"
echo "Repo: ${REPO_ID}"
echo "Failure code: ${FAILURE_CODE}"
echo "Artifacts saved to: ${E2E_ARTIFACT_DIR}"
