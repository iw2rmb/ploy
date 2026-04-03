#!/usr/bin/env bash
set -euo pipefail

# E2E: ORW apply on failing branch -> Build Gate fails -> healing -> re-gate.
# Direct Codex mode: no amata.spec for router or healing; prompt via Hydra in mount.
#
# Validates (strict) — parity contract with scenario-orw-fail:
#   1. Final repo status is "Success".
#   2. Router produced a non-empty bug_summary (direct mode router summary).
#   3. A heal job is present (healing attempt in direct mode).
#   4. A re_gate job is present (re-gate status sequence).
#   5. Codex handshake artifacts satisfy the metadata contract (strict mode).
#   6. codex-last.txt satisfies the JSON schema contract: valid JSON, .error_kind == "code",
#      .bug_summary present and non-empty, no unresolved template tokens.
#   7. (Negative gate) Running with direct mode but without prompt file fails deterministically
#      with "prompt required" — enforcement is proven end-to-end.
#
# Proves: direct codex exec enforces prompt file delivery end-to-end in the healing loop.

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

# 5. codex-last.txt must satisfy the JSON schema contract.
CODEX_LAST="${E2E_ARTIFACT_DIR}/codex-last.txt"
if [[ -f "$CODEX_LAST" ]]; then
  if ! jq -e . "$CODEX_LAST" > /dev/null 2>&1; then
    echo "  ! codex-last.txt: not valid JSON — direct-mode router summary contract violated" >&2
    FAILED=1
  else
    echo "  + codex-last.txt: valid JSON"

    ERROR_KIND="$(jq -r '.error_kind // empty' "$CODEX_LAST")"
    if [[ "$ERROR_KIND" == "code" ]]; then
      echo "  + codex-last.txt .error_kind: \"code\""
    else
      echo "  ! codex-last.txt .error_kind: expected \"code\", got \"${ERROR_KIND}\"" >&2
      FAILED=1
    fi

    CODEX_BUG_SUMMARY="$(jq -r '.bug_summary // empty' "$CODEX_LAST")"
    if [[ -n "$CODEX_BUG_SUMMARY" ]]; then
      echo "  + codex-last.txt .bug_summary: present"
    else
      echo "  ! codex-last.txt .bug_summary: missing or empty" >&2
      FAILED=1
    fi

    if grep -qF '{{' "$CODEX_LAST"; then
      echo "  ! codex-last.txt: contains unresolved template tokens ({{)" >&2
      FAILED=1
    else
      echo "  + codex-last.txt: no unresolved template tokens"
    fi
  fi
else
  echo "  ! codex-last.txt: missing" >&2
  FAILED=1
fi

echo ""
echo "=== Negative gate: direct mode without prompt file must fail deterministically ==="

# 7. A spec that uses direct codex mode but omits the prompt file must be rejected
#    by codex with "prompt required" before any healing attempt.
_NO_PROMPT_SPEC="$(mktemp "${TMPDIR:-/tmp}/ploy-e2e-no-prompt.XXXXXX.yaml")"
cat >"$_NO_PROMPT_SPEC" <<'YAML'
apiVersion: ploy.mig/v1alpha1
kind: MigRunSpec

steps:
  - image: localhost:5000/ploy/orw-cli-maven:latest
    env:
      RECIPE_GROUP: org.openrewrite.recipe
      RECIPE_ARTIFACT: rewrite-migrate-java
      RECIPE_VERSION: "3.20.0"
      RECIPE_CLASSNAME: org.openrewrite.java.migrate.UpgradeToJava17

build_gate:
  enabled: true
  # Router: direct Codex mode — prompt file intentionally omitted to prove enforcement.
  router:
    image: localhost:5000/ploy/codex:latest
    home:
      - ~/.codex/auth.json:.codex/auth.json

mr_on_fail: false
mr_on_success: false
YAML

_NO_PROMPT_OUT="$(e2e_mig_run_json \
  --repo-url "$REPO" \
  --repo-base-ref "$BASE_REF" \
  --repo-target-ref "$TARGET_REF" \
  --spec "$_NO_PROMPT_SPEC" \
  --follow 2>&1)" || true
rm -f "$_NO_PROMPT_SPEC"

if printf '%s' "$_NO_PROMPT_OUT" | grep -qF "prompt required"; then
  echo "  + negative gate: run without prompt file rejected with 'prompt required' (enforcement confirmed)"
else
  echo "  ! negative gate: expected 'prompt required' error but got:" >&2
  printf '%s\n' "$_NO_PROMPT_OUT" >&2
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
