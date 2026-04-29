#!/usr/bin/env bash
# E2E scenario: Multi-step Migs run with post-mig gate healing.
#
# This script validates gate-heal-regate behavior when a mig transformation
# introduces a compile error (post-mig gate fails), requiring healing to fix
# it before the run can continue.
#
# Scenario flow:
#   1. Submit multi-step mig run to fail-missing-symbol branch
#   2. Some steps may trigger post-gate failures (missing class reference)
#   3. Healing mig creates the missing class
#   4. Re-gate passes after healing
#   5. Subsequent steps execute successfully
#
# This guards against regressions in:
# - Post-mig gate failure detection
# - Healing execution after post-gate failures
# - Re-gate success propagation
# - GateSummary reflecting final (post-mig) gate result
#
# Prerequisites:
# - ploy binary available at dist/ploy (run: make build)
# - Cluster descriptor at ${PLOY_CONFIG_HOME:-$HOME/.config/ploy}/default
# - Codex auth at ~/.codex/auth.json (for healing)
# - Optional: PLOY_GITLAB_PAT for MR creation
#
# Usage:
#   # From repository root:
#   bash tests/e2e/migs/scenario-post-mig-heal/run.sh
#
#   # With custom configuration:
#   REPO_URL="https://gitlab.com/example/repo.git" \
#   REPO_BASE_REF="fail-branch" \
#   bash tests/e2e/migs/scenario-post-mig-heal/run.sh
#
#   # Skip artifact collection:
#   SKIP_ARTIFACTS=1 bash tests/e2e/migs/scenario-post-mig-heal/run.sh

set -euo pipefail

################################################################################
# CONFIGURATION
################################################################################

# Default to the local Docker cluster descriptor written by ploy cluster deploy.
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
export PLOY_CONFIG_HOME="${PLOY_CONFIG_HOME:-$HOME/.config/ploy}"
source "$REPO_ROOT/tests/e2e/lib/ensure_local_descriptor.sh"
ensure_local_descriptor "$REPO_ROOT" "$PLOY_CONFIG_HOME"

# Locate ploy binary (check multiple possible locations).
PLOY_BIN=""
for candidate in "../../../../dist/ploy" "./dist/ploy" "dist/ploy"; do
  if [[ -x "$candidate" ]]; then
    PLOY_BIN="$candidate"
    break
  fi
done

if [[ -z "$PLOY_BIN" ]]; then
  echo "Error: ploy binary not found. Run 'make build' first." >&2
  exit 1
fi

# Repository and branch configuration (override via environment variables).
# Uses e2e/fail-missing-symbol branch which has code referencing UndefinedClass.
REPO_URL="${REPO_URL:-https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git}"
REPO_BASE_REF="${REPO_BASE_REF:-e2e/fail-missing-symbol}"
REPO_TARGET_REF="${REPO_TARGET_REF:-migs-e2e-post-mig-heal-$(date +%y%m%d%H%M%S)}"

# Spec file location (relative to script directory).
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SPEC_FILE="${SCRIPT_DIR}/mig.yaml"

# Artifact collection (optional).
SKIP_ARTIFACTS="${SKIP_ARTIFACTS:-0}"
if [[ "$SKIP_ARTIFACTS" == "0" ]]; then
  TS=$(date +%y%m%d%H%M%S)
  ARTIFACT_DIR="${ARTIFACT_DIR:-./tmp/migs/scenario-post-mig-heal/${TS}}"
  mkdir -p "${ARTIFACT_DIR}"
fi

################################################################################
# PRE-FLIGHT CHECKS
################################################################################

echo "=========================================="
echo "E2E: Post-Mig Gate Healing Scenario"
echo "=========================================="
echo "Ploy binary:     $PLOY_BIN"
echo "PLOY_CONFIG_HOME: $PLOY_CONFIG_HOME"
echo "Repo URL:        $REPO_URL"
echo "Base ref:        $REPO_BASE_REF"
echo "Target ref:      $REPO_TARGET_REF"
echo "Spec file:       $SPEC_FILE"
if [[ "$SKIP_ARTIFACTS" == "0" ]]; then
  echo "Artifacts:       $ARTIFACT_DIR"
