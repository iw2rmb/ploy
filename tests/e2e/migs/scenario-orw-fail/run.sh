#!/usr/bin/env bash
set -euo pipefail

# E2E: ORW apply on failing branch -> Build Gate fails -> healing -> re-gate.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=tests/e2e/lib/harness.sh
source "${SCRIPT_DIR}/../../lib/harness.sh"

e2e_init "${BASH_SOURCE[0]}"
e2e_artifacts_init "$REPO_ROOT/tmp/migs/scenario-orw-fail"

REPO="${PLOY_E2E_REPO_OVERRIDE:-https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git}"
BASE_REF="${PLOY_E2E_BASE_REF:-e2e/fail-missing-symbol}"
TARGET_REF="${PLOY_E2E_TARGET_REF:-migs-upgrade-java17-heal}"
SPEC="${PLOY_E2E_SPEC:-${SCRIPT_DIR}/mig.yaml}"

RUN_JSON="$(e2e_mig_run_json \
  --repo-url "$REPO" \
  --repo-base-ref "$BASE_REF" \
  --repo-target-ref "$TARGET_REF" \
  --spec "$SPEC" \
  --follow)"
RUN_ID="$(e2e_mig_run_id "$RUN_JSON")"

if [[ -z "$RUN_ID" ]]; then
  echo "error: failed to parse run_id from mig run output" >&2
  printf '%s\n' "$RUN_JSON" >&2
  exit 1
fi

e2e_run_status_safe "$RUN_ID"

echo ""
echo "Extracting Codex mig-out artifact bundles (if present)..."
e2e_extract_mig_out_bundles "$E2E_ARTIFACT_DIR" || true

echo ""
echo "Validating Codex healing pipeline artifacts..."
e2e_validate_codex_handshake "$E2E_ARTIFACT_DIR" advisory || true

echo ""
echo "OK: scenario-orw-fail (spec-driven healing with Codex handshake validation)."
echo "Artifacts saved to: ${E2E_ARTIFACT_DIR}"
