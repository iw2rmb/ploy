#!/usr/bin/env bash
set -euo pipefail

# E2E: ORW apply on failing branch -> Build Gate fails -> Healing via Codex -> re‑gate -> proceed.

REPO=${REPO:-twork-crm/customer-device}
# Allow overriding REPO_URL externally; otherwise build it from GitLab creds.
REPO_URL=${REPO_URL:-https://${GITLAB_USER}:${GITLAB_PAT}@gitlab.tcsbank.ru/${REPO}.git}
BASE_REF=${PLOY_BASE_REF:-master}
TARGET_REF=${PLOY_TARGET_REF:-lsc-java11-to-17}

# Artifacts directory: default to ./lib/mods/scenario-orw-fail/<YYMMDDHHmmss>/
TS=$(date +%y%m%d%H%M%S)
ARTIFACT_BASE=${PLOY_E2E_ARTIFACT_BASE:-/Users/v.v.kovalev/@iw2rmb/ploy/lib/j11to17/runs}
ARTIFACT_DIR=${PLOY_E2E_ARTIFACT_DIR:-${ARTIFACT_BASE}/${TS}}
mkdir -p "${ARTIFACT_DIR}"

SPEC=${PLOY_SPEC:-mig.yaml}

# Optional per-run GitLab overrides
EXTRA_FLAGS=()
if [[ -n "${GITLAB_PAT:-}" ]]; then
  EXTRA_FLAGS+=(--gitlab-pat "${GITLAB_PAT}")
fi

if [[ -n "${GITLAB_DOMAIN:-}" ]]; then
  EXTRA_FLAGS+=(--gitlab-domain "${GITLAB_DOMAIN}")
fi

RUN=$(/Users/v.v.kovalev/ploy/dist/ploy run --json \
  --repo "$REPO_URL" \
  --base-ref "$BASE_REF" \
  --target-ref "$TARGET_REF" \
  --spec "$SPEC" \
  --follow \
  --artifact-dir "${ARTIFACT_DIR}" \
  "${EXTRA_FLAGS[@]}" | jq -r '.run_id')

if [[ -n "${RUN:-}" ]]; then
  /Users/v.v.kovalev/ploy/dist/ploy run status "$RUN" || true
fi

echo "OK: java11-to-17."
echo "Artifacts saved to: ${ARTIFACT_DIR}"
