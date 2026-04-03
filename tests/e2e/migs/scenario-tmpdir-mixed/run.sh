#!/usr/bin/env bash
set -euo pipefail

# E2E: Hydra in-record mixed inputs (plain file + directory).
#
# Validates that a spec with two `in` entries — one plain file and one
# directory — results in both being visible under /in inside the container.
#
# Assertions:
#   1. Run completes with status "Success".
#   2. /in/config.json is present in the container (file entry).
#   3. /in/scripts/build.sh is present in the container (directory entry).

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=tests/e2e/lib/harness.sh
source "${SCRIPT_DIR}/../../lib/harness.sh"

e2e_init "${BASH_SOURCE[0]}"
e2e_artifacts_init "$REPO_ROOT/tmp/migs/scenario-in-mixed"

REPO="${PLOY_E2E_REPO_OVERRIDE:-https://github.com/octocat/Hello-World.git}"
BASE_REF="${PLOY_E2E_BASE_REF:-master}"
TARGET_REF="${PLOY_E2E_TARGET_REF:-e2e/in-mixed}"

FIXTURES_DIR="${SCRIPT_DIR}/fixtures"
CONFIG_PATH="${FIXTURES_DIR}/config.json"
SCRIPTS_PATH="${FIXTURES_DIR}/scripts"

echo "=========================================="
echo "Hydra In-Record Mixed Inputs E2E Scenario"
echo "=========================================="
echo "Repo:        $REPO"
echo "Base ref:    $BASE_REF"
echo "Target ref:  $TARGET_REF"
echo "Fixtures:    $FIXTURES_DIR"
echo "Artifacts:   $E2E_ARTIFACT_DIR"
echo "=========================================="

# Generate a spec with Hydra in-record entries using absolute fixture paths.
SPEC_FILE="$(mktemp "${TMPDIR:-/tmp}/ploy-e2e-in-mixed.XXXXXX.yaml")"
trap 'rm -f "$SPEC_FILE"' EXIT

cat >"$SPEC_FILE" <<YAML
apiVersion: ploy.mig/v1alpha1
kind: MigRunSpec
steps:
  - image: alpine:3.20
    command: >-
      sh -c '
        set -e;
        test -f /in/config.json || { echo "FAIL: /in/config.json missing"; exit 1; };
        test -f /in/build.sh    || { echo "FAIL: /in/build.sh missing"; exit 1; };
        echo "OK: /in/config.json present";
        echo "OK: /in/build.sh present";
        cat /in/config.json
      '
    in:
      - ${CONFIG_PATH}:/in/config.json
      - ${SCRIPTS_PATH}/build.sh:/in/build.sh
YAML

RUN_JSON="$(e2e_mig_run_json \
  --repo-url "$REPO" \
  --repo-base-ref "$BASE_REF" \
  --repo-target-ref "$TARGET_REF" \
  --spec "$SPEC_FILE" \
  --follow)"

RUN_ID="$(e2e_mig_run_id "$RUN_JSON")"
if [[ -z "$RUN_ID" ]]; then
  echo "error: failed to parse run_id from mig run output" >&2
  printf '%s\n' "$RUN_JSON" >&2
  exit 1
fi

e2e_run_status_safe "$RUN_ID"

FAILED=0

REPO_STATUS="$(printf '%s' "$RUN_JSON" | jq -r '.repos[0].status // empty')"
if [[ "$REPO_STATUS" == "Success" ]]; then
  echo "  + repo status: Success"
else
  echo "  ! repo status: expected Success, got '${REPO_STATUS}'" >&2
  FAILED=1
fi

if ((FAILED > 0)); then
  echo "FAIL: scenario-in-mixed — assertions failed."
  echo "Artifacts saved to: ${E2E_ARTIFACT_DIR}"
  exit 1
fi

echo ""
echo "OK: scenario-in-mixed (file + directory in-record entries visible under /in)."
echo "Artifacts saved to: ${E2E_ARTIFACT_DIR}"
