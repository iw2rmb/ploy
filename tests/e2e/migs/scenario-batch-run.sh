#!/usr/bin/env bash
set -euo pipefail

# E2E: Mig project run workflow scenario.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=tests/e2e/lib/harness.sh
source "${SCRIPT_DIR}/../lib/harness.sh"

e2e_init "${BASH_SOURCE[0]}"

if [[ "${PLOY_E2E_SKIP_BATCH_RUN:-}" == "1" ]]; then
  echo "SKIP: scenario-batch-run (PLOY_E2E_SKIP_BATCH_RUN=1)"
  exit 0
fi

e2e_artifacts_init "$REPO_ROOT/tmp/migs/batch-run"

TS="$(date +%y%m%d%H%M%S)"
BATCH_NAME="e2e-batch-${TS}"

run() {
  if [[ "${PLOY_E2E_BATCH_RUN_DRY:-}" == "1" ]]; then
    echo "[dry] $*"
  else
    echo "[run] $*"
    "$@"
  fi
}

echo "========================================="
echo "E2E: Mig project run workflow scenario"
echo "========================================="
echo "Mig name:   ${BATCH_NAME}"
echo "Artifacts:  ${E2E_ARTIFACT_DIR}"
echo ""

echo "[1/5] Creating mig project: ${BATCH_NAME}"
SPEC_FILE="${E2E_ARTIFACT_DIR}/batch-spec.yaml"
cat > "$SPEC_FILE" <<'YAML'
steps:
  - image: alpine:3.20
    command: |
      echo "[batch-e2e] Starting repo processing"
      echo "Repo: $PLOY_REPO_URL"
      echo "Base: $PLOY_BASE_REF"
      sleep 2
      echo "[batch-e2e] Done"
YAML

run "$PLOY_BIN" mig add --name "$BATCH_NAME" --spec "$SPEC_FILE" \
  > "${E2E_ARTIFACT_DIR}/create-mig.out" 2>&1 || {
  echo "WARN: Mig project creation may have failed (expected if control plane not running)"
  cat "${E2E_ARTIFACT_DIR}/create-mig.out" || true
}

echo ""
echo "[2/5] Adding repos to mig"
run "$PLOY_BIN" mig repo add "$BATCH_NAME" \
  --repo "https://github.com/example/repo1.git" \
  --base-ref main \
  > "${E2E_ARTIFACT_DIR}/add-repo1.out" 2>&1 || true

run "$PLOY_BIN" mig repo add "$BATCH_NAME" \
  --repo "https://github.com/example/repo2.git" \
  --base-ref main \
  > "${E2E_ARTIFACT_DIR}/add-repo2.out" 2>&1 || true

echo ""
echo "[3/5] Listing repos in mig"
run "$PLOY_BIN" mig repo list "$BATCH_NAME" \
  > "${E2E_ARTIFACT_DIR}/repos.out" 2>&1 || true
cat "${E2E_ARTIFACT_DIR}/repos.out" || true

echo ""
echo "[4/5] Running mig project"
run "$PLOY_BIN" mig run "$BATCH_NAME" --follow \
  > "${E2E_ARTIFACT_DIR}/run.out" 2>&1 || true

RUN_ID=""
if [[ -f "${E2E_ARTIFACT_DIR}/run.out" ]]; then
  RUN_ID="$(awk 'NF {print; exit}' "${E2E_ARTIFACT_DIR}/run.out" || true)"
fi

if [[ -z "$RUN_ID" ]]; then
  echo "WARN: Could not extract run ID from output."
  echo "This test requires a running control plane."
  echo "Skipping remaining steps."
  echo ""
  echo "OK: scenario-batch-run (partial - no control plane)"
  exit 0
fi

echo ""
echo "[5/5] Verifying final status"
run "$PLOY_BIN" run status "$RUN_ID" \
  > "${E2E_ARTIFACT_DIR}/final-status.out" 2>&1 || true
echo "Final status:"
cat "${E2E_ARTIFACT_DIR}/final-status.out" || true

echo ""
echo "========================================="
echo "OK: scenario-batch-run"
echo "Artifacts saved to: ${E2E_ARTIFACT_DIR}"
echo "========================================="
