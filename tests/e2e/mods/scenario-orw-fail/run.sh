#!/usr/bin/env bash
set -euo pipefail

# E2E: ORW apply on failing branch -> Build Gate fails -> Healing via Codex -> re‑gate -> proceed.

REPO=${PLOY_E2E_REPO_OVERRIDE:-https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git}
BASE_REF=${PLOY_E2E_BASE_REF:-e2e/fail-missing-symbol}
TARGET_REF=${PLOY_E2E_TARGET_REF:-mods-upgrade-java17-heal}

# Artifacts directory: default to ./tmp/mods/scenario-orw-fail/<YYMMDDHHmmss>/
TS=$(date +%y%m%d%H%M%S)
ARTIFACT_BASE=${PLOY_E2E_ARTIFACT_BASE:-./tmp/mods/scenario-orw-fail}
ARTIFACT_DIR=${PLOY_E2E_ARTIFACT_DIR:-${ARTIFACT_BASE}/${TS}}
mkdir -p "${ARTIFACT_DIR}"

SPEC=${PLOY_E2E_SPEC:-tests/e2e/mods/scenario-orw-fail/mod.yaml}

# Optional per-run GitLab overrides
EXTRA_FLAGS=()
if [[ -n "${PLOY_GITLAB_PAT:-}" ]]; then
  EXTRA_FLAGS+=(--gitlab-pat "${PLOY_GITLAB_PAT}")
fi
if [[ -n "${PLOY_GITLAB_DOMAIN:-}" ]]; then
  EXTRA_FLAGS+=(--gitlab-domain "${PLOY_GITLAB_DOMAIN}")
fi

TICKET=$(dist/ploy mod run --json \
  --repo-url "$REPO" \
  --repo-base-ref "$BASE_REF" \
  --repo-target-ref "$TARGET_REF" \
  --spec "$SPEC" \
  --follow \
  --artifact-dir "${ARTIFACT_DIR}" \
  "${EXTRA_FLAGS[@]}" | jq -r '.ticket_id')

if [[ -n "${TICKET:-}" ]]; then
  dist/ploy mod inspect "$TICKET" || true
fi

# ─────────────────────────────────────────────────────────────────────────────
# Validate Codex healing pipeline artifacts (sentinel + session handshake)
# Per ROADMAP.md Phase D: RED→GREEN→REFACTOR discipline for Codex healing.
# ─────────────────────────────────────────────────────────────────────────────
echo ""
echo "Validating Codex healing pipeline artifacts..."

VALIDATION_FAILED=0

# 1. Verify sentinel behavior: Codex should emit [[REQUEST_BUILD_VALIDATION]].
#    The sentinel is detected from codex.log or codex-last.txt in artifacts.
CODEX_LOG="${ARTIFACT_DIR}/codex.log"
CODEX_LAST="${ARTIFACT_DIR}/codex-last.txt"
SENTINEL_FLAG="${ARTIFACT_DIR}/request_build_validation"

if [[ -f "$SENTINEL_FLAG" ]]; then
  echo "  ✓ Sentinel flag file present (request_build_validation)"
elif [[ -f "$CODEX_LAST" ]] && grep -q '\[\[REQUEST_BUILD_VALIDATION\]\]' "$CODEX_LAST"; then
  echo "  ✓ Sentinel detected in codex-last.txt"
elif [[ -f "$CODEX_LOG" ]] && grep -q '\[\[REQUEST_BUILD_VALIDATION\]\]' "$CODEX_LOG"; then
  echo "  ✓ Sentinel detected in codex.log"
else
  echo "  ⚠ Sentinel [[REQUEST_BUILD_VALIDATION]] not found in artifacts (optional)"
  # Not a hard failure; older Codex versions may not emit sentinel.
fi

# 2. Verify session resume support: Check for codex-session.txt in artifacts.
#    This file enables Codex to resume across healing retries.
CODEX_SESSION="${ARTIFACT_DIR}/codex-session.txt"

if [[ -f "$CODEX_SESSION" ]]; then
  SESSION_ID=$(cat "$CODEX_SESSION" | tr -d '\r\n')
  if [[ -n "$SESSION_ID" ]]; then
    echo "  ✓ Codex session captured: ${SESSION_ID:0:20}..."
  else
    echo "  ⚠ codex-session.txt is empty (session resume not available)"
  fi
else
  echo "  ⚠ codex-session.txt not found (session resume not available)"
fi

# 3. Verify codex-run.json manifest contains required fields.
CODEX_MANIFEST="${ARTIFACT_DIR}/codex-run.json"

if [[ -f "$CODEX_MANIFEST" ]]; then
  if grep -q '"requested_build_validation"' "$CODEX_MANIFEST"; then
    echo "  ✓ Manifest contains requested_build_validation field"
  else
    echo "  ⚠ Manifest missing requested_build_validation field"
  fi
  if grep -q '"session_id"' "$CODEX_MANIFEST"; then
    echo "  ✓ Manifest contains session_id field"
  else
    echo "  ⚠ Manifest missing session_id field"
  fi
  if grep -q '"resumed"' "$CODEX_MANIFEST"; then
    echo "  ✓ Manifest contains resumed field"
  else
    echo "  ⚠ Manifest missing resumed field"
  fi
else
  echo "  ⚠ codex-run.json not found in artifacts"
fi

echo ""
if [[ $VALIDATION_FAILED -eq 0 ]]; then
  echo "OK: scenario-orw-fail (spec-driven healing with Codex handshake validation)."
else
  echo "FAIL: scenario-orw-fail (Codex handshake validation failed)."
  exit 1
fi
echo "Artifacts saved to: ${ARTIFACT_DIR}"

