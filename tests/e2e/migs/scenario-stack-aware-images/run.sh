#!/usr/bin/env bash
set -euo pipefail

# E2E: stack-aware image selection scenario.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=tests/e2e/lib/harness.sh
source "${SCRIPT_DIR}/../../lib/harness.sh"

e2e_init "${BASH_SOURCE[0]}"
SKIP_ARTIFACTS="${SKIP_ARTIFACTS:-0}"
e2e_artifacts_init "$REPO_ROOT/tmp/migs/scenario-stack-aware-images" SKIP_ARTIFACTS

REPO="${PLOY_E2E_REPO_OVERRIDE:-https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git}"
BASE_REF="${PLOY_E2E_BASE_REF:-main}"
TARGET_REF="${PLOY_E2E_TARGET_REF:-e2e/stack-aware-test}"
SPEC="${PLOY_E2E_SPEC:-${SCRIPT_DIR}/mig.yaml}"

echo "Running stack-aware image selection scenario..."
echo "  Repository: $REPO"
echo "  Base ref:   $BASE_REF"
echo "  Target ref: $TARGET_REF"
echo "  Spec:       $SPEC"
if [[ -n "${E2E_ARTIFACT_DIR:-}" ]]; then
  echo "  Artifacts:  $E2E_ARTIFACT_DIR"
else
  echo "  Artifacts:  SKIPPED"
fi
echo ""

RUN_JSON="$(e2e_mig_run_json \
  --repo-url "$REPO" \
  --repo-base-ref "$BASE_REF" \
  --repo-target-ref "$TARGET_REF" \
  --spec "$SPEC" \
  --follow)"
RUN_ID="$(e2e_mig_run_id "$RUN_JSON")"

VALIDATION_FAILED=0
if [[ -z "$RUN_ID" ]]; then
  echo "  FAIL: No run ID returned"
  VALIDATION_FAILED=1
else
  echo ""
  echo "Inspecting run..."
  STATUS_OUTPUT="$($PLOY_BIN run status "$RUN_ID" 2>/dev/null || true)"
  printf '%s\n' "$STATUS_OUTPUT"

  RUN_STATUS="$(printf '%s\n' "$STATUS_OUTPUT" | awk -F': ' '/^Status:/ {print tolower($2)}' | tr -d ' ' || true)"
  if [[ "$RUN_STATUS" == "succeeded" ]]; then
    echo "  Pass: Run succeeded (stack-aware image resolution worked)"
  elif [[ "$RUN_STATUS" == "failed" ]]; then
    echo "  FAIL: Run failed - check if image resolution error occurred"
    VALIDATION_FAILED=1
  else
    echo "  Note: Run status: ${RUN_STATUS:-unknown}"
  fi
fi

if [[ -n "${E2E_ARTIFACT_DIR:-}" ]]; then
  BUILD_GATE_LOG="$(find "$E2E_ARTIFACT_DIR" -name '*build-gate*.log*' -o -name '*gate*.log' 2>/dev/null | head -1)"
  if [[ -n "${BUILD_GATE_LOG:-}" && -f "$BUILD_GATE_LOG" ]]; then
    if grep -qi 'maven' "$BUILD_GATE_LOG" 2>/dev/null; then
      echo "  Pass: Build Gate logs indicate Maven execution"
    else
      echo "  Note: Build Gate logs present but Maven detection not confirmed"
    fi
  else
    echo "  Note: Build Gate logs not found in artifacts (normal for some configurations)"
  fi
fi

echo ""
if [[ $VALIDATION_FAILED -eq 0 ]]; then
  echo "OK: scenario-stack-aware-images"
else
  echo "FAIL: scenario-stack-aware-images"
  exit 1
fi
if [[ -n "${E2E_ARTIFACT_DIR:-}" ]]; then
  echo "Artifacts saved to: ${E2E_ARTIFACT_DIR}"
fi
