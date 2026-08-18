#!/usr/bin/env bash
set -euo pipefail

# E2E: simple container self-test to validate container runtime + SSE logs.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=tests/e2e/lib/harness.sh
source "${SCRIPT_DIR}/../lib/harness.sh"

e2e_init "${BASH_SOURCE[0]}"
e2e_artifacts_init "$REPO_ROOT/tmp/migs/selftest"

REPO="${PLOY_E2E_REPO_OVERRIDE:-https://github.com/octocat/Hello-World.git}"
BASE_REF="${PLOY_E2E_BASE_REF:-master}"
E2E_IMAGE="$(e2e_runtime_image)"
CMD='echo "[selftest] hello"; uname -a; sleep 3; echo "[selftest] done"'
SPEC_FILE="${E2E_ARTIFACT_DIR}/selftest.yaml"

cat > "$SPEC_FILE" <<YAML
build_gate:
  disabled: true
steps:
  - image: ${E2E_IMAGE}
    command: '$CMD'
YAML

RUN_JSON="$(e2e_mig_run_json \
  "$SPEC_FILE" \
  "$(e2e_repo_selector "$REPO" "$BASE_REF")" \
  --follow \
  --pull "$E2E_ARTIFACT_DIR")"

RUN_STATUS="$(printf '%s' "$RUN_JSON" | jq -r '.repos[0].status // empty')"
JOB_ID="$(printf '%s' "$RUN_JSON" | jq -r '.repos[0].jobs[] | select(.job_type == "mig") | .job_id')"
if [[ "$RUN_STATUS" != "Success" || -z "$JOB_ID" ]]; then
  echo "FAIL: selftest expected a successful mig job, got status='${RUN_STATUS}' job_id='${JOB_ID}'" >&2
  exit 1
fi

JOB_LOG="$(e2e_wait_job_log "$JOB_ID")"
printf '%s\n' "$JOB_LOG" >"${E2E_ARTIFACT_DIR}/selftest.log"
if ! printf '%s' "$JOB_LOG" | grep -qF '[selftest] hello' ||
   ! printf '%s' "$JOB_LOG" | grep -qF '[selftest] done'; then
  echo "FAIL: selftest job log is missing expected start or completion output" >&2
  exit 1
fi

echo "OK: selftest scenario (container execution and logs verified)"
echo "Artifacts saved to: ${E2E_ARTIFACT_DIR}"
