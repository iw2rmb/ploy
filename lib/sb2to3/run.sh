#!/usr/bin/env bash
set -euo pipefail

# E2E: OpenRewrite apply for Spring Boot 2.x -> 3.0 with Build Gate and healing.

REPO=${REPO:-invest-capital/capital-funds-api}
# Allow overriding REPO_URL externally; otherwise build it from GitLab creds.
REPO_URL=${REPO_URL:-https://${GITLAB_USER}:${GITLAB_PAT}@gitlab.tcsbank.ru/${REPO}.git}
TARGET_REF=${PLOY_TARGET_REF:-lsc-sb2-to-3-0}

resolve_base_ref() {
  if [[ -n "${PLOY_BASE_REF:-}" ]]; then
    printf '%s\n' "${PLOY_BASE_REF}"
    return
  fi

  # Default to remote HEAD branch when available (main/master/etc.).
  local head_ref=""
  head_ref=$(git ls-remote --symref "${REPO_URL}" HEAD 2>/dev/null | awk '/^ref:/ {print $2}' | sed 's#^refs/heads/##' | head -n1 || true)
  if [[ -n "${head_ref}" ]]; then
    printf '%s\n' "${head_ref}"
    return
  fi

  printf '%s\n' "main"
}

BASE_REF=$(resolve_base_ref)

# Artifacts directory: default to ./lib/sb2to3/runs/<YYMMDDHHmmss>/
TS=$(date +%y%m%d%H%M%S)
ARTIFACT_BASE=${PLOY_E2E_ARTIFACT_BASE:-/Users/v.v.kovalev/@iw2rmb/ploy/lib/sb2to3/runs}
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

echo "OK: sb2-to-3-0."
echo "Artifacts saved to: ${ARTIFACT_DIR}"
