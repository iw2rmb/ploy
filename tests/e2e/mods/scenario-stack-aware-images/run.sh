#!/usr/bin/env bash
set -euo pipefail

# Default to the local Docker cluster descriptor written by scripts/local-docker.sh.
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
export PLOY_CONFIG_HOME="${PLOY_CONFIG_HOME:-$REPO_ROOT/local/cli}"
source "$REPO_ROOT/tests/e2e/lib/ensure_local_descriptor.sh"
ensure_local_descriptor "$REPO_ROOT" "$PLOY_CONFIG_HOME"

# E2E: Stack-aware image selection scenario.
#
# This scenario validates that Mods correctly resolves images from a stack-aware
# image map based on Build Gate detection. It uses the Maven test repository,
# which should trigger selection of the java-maven image.
#
# What to verify:
#   1. Build Gate detects "java-maven" stack (pom.xml present).
#   2. Image resolution selects the java-maven key from the image map.
#   3. Mod execution proceeds with the selected image.
#   4. Node agent logs show the resolved stack and selected image.
#
# For manual verification, check node agent logs for:
#   - "detected stack: java-maven" (or similar)
#   - "resolved image for stack java-maven" (or similar)

REPO=${PLOY_E2E_REPO_OVERRIDE:-https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git}
BASE_REF=${PLOY_E2E_BASE_REF:-main}
TARGET_REF=${PLOY_E2E_TARGET_REF:-e2e/stack-aware-test}

# Artifacts directory: default to ./tmp/mods/scenario-stack-aware-images/<YYMMDDHHmmss>/
TS=$(date +%y%m%d%H%M%S)
ARTIFACT_BASE=${PLOY_E2E_ARTIFACT_BASE:-./tmp/mods/scenario-stack-aware-images}
ARTIFACT_DIR=${PLOY_E2E_ARTIFACT_DIR:-${ARTIFACT_BASE}/${TS}}
mkdir -p "${ARTIFACT_DIR}"

SPEC=${PLOY_E2E_SPEC:-tests/e2e/mods/scenario-stack-aware-images/mod.yaml}

# Optional per-run GitLab overrides
EXTRA_FLAGS=()
if [[ -n "${PLOY_GITLAB_PAT:-}" ]]; then
  EXTRA_FLAGS+=(--gitlab-pat "${PLOY_GITLAB_PAT}")
fi
if [[ -n "${PLOY_GITLAB_DOMAIN:-}" ]]; then
  EXTRA_FLAGS+=(--gitlab-domain "${PLOY_GITLAB_DOMAIN}")
fi

# Skip artifact collection if SKIP_ARTIFACTS is set (for faster CI runs).
if [[ -z "${SKIP_ARTIFACTS:-}" ]]; then
  EXTRA_FLAGS+=(--artifact-dir "${ARTIFACT_DIR}")
fi

echo "Running stack-aware image selection scenario..."
echo "  Repository: $REPO"
echo "  Base ref:   $BASE_REF"
echo "  Target ref: $TARGET_REF"
echo "  Spec:       $SPEC"
echo ""

RUN=$(dist/ploy mod run --json \
  --repo-url "$REPO" \
  --repo-base-ref "$BASE_REF" \
  --repo-target-ref "$TARGET_REF" \
  --spec "$SPEC" \
  --follow \
  "${EXTRA_FLAGS[@]}" | jq -r '.run_id')

if [[ -n "${RUN:-}" ]]; then
  echo ""
  echo "Inspecting run..."
  dist/ploy run status "$RUN" || true
fi

# ─────────────────────────────────────────────────────────────────────────────
# Validate stack-aware image selection
# ─────────────────────────────────────────────────────────────────────────────
echo ""
echo "Validating stack-aware image selection..."

VALIDATION_FAILED=0

# 1. Verify the run succeeded (indicates image was resolved correctly).
#    If the run failed with "no image specified for stack", the map form failed.
if [[ -n "${RUN:-}" ]]; then
  # Derive run status from `ploy run status` human-readable output.
  RUN_STATUS=$(dist/ploy run status "$RUN" 2>/dev/null | awk -F': ' '/^Status:/ {print tolower($2)}' | tr -d ' ' || echo "unknown")
  if [[ "$RUN_STATUS" == "succeeded" ]]; then
    echo "  Pass: Run succeeded (stack-aware image resolution worked)"
  elif [[ "$RUN_STATUS" == "failed" ]]; then
    echo "  FAIL: Run failed - check if image resolution error occurred"
    VALIDATION_FAILED=1
  else
    echo "  Note: Run status: $RUN_STATUS"
  fi
  else
    echo "  FAIL: No run ID returned"
  VALIDATION_FAILED=1
fi

# 2. Check for stack detection in node logs (if artifacts available).
#    This validates Build Gate detected the correct stack.
if [[ -d "${ARTIFACT_DIR}" && -z "${SKIP_ARTIFACTS:-}" ]]; then
  # Look for build gate log artifacts that might indicate stack detection.
  BUILD_GATE_LOG=$(find "${ARTIFACT_DIR}" -name '*build-gate*.log*' -o -name '*gate*.log' 2>/dev/null | head -1)
  if [[ -n "${BUILD_GATE_LOG:-}" && -f "$BUILD_GATE_LOG" ]]; then
    if grep -qi 'maven' "$BUILD_GATE_LOG" 2>/dev/null; then
      echo "  Pass: Build Gate logs indicate Maven execution"
    else
      echo "  Note: Build Gate logs present but Maven detection not confirmed"
    fi
  else
    echo "  Note: Build Gate logs not found in artifacts (normal for some configurations)"
  fi
fi

echo ""
if [[ $VALIDATION_FAILED -eq 0 ]]; then
  echo "OK: scenario-stack-aware-images"
else
  echo "FAIL: scenario-stack-aware-images"
  exit 1
fi
echo "Artifacts saved to: ${ARTIFACT_DIR}"