else
  echo "Artifacts:       SKIPPED"
fi
echo "=========================================="
echo ""

# Verify spec file exists.
if [[ ! -f "$SPEC_FILE" ]]; then
  echo "Error: Spec file not found at $SPEC_FILE" >&2
  exit 1
fi

# Check for Codex auth (recommended for healing).
if [[ ! -f "${HOME}/.codex/auth.json" ]]; then
  echo "Warning: ~/.codex/auth.json not found."
  echo "         Healing may fail without Codex authentication."
  echo "         Set up Codex auth before running this scenario."
  echo ""
fi

# Check for GitLab PAT (optional).
if [[ -z "${PLOY_GITLAB_PAT:-}" ]]; then
  echo "Note: PLOY_GITLAB_PAT not set. MR creation will be skipped."
  echo "      To enable MR validation: export PLOY_GITLAB_PAT=your-token"
  echo ""
fi

################################################################################
# SUBMIT RUN AND FOLLOW LOGS
################################################################################

echo "Submitting multi-step mig run with post-mig healing..."
echo ""

# Build command with required flags.
CMD_ARGS=(
  "$PLOY_BIN"
  mig run
  --repo-url "$REPO_URL"
  --repo-base-ref "$REPO_BASE_REF"
  --repo-target-ref "$REPO_TARGET_REF"
  --spec "$SPEC_FILE"
  --follow
)

# Add artifact directory if collection is enabled.
if [[ "$SKIP_ARTIFACTS" == "0" ]]; then
  CMD_ARGS+=(--artifact-dir "$ARTIFACT_DIR")
fi

# Execute the run.
# Disable 'set -e' temporarily to capture exit code and provide summary.
set +e
"${CMD_ARGS[@]}"
EXIT_CODE=$?
set -e

echo ""
echo "=========================================="
if [[ $EXIT_CODE -eq 0 ]]; then
  echo "✓ Post-mig gate healing scenario PASSED"
else
  echo "✗ Post-mig gate healing scenario FAILED (exit code: $EXIT_CODE)"
fi
echo "=========================================="
echo ""

################################################################################
# POST-RUN VALIDATION
################################################################################

