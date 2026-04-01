#!/usr/bin/env bash
# E2E scenario: Multi-step, multi-node Mods run with rehydration validation
#
# This script validates the complete multi-node execution flow for multi-step Mods runs.
# It submits a three-step Java migration workflow and validates that:
# - Steps can execute on different nodes (when multi-node cluster is available)
# - Rehydration works correctly (base clone + ordered diff application)
# - Build gate validation passes after each step
# - Final MR content reflects cumulative changes from all steps
#
# Prerequisites:
# - ploy binary available at dist/ploy (run: make build)
# - Cluster descriptor configured at ~/.config/ploy/default
# - Access to target repository (public read, auth for MR creation)
# - Optional: PLOY_GITLAB_PAT for MR creation validation
#
# Usage:
#   # From repository root:
#   bash tests/e2e/migs/scenario-multi-node-rehydration/run.sh
#
#   # With custom configuration:
#   REPO_URL="https://gitlab.com/example/repo.git" \
#   REPO_BASE_REF="main" \
#   REPO_TARGET_REF="test-branch" \
#   ARTIFACT_DIR="./tmp/custom" \
#   bash tests/e2e/migs/scenario-multi-node-rehydration/run.sh
#
#   # Skip artifact collection (faster, for CI):
#   SKIP_ARTIFACTS=1 bash tests/e2e/migs/scenario-multi-node-rehydration/run.sh

set -euo pipefail

# Default to the local Docker cluster descriptor written by deploy/runtime/run.sh.
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
export PLOY_CONFIG_HOME="${PLOY_CONFIG_HOME:-$HOME/.config/ploy/local}"
source "$REPO_ROOT/tests/e2e/lib/ensure_local_descriptor.sh"
ensure_local_descriptor "$REPO_ROOT" "$PLOY_CONFIG_HOME"

################################################################################
# CONFIGURATION
################################################################################

# Locate ploy binary (check multiple possible locations)
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

# Repository and branch configuration (override via environment variables)
REPO_URL="${REPO_URL:-https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git}"
REPO_BASE_REF="${REPO_BASE_REF:-main}"
REPO_TARGET_REF="${REPO_TARGET_REF:-java6-multy-mig}"

# Spec file location (relative to script directory)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SPEC_FILE="${SCRIPT_DIR}/mig.yaml"

# Artifact collection (optional)
SKIP_ARTIFACTS="${SKIP_ARTIFACTS:-0}"
if [[ "$SKIP_ARTIFACTS" == "0" ]]; then
  TS=$(date +%y%m%d%H%M%S)
  ARTIFACT_DIR="${ARTIFACT_DIR:-./tmp/migs/scenario-multi-node-rehydration/${TS}}"
  mkdir -p "${ARTIFACT_DIR}"
fi

################################################################################
# PRE-FLIGHT CHECKS
################################################################################

echo "=========================================="
echo "E2E: Multi-Node Rehydration Scenario"
echo "=========================================="
echo "Ploy binary:     $PLOY_BIN"
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

# Verify spec file exists
if [[ ! -f "$SPEC_FILE" ]]; then
  echo "Error: Spec file not found at $SPEC_FILE" >&2
  exit 1
fi

# Check if GitLab PAT is set (optional but recommended for MR validation)
if [[ -z "${PLOY_GITLAB_PAT:-}" ]]; then
  echo "Warning: PLOY_GITLAB_PAT not set. MR creation will be skipped."
  echo "         To enable MR validation: export PLOY_GITLAB_PAT=your-token"
  echo ""
fi

################################################################################
# SUBMIT RUN AND FOLLOW LOGS
################################################################################

echo "Submitting multi-step mig run..."
echo ""

# Build command with required flags
CMD_ARGS=(
  "$PLOY_BIN"
  mig run
  --repo-url "$REPO_URL"
  --repo-base-ref "$REPO_BASE_REF"
  --repo-target-ref "$REPO_TARGET_REF"
  --spec "$SPEC_FILE"
  --follow
)

# Add artifact directory if collection is enabled
if [[ "$SKIP_ARTIFACTS" == "0" ]]; then
  CMD_ARGS+=(--artifact-dir "$ARTIFACT_DIR")
fi

# Execute the run
# The --follow flag will stream logs until the run completes or fails.
# Temporarily disable 'set -e' so we can capture the exit code and
# print a post-run summary even on failure.
set +e
"${CMD_ARGS[@]}"
EXIT_CODE=$?
set -e

echo ""
echo "=========================================="
if [[ $EXIT_CODE -eq 0 ]]; then
  echo "✓ Multi-node rehydration scenario PASSED"
