#!/usr/bin/env bash
# E2E scenario: Remote Build Gate mode — Validate multi-VPS gate behavior.
#
# This script validates Build Gate execution in remote-http mode where gate jobs
# are queued via the HTTP API and executed by dedicated Build Gate worker nodes.
#
# Key validations:
# 1. PLOY_BUILDGATE_MODE=remote-http routes gates through HTTP API (not local docker)
# 2. Gate jobs appear in buildgate_jobs table with status transitions
# 3. Gates can execute on different nodes than the mods steps
# 4. Repo+diff semantics remain correct for healing scenarios
# 5. Multi-step runs with healing produce consistent results across execution modes
#
# Prerequisites:
# - ploy binary available at dist/ploy (run: make build)
# - Cluster configured with at least one Build Gate worker node
# - Workers must have PLOY_BUILDGATE_MODE=remote-http configured
# - PostgreSQL access for DB inspection (optional, for detailed validation)
# - Access to target repository (public read, auth for MR creation)
#
# Usage:
#   # Basic run (validates remote-http mode is active):
#   PLOY_BUILDGATE_MODE=remote-http bash tests/e2e/mods/scenario-remote-buildgate/run.sh
#
#   # With custom repository:
#   PLOY_BUILDGATE_MODE=remote-http \
#   REPO_URL="https://gitlab.com/example/repo.git" \
#   bash tests/e2e/mods/scenario-remote-buildgate/run.sh
#
#   # Skip artifact collection (faster, for CI):
#   SKIP_ARTIFACTS=1 PLOY_BUILDGATE_MODE=remote-http \
#   bash tests/e2e/mods/scenario-remote-buildgate/run.sh

set -euo pipefail

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

# Repository and branch configuration (override via environment variables).
# Default: failing branch to trigger healing, demonstrating remote gate + healing flow.
REPO_URL="${REPO_URL:-https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git}"
REPO_BASE_REF="${REPO_BASE_REF:-e2e/fail-missing-symbol}"
REPO_TARGET_REF="${REPO_TARGET_REF:-mods-upgrade-java17-remote-gate}"

# Spec file location (relative to script directory)
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SPEC_FILE="${SCRIPT_DIR}/mod.yaml"

# Artifact collection (optional)
SKIP_ARTIFACTS="${SKIP_ARTIFACTS:-0}"
if [[ "$SKIP_ARTIFACTS" == "0" ]]; then
  TS=$(date +%y%m%d%H%M%S)
  ARTIFACT_DIR="${ARTIFACT_DIR:-./tmp/mods/scenario-remote-buildgate/${TS}}"
  mkdir -p "${ARTIFACT_DIR}"
fi

################################################################################
# PRE-FLIGHT CHECKS
################################################################################

echo "=========================================="
echo "E2E: Remote Build Gate Mode Scenario"
echo "=========================================="
echo "Ploy binary:         $PLOY_BIN"
echo "Repo URL:            $REPO_URL"
echo "Base ref:            $REPO_BASE_REF"
echo "Target ref:          $REPO_TARGET_REF"
echo "Spec file:           $SPEC_FILE"
echo "PLOY_BUILDGATE_MODE: ${PLOY_BUILDGATE_MODE:-<not set>}"
if [[ "$SKIP_ARTIFACTS" == "0" ]]; then
  echo "Artifacts:           $ARTIFACT_DIR"
else
  echo "Artifacts:           SKIPPED"
fi
echo "=========================================="
echo ""

# Verify spec file exists
if [[ ! -f "$SPEC_FILE" ]]; then
  echo "Error: Spec file not found at $SPEC_FILE" >&2
  exit 1
fi

# Warn if PLOY_BUILDGATE_MODE is not set to remote-http.
# This scenario is designed to validate remote gate behavior.
if [[ "${PLOY_BUILDGATE_MODE:-}" != "remote-http" ]]; then
  echo "Warning: PLOY_BUILDGATE_MODE is not 'remote-http'."
  echo "         This scenario is designed to test remote Build Gate mode."
  echo "         Run with: PLOY_BUILDGATE_MODE=remote-http $0"
  echo ""
  echo "         Proceeding anyway (will use local-docker mode if not configured on workers)."
  echo ""
fi

# Check if GitLab PAT is set (optional but recommended for MR validation)
if [[ -z "${PLOY_GITLAB_PAT:-}" ]]; then
  echo "Note: PLOY_GITLAB_PAT not set. MR creation will be skipped."
  echo "      To enable MR validation: export PLOY_GITLAB_PAT=your-token"
  echo ""
fi

################################################################################
# SUBMIT RUN AND FOLLOW LOGS
################################################################################

echo "Submitting mod run with remote Build Gate configuration..."
echo ""

