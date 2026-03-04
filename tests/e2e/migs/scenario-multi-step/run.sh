#!/usr/bin/env bash
set -euo pipefail

# Multi-step migs E2E scenario runner.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=tests/e2e/lib/harness.sh
source "${SCRIPT_DIR}/../../lib/harness.sh"

e2e_init "${BASH_SOURCE[0]}"
e2e_artifacts_init "$REPO_ROOT/tmp/migs/scenario-multi-step"

REPO_URL="${REPO_URL:-https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git}"
REPO_BASE_REF="${REPO_BASE_REF:-main}"
REPO_TARGET_REF="${REPO_TARGET_REF:-java6-multy-mig}"
SPEC_FILE="${SCRIPT_DIR}/mig.yaml"

echo "=========================================="
echo "Multi-step Mods E2E Scenario"
echo "=========================================="
echo "Repo URL:         $REPO_URL"
echo "Base ref:         $REPO_BASE_REF"
echo "Target ref:       $REPO_TARGET_REF"
echo "Spec file:        $SPEC_FILE"
echo "PLOY_CONFIG_HOME: $PLOY_CONFIG_HOME"
echo "Artifacts:        $E2E_ARTIFACT_DIR"
echo "=========================================="

"$PLOY_BIN" mig run \
  --repo-url "$REPO_URL" \
  --repo-base-ref "$REPO_BASE_REF" \
  --repo-target-ref "$REPO_TARGET_REF" \
  --spec "$SPEC_FILE" \
  --follow \
  --artifact-dir "$E2E_ARTIFACT_DIR"

echo ""
echo "Multi-step mig run completed."
echo "Check the logs above for per-step execution status and diffs."