else
  echo "✗ Multi-node rehydration scenario FAILED (exit code: $EXIT_CODE)"
fi
echo "=========================================="
echo ""

################################################################################
# POST-RUN VALIDATION SUMMARY
################################################################################

if [[ $EXIT_CODE -eq 0 ]]; then
  echo "Validation checklist (manual verification):"
  echo ""
  echo "1. Multi-node execution (if multi-node cluster):"
  echo "   - Check control plane logs or status API for node assignments"
  echo "   - Verify different steps were claimed by different nodes"
  echo "   - Command: dist/ploy run status <run-id>"
  echo ""
  echo "2. Rehydration correctness:"
  echo "   - Review step logs for successful diff application"
  echo "   - No 'git apply' failures should appear in logs"
  if [[ "$SKIP_ARTIFACTS" == "0" ]]; then
    echo "   - Check artifacts in: $ARTIFACT_DIR"
  fi
  echo ""
  echo "3. Build gate validation:"
  echo "   - All steps should show build gate passed"
  echo "   - No healing should be required for this scenario"
  echo ""
  echo "4. MR content (if PLOY_GITLAB_PAT was set):"
  echo "   - Check GitLab for created MR"
  echo "   - Verify MR diff contains all three Java migration changes"
  echo "   - MR should reference target ref: $REPO_TARGET_REF"
  echo ""
  echo "5. Single-node fallback:"
  echo "   - If running on single-node cluster, all steps execute on same node"
  echo "   - Rehydration still occurs and validates correctly"
  echo ""

  # ─────────────────────────────────────────────────────────────────────────────
  # Validate Codex healing pipeline artifacts (if healing occurred)
  # Validation checklist for Codex healing.
  # ─────────────────────────────────────────────────────────────────────────────
  if [[ "$SKIP_ARTIFACTS" == "0" ]]; then
    echo "Extracting Codex mig-out artifact bundles (if present)..."
    shopt -s nullglob
    mod_out_bundles=("${ARTIFACT_DIR}"/*_mig-out.bin)
    shopt -u nullglob
    if ((${#mod_out_bundles[@]} == 0)); then
      echo "   - no mig-out bundles found in ${ARTIFACT_DIR} (Codex artifacts may be missing)"
    else
      for bundle in "${mod_out_bundles[@]}"; do
        echo "   extracting $(basename "$bundle")"
        if ! tar -xzf "$bundle" -C "${ARTIFACT_DIR}"; then
          echo "   ⚠ failed to extract ${bundle}"
        fi
      done
    fi
    echo ""

    echo "6. Codex healing handshake (if healing was triggered):"
    echo "   - Codex completion: Codex exits after editing; node agent re-gates when workspace diffs exist"
    echo "   - Session resume: Check for codex-session.txt artifact for retry continuity"
    echo ""

    # Automated validation of Codex artifacts (if present).
    CODEX_LOG="${ARTIFACT_DIR}/codex.log"
    CODEX_SESSION="${ARTIFACT_DIR}/codex-session.txt"
    CODEX_MANIFEST="${ARTIFACT_DIR}/codex-run.json"

    echo "   Automated artifact checks:"
    if [[ -f "$CODEX_LOG" ]]; then
      echo "   ✓ codex.log present"
    else
      echo "   - codex.log not present (no Codex healing in this run)"
    fi

    if [[ -f "$CODEX_SESSION" ]]; then
      SESSION_ID=$(cat "$CODEX_SESSION" | tr -d '\r\n')
      if [[ -n "$SESSION_ID" ]]; then
        echo "   ✓ Session ID captured for resume: ${SESSION_ID:0:20}..."
      fi
    fi

    if [[ -f "$CODEX_MANIFEST" ]]; then
      echo "   ✓ codex-run.json manifest present"
    fi
    echo ""
  fi

  if [[ "$SKIP_ARTIFACTS" == "0" ]]; then
    echo "Artifacts saved to: $ARTIFACT_DIR"
    echo "Review artifacts with:"
    echo "  ls -lh $ARTIFACT_DIR"
    echo "  cat $ARTIFACT_DIR/*.log"
    echo ""
  fi

  echo "Next steps:"
  echo "  - Review control plane logs: docker compose -f deploy/runtime/docker-compose.yml logs -f server"
  echo "  - Review node logs: docker compose -f deploy/runtime/docker-compose.yml logs -f node"
  echo "  - Check run status: PLOY_CONFIG_HOME=$PLOY_CONFIG_HOME dist/ploy run status <run-id>"
  echo ""
fi

exit $EXIT_CODE