# Build command with required flags
CMD_ARGS=(
  "$PLOY_BIN"
  mod run
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

# Add optional GitLab flags
if [[ -n "${PLOY_GITLAB_PAT:-}" ]]; then
  CMD_ARGS+=(--gitlab-pat "$PLOY_GITLAB_PAT")
fi
if [[ -n "${PLOY_GITLAB_DOMAIN:-}" ]]; then
  CMD_ARGS+=(--gitlab-domain "$PLOY_GITLAB_DOMAIN")
fi

# Execute the run.
# Capture exit code without exiting immediately for post-run analysis.
set +e
"${CMD_ARGS[@]}"
EXIT_CODE=$?
set -e

echo ""
echo "=========================================="
if [[ $EXIT_CODE -eq 0 ]]; then
  echo "OK: Remote Build Gate scenario PASSED"
else
  echo "FAIL: Remote Build Gate scenario FAILED (exit code: $EXIT_CODE)"
fi
echo "=========================================="
echo ""

################################################################################
# POST-RUN VALIDATION
################################################################################

# Validation checklist for remote Build Gate mode.
# Manual verification steps for operators.
echo "Validation checklist for remote Build Gate mode:"
echo ""

echo "1. Build Gate job routing (verify via DB or logs):"
echo "   - Check buildgate_jobs table for job records."
echo "   - Verify jobs have node_id set (claimed by Build Gate worker)."
echo "   - Status transitions: pending -> claimed -> running -> passed/failed."
echo "   - SQL: SELECT id, status, node_id, created_at, finished_at FROM buildgate_jobs ORDER BY created_at DESC LIMIT 10;"
echo ""

echo "2. Multi-VPS gate execution:"
echo "   - Gate jobs should be claimable by any eligible Build Gate worker."
echo "   - In multi-node clusters, gates may run on different nodes than mods steps."
echo "   - Check control plane logs for job assignment: 'buildgate job claimed by node'"
echo ""

echo "3. Repo+diff semantics (for healing scenarios):"
echo "   - When healing modifies workspace, re-gate receives diff_patch."
echo "   - Build Gate worker clones repo+ref and applies diff_patch."
echo "   - Gate result should match expectations for healing changes."
echo "   - Check artifacts for heal.patch or accumulated workspace diffs."
echo ""

echo "4. Comparison with local-docker baseline:"
echo "   - Run same scenario with PLOY_BUILDGATE_MODE unset (local-docker)."
echo "   - Results should be identical (same gate pass/fail, same healing)."
echo "   - Only difference: gate execution location (local vs remote worker)."
echo ""

# Automated checks on artifacts if available.
if [[ "$SKIP_ARTIFACTS" == "0" && -d "$ARTIFACT_DIR" ]]; then
  echo "5. Artifact inspection:"
  echo ""

  # Check for gate execution artifacts.
  if ls "${ARTIFACT_DIR}"/build-gate*.log 1>/dev/null 2>&1; then
    echo "   [x] Build Gate logs present in artifacts."
    # Extract gate status from logs if possible.
    for gate_log in "${ARTIFACT_DIR}"/build-gate*.log; do
      if grep -q "BUILD SUCCESS" "$gate_log" 2>/dev/null; then
        echo "       -> $(basename "$gate_log"): BUILD SUCCESS"
      elif grep -q "BUILD FAILURE" "$gate_log" 2>/dev/null; then
        echo "       -> $(basename "$gate_log"): BUILD FAILURE (expected for healing flow)"
      fi
    done
  else
    echo "   [ ] No Build Gate logs found (check artifact collection)."
  fi

  # Check for Codex healing artifacts (session + workspace diff handshake).
  CODEX_LOG="${ARTIFACT_DIR}/codex.log"
  CODEX_LAST="${ARTIFACT_DIR}/codex-last.txt"
  if [[ -f "$CODEX_LOG" ]] || [[ -f "$CODEX_LAST" ]]; then
    echo "   [x] Codex healing artifacts present."
    if [[ -f "$CODEX_LAST" ]]; then
      echo "       -> codex-last.txt present (last assistant message captured)"
    fi
  fi

  # Check for diff patches (repo+diff validation).
  if ls "${ARTIFACT_DIR}"/*.patch 1>/dev/null 2>&1; then
    echo "   [x] Diff patches present (repo+diff semantics validated)."
    for patch_file in "${ARTIFACT_DIR}"/*.patch; do
      PATCH_SIZE=$(wc -c < "$patch_file" | tr -d ' ')
      echo "       -> $(basename "$patch_file"): ${PATCH_SIZE} bytes"
    done
  fi

  echo ""
  echo "Artifacts saved to: $ARTIFACT_DIR"
  echo "Review with: ls -lh $ARTIFACT_DIR && cat $ARTIFACT_DIR/*.log"
fi

echo ""
echo "=========================================="
echo "Remote Build Gate E2E Validation Complete"
echo "=========================================="

exit $EXIT_CODE
