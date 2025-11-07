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

echo "OK: scenario-orw-fail (spec-driven healing)."
echo "Artifacts saved to: ${ARTIFACT_DIR}"

