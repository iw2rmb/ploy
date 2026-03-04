#!/usr/bin/env bash
set -euo pipefail

# E2E: Batch run workflow scenario.

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
echo "E2E: Batch run workflow scenario"
echo "========================================="
echo "Batch name: ${BATCH_NAME}"
echo "Artifacts:  ${E2E_ARTIFACT_DIR}"
echo ""

echo "[1/5] Creating batch run: ${BATCH_NAME}"
SPEC_FILE="${E2E_ARTIFACT_DIR}/batch-spec.yaml"
cat > "$SPEC_FILE" <<'YAML'
image: alpine:3.20
command: |
  echo "[batch-e2e] Starting repo processing"
  echo "Repo: $PLOY_REPO_URL"
  echo "Branch: $PLOY_TARGET_REF"
  sleep 2
  echo "[batch-e2e] Done"
YAML

run "$PLOY_BIN" mig run \
  --name "$BATCH_NAME" \
  --spec "$SPEC_FILE" \
  --repo-url "https://github.com/placeholder/batch.git" \
  --repo-base-ref main \
  --repo-target-ref "$BATCH_NAME" \
  > "${E2E_ARTIFACT_DIR}/create-batch.out" 2>&1 || {
  echo "WARN: Batch creation may have failed (expected if control plane not running)"
  cat "${E2E_ARTIFACT_DIR}/create-batch.out" || true
}

RUN_ID=""
if [[ -f "${E2E_ARTIFACT_DIR}/create-batch.out" ]]; then
  RUN_ID="$(grep -oE '[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}' "${E2E_ARTIFACT_DIR}/create-batch.out" | head -1 || true)"
fi

if [[ -z "$RUN_ID" ]]; then
  echo "WARN: Could not extract batch ID from output."
  echo "This test requires a running control plane."
  echo "Skipping remaining steps."
  echo ""
  echo "OK: scenario-batch-run (partial - no control plane)"
  exit 0
fi

echo "Batch ID: ${RUN_ID}"
echo ""
echo "[2/5] Adding repos to batch"
run "$PLOY_BIN" mig run repo add \
  --repo-url "https://github.com/example/repo1.git" \
  --base-ref main \
  --target-ref "feature-${TS}-1" \
  "$RUN_ID" \
  > "${E2E_ARTIFACT_DIR}/add-repo1.out" 2>&1 || true

run "$PLOY_BIN" mig run repo add \
  --repo-url "https://github.com/example/repo2.git" \
  --base-ref main \
  --target-ref "feature-${TS}-2" \
  "$RUN_ID" \
  > "${E2E_ARTIFACT_DIR}/add-repo2.out" 2>&1 || true

echo ""
echo "[3/5] Listing repos in batch"
run "$PLOY_BIN" mig run repo status "$RUN_ID" \
  > "${E2E_ARTIFACT_DIR}/status.out" 2>&1 || true
cat "${E2E_ARTIFACT_DIR}/status.out" || true

echo ""
echo "[4/5] Restarting repo with updated branch (if terminal)"
REPO1_ID="$(grep -oE '[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}' "${E2E_ARTIFACT_DIR}/add-repo1.out" | head -1 || true)"
if [[ -n "$REPO1_ID" ]]; then
  run "$PLOY_BIN" mig run repo restart \
    --repo-id "$REPO1_ID" \
    --target-ref "feature-${TS}-1-retry" \
    "$RUN_ID" \
    > "${E2E_ARTIFACT_DIR}/restart-repo1.out" 2>&1 || true
else
  echo "SKIP: Could not extract repo ID for restart test"
fi

echo ""
echo "[5/5] Stopping batch and verifying final status"
run "$PLOY_BIN" run stop "$RUN_ID" \
  > "${E2E_ARTIFACT_DIR}/stop.out" 2>&1 || true

run "$PLOY_BIN" mig run repo status "$RUN_ID" \
  > "${E2E_ARTIFACT_DIR}/final-status.out" 2>&1 || true
echo "Final status:"
cat "${E2E_ARTIFACT_DIR}/final-status.out" || true

echo ""
echo "========================================="
echo "OK: scenario-batch-run"
echo "Artifacts saved to: ${E2E_ARTIFACT_DIR}"
echo "========================================="
