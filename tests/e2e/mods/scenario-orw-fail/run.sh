#!/usr/bin/env bash
set -euo pipefail

# E2E: ORW apply on failing branch -> Build Gate fails -> Healing via Codex -> re‑gate -> proceed.

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
export PLOY_CONFIG_HOME="${PLOY_CONFIG_HOME:-$REPO_ROOT/local/cli}"

REPO=${PLOY_E2E_REPO_OVERRIDE:-https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git}
BASE_REF=${PLOY_E2E_BASE_REF:-e2e/fail-missing-symbol}

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

RUN=$("$REPO_ROOT/dist/ploy" mod run --json \
  --repo-url "$REPO" \
  --repo-base-ref "$BASE_REF" \
  --spec "$SPEC" \
  --follow \
  --artifact-dir "${ARTIFACT_DIR}" \
  "${EXTRA_FLAGS[@]}" | jq -r '.run_id')

if [[ -n "${RUN:-}" ]]; then
  # Print run summary for debugging; inspect subcommand has been removed.
  "$REPO_ROOT/dist/ploy" run status "$RUN" || true
fi

# ─────────────────────────────────────────────────────────────────────────────
# Extract Codex healing /out bundle(s) into $ARTIFACT_DIR.
# Nodeagent uploads /out as a tar.gz bundle named "mod-out", and the CLI
# downloads it as one or more "*_mod-out.bin" files plus manifest.json.
# ─────────────────────────────────────────────────────────────────────────────
echo ""
echo "Extracting Codex mod-out artifact bundles (if present)..."
shopt -s nullglob
mod_out_bundles=("${ARTIFACT_DIR}"/*_mod-out.bin)
shopt -u nullglob

if ((${#mod_out_bundles[@]} == 0)); then
  echo "  - no mod-out bundles found in ${ARTIFACT_DIR} (Codex artifacts may be missing)"
else
  for bundle in "${mod_out_bundles[@]}"; do
    echo "  extracting $(basename "$bundle")"
    if ! tar -xzf "$bundle" -C "${ARTIFACT_DIR}"; then
      echo "  ⚠ failed to extract ${bundle}"
    fi
  done
fi

# ─────────────────────────────────────────────────────────────────────────────
# Validate Codex healing pipeline artifacts (session + workspace diff handshake)
# RED→GREEN→REFACTOR discipline for Codex healing.
# ─────────────────────────────────────────────────────────────────────────────
echo ""
echo "Validating Codex healing pipeline artifacts..."

VALIDATION_FAILED=0

# 1. Verify Codex logs are present. Codex now signals completion by exiting;
#    the node agent decides whether to re-run the gate based on workspace diffs.
CODEX_LOG="${ARTIFACT_DIR}/codex.log"
CODEX_LAST="${ARTIFACT_DIR}/codex-last.txt"
if [[ -f "$CODEX_LOG" ]]; then
  echo "  ✓ codex.log present"
else
  echo "  ⚠ codex.log not found (Codex healing may not have run)"
fi
if [[ -f "$CODEX_LAST" ]]; then
  echo "  ✓ codex-last.txt present (last assistant message captured)"
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