if [[ "$SKIP_ARTIFACTS" == "0" ]]; then
  echo "Validating post-mig healing artifacts..."
  echo ""

  # ────────────────────────────────────────────────────────────────────────────
  # Extract Codex healing /out bundle(s) into $ARTIFACT_DIR.
  # Nodeagent uploads /out as a tar.gz bundle named "mig-out", and the CLI
  # downloads it as "*_mig-out.bin" files alongside manifest.json.
  # ────────────────────────────────────────────────────────────────────────────
  echo "Extracting Codex mig-out artifact bundles (if present)..."
  shopt -s nullglob
  mig_out_bundles=("${ARTIFACT_DIR}"/*_mig-out.bin)
  shopt -u nullglob
  if ((${#mig_out_bundles[@]} == 0)); then
    echo "  - no mig-out bundles found in ${ARTIFACT_DIR} (Codex artifacts may be missing)"
  else
    for bundle in "${mig_out_bundles[@]}"; do
      echo "  extracting $(basename "$bundle")"
      if ! tar -xzf "$bundle" -C "${ARTIFACT_DIR}"; then
        echo "  ⚠ failed to extract ${bundle}"
      fi
    done
  fi
  echo ""

  VALIDATION_FAILED=0

  # ─────────────────────────────────────────────────────────────────────────────
  # 1. Verify healing summary artifact is present. Codex now signals completion by exiting;
  #    the node agent decides whether to re-run the gate based on workspace diffs.
  # ─────────────────────────────────────────────────────────────────────────────
  CODEX_LAST="${ARTIFACT_DIR}/codex-last.txt"

  echo "  1. Healing artifacts:"
  if [[ -f "$CODEX_LAST" ]]; then
    echo "     ✓ codex-last.txt present (last assistant message captured)"
  else
    echo "     - codex-last.txt not found (healing may not have run)"
  fi
  echo ""

  # ─────────────────────────────────────────────────────────────────────────────
  # 2. Verify codex-last.txt artifact.
  # ─────────────────────────────────────────────────────────────────────────────
  CODEX_LAST="${ARTIFACT_DIR}/codex-last.txt"

  echo "  2. Healing summary artifact:"
  if [[ -f "$CODEX_LAST" ]]; then
    echo "     ✓ codex-last.txt present"
  else
    echo "     - codex-last.txt not found"
    echo "       This is expected if healing did not run."
  fi
  echo ""

  # ─────────────────────────────────────────────────────────────────────────────
  # 3. Verify gate-related artifacts exist (from Build Gate runs).
  # ─────────────────────────────────────────────────────────────────────────────
  echo "  3. Build Gate artifacts:"
  GATE_ARTIFACTS=$(find "$ARTIFACT_DIR" -name "*build-gate*.log*" -o -name "*build-gate*.bin" 2>/dev/null | wc -l)
  if [[ $GATE_ARTIFACTS -gt 0 ]]; then
    echo "     ✓ Found $GATE_ARTIFACTS gate-related artifacts"
    find "$ARTIFACT_DIR" -name "*build-gate*" -exec basename {} \; 2>/dev/null | head -5 | while read -r f; do
      echo "       - $f"
    done
  else
    echo "     ⚠ No build-gate artifacts found"
    echo "       Gate artifacts should be present for post-mig gate runs."
  fi
  echo ""

  # ─────────────────────────────────────────────────────────────────────────────
  # 5. Summary
  # ─────────────────────────────────────────────────────────────────────────────
  echo "  Summary:"
  echo "     Artifacts saved to: $ARTIFACT_DIR"
  echo ""
  if [[ $EXIT_CODE -eq 0 ]]; then
    echo "  Post-mig gate healing validation complete."
    echo "  Run succeeded; GateSummary should reflect final (post-mig) gate result."
  else
    echo "  Run failed. Check artifacts and logs for details."
    echo "  Possible causes:"
    echo "  - Healing could not fix the post-gate error (retries exhausted)"
    echo "  - Codex auth not configured (healing container failed)"
    echo "  - Network/cluster issues"
  fi
  echo ""
fi

################################################################################
# VALIDATION CHECKLIST (manual)
################################################################################

if [[ $EXIT_CODE -eq 0 ]]; then
  echo "Manual validation checklist:"
  echo ""
  echo "1. Post-mig gate failure detection:"
  echo "   - Review SSE events for 'gate_failed' events after mig steps"
  echo "   - Logs should indicate post-mig (not pre-mig) gate failure"
  echo ""
  echo "2. Healing execution:"
  echo "   - Confirm Codex edited files under /workspace and exited (no in-container gate runs)"
  echo "   - Verify healing created/modified files to fix the error"
  echo ""
  echo "3. Re-gate success:"
  echo "   - Review SSE events for 'gate_passed' after healing"
  echo "   - GateSummary should show passing final-gate (see run status metadata)"
  echo "   - Command: dist/ploy run status <run-id>"
  echo ""
  echo "4. Multi-step continuation:"
  echo "   - All steps should complete after healing"
  echo "   - Final status should be success"
  echo ""
  echo "5. MR content (if PLOY_GITLAB_PAT was set):"
  echo "   - MR should contain original transformation + healing fixes"
  echo "   - MR title should reference target ref: $REPO_TARGET_REF"
  echo ""
fi

exit $EXIT_CODE
