#!/usr/bin/env bash
set -euo pipefail

# E2E: ORW apply on failing branch -> Build Gate fails -> healing -> re-gate.
# Direct Codex mode: no amata.spec for router or healing; CODEX_PROMPT required.
#
# Validates (strict) — parity contract with scenario-orw-fail:
#   1. Final repo status is "Success".
#   2. Router produced a non-empty bug_summary (direct mode router summary).
#   3. A heal job is present (healing attempt in direct mode).
#   4. A re_gate job is present (re-gate status sequence).
#   5. Codex handshake artifacts satisfy the metadata contract (strict mode).
#   6. codex-last.txt contains valid JSON (router summary contract).
#
# Proves: direct codex exec enforces CODEX_PROMPT end-to-end in the healing loop.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=tests/e2e/lib/harness.sh
source "${SCRIPT_DIR}/../../lib/harness.sh"

e2e_init "${BASH_SOURCE[0]}"
e2e_artifacts_init "$REPO_ROOT/tmp/migs/scenario-orw-fail-direct"

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
echo "=== Validating healing-loop outcomes — direct mode (strict) ==="

FAILED=0

# 1. Final repo status must be Success.
REPO_STATUS="$(printf '%s' "$RUN_JSON" | jq -r '.repos[0].status // empty')"
if [[ "$REPO_STATUS" == "Success" ]]; then
  echo "  + repo status: Success"
else
  echo "  ! repo status: expected Success, got '${REPO_STATUS}'" >&2
  FAILED=1
fi

# 2. Router must have produced a non-empty bug_summary (direct mode).
BUG_SUMMARY="$(printf '%s' "$RUN_JSON" | jq -r '[.repos[0].jobs[] | select(.bug_summary != null and .bug_summary != "")] | first | .bug_summary // empty')"
if [[ -n "$BUG_SUMMARY" ]]; then
  echo "  + router bug_summary: present (${BUG_SUMMARY:0:60}...)"
else
  echo "  ! router bug_summary: missing or empty — direct-mode router did not produce a summary" >&2
  FAILED=1
fi

# 3. A heal job must be present (healing attempt in direct mode).
HEAL_JOB="$(printf '%s' "$RUN_JSON" | jq -r '[.repos[0].jobs[] | select(.job_type == "heal")] | first | .job_type // empty')"
if [[ "$HEAL_JOB" == "heal" ]]; then
  echo "  + heal job: present"
else
  echo "  ! heal job: missing — direct-mode healing was not attempted" >&2
  FAILED=1
fi

# 4. A re_gate job must be present.
REGATE_JOB="$(printf '%s' "$RUN_JSON" | jq -r '[.repos[0].jobs[] | select(.job_type == "re_gate")] | first | .job_type // empty')"
if [[ "$REGATE_JOB" == "re_gate" ]]; then
  echo "  + re_gate job: present"
else
  echo "  ! re_gate job: missing — re-gate did not run after direct-mode healing" >&2
  FAILED=1
fi

echo ""
echo "Extracting Codex mig-out artifact bundles..."
e2e_extract_mig_out_bundles "$E2E_ARTIFACT_DIR"

echo ""
echo "Validating Codex healing pipeline artifacts (strict)..."
if ! e2e_validate_codex_handshake "$E2E_ARTIFACT_DIR" strict; then
  echo "  ! codex handshake: metadata contract not satisfied" >&2
  FAILED=1
fi

# 5. codex-last.txt must contain valid JSON (router summary contract, direct mode).
CODEX_LAST="${E2E_ARTIFACT_DIR}/codex-last.txt"
if [[ -f "$CODEX_LAST" ]]; then
  if jq -e . "$CODEX_LAST" > /dev/null 2>&1; then
    echo "  + codex-last.txt: valid JSON (direct-mode router summary contract satisfied)"
  else
    echo "  ! codex-last.txt: not valid JSON — direct-mode router summary contract violated" >&2
    FAILED=1
  fi
else
  echo "  ! codex-last.txt: missing" >&2
  FAILED=1
fi

echo ""
if ((FAILED > 0)); then
  echo "FAIL: scenario-orw-fail-direct — one or more healing-loop assertions failed."
  echo "Artifacts saved to: ${E2E_ARTIFACT_DIR}"
  exit 1
fi

echo "OK: scenario-orw-fail-direct (direct-mode router and healing — all assertions passed)."
echo "Artifacts saved to: ${E2E_ARTIFACT_DIR}"
