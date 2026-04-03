#!/usr/bin/env bash
set -euo pipefail

# E2E: Hydra /out upload continuity.
#
# Validates that files written to /out/ during step execution are uploaded
# as artifacts and retrievable via the artifact download API after run
# completion. This proves the out-record → upload → artifact pipeline
# is intact end-to-end.
#
# Flow:
#   1. Step writes deterministic content to /out/report.json.
#   2. Run completes with Success.
#   3. Artifacts are downloaded via --artifact-dir.
#   4. Assert the downloaded artifact contains the expected content.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=tests/e2e/lib/harness.sh
source "${SCRIPT_DIR}/../../lib/harness.sh"

e2e_init "${BASH_SOURCE[0]}"
e2e_artifacts_init "$REPO_ROOT/tmp/migs/scenario-hydra-out-upload"

REPO="${PLOY_E2E_REPO_OVERRIDE:-https://github.com/octocat/Hello-World.git}"
BASE_REF="${PLOY_E2E_BASE_REF:-master}"
TARGET_REF="${PLOY_E2E_TARGET_REF:-e2e/hydra-out-upload}"

echo "=========================================="
echo "Hydra /out Upload Continuity E2E Scenario"
echo "=========================================="
echo "Repo:       $REPO"
echo "Base ref:   $BASE_REF"
echo "Target ref: $TARGET_REF"
echo "Artifacts:  $E2E_ARTIFACT_DIR"
echo "=========================================="

# Create a seed file for the out record (CLI compiles the source path).
OUT_SEED="$(mktemp "${TMPDIR:-/tmp}/ploy-e2e-out-seed.XXXXXX.json")"
echo '{"status":"seed"}' >"$OUT_SEED"
trap 'rm -f "$OUT_SEED"' EXIT

SPEC_FILE="$(mktemp "${TMPDIR:-/tmp}/ploy-e2e-out-upload.XXXXXX.yaml")"
trap 'rm -f "$OUT_SEED" "$SPEC_FILE"' EXIT

cat >"$SPEC_FILE" <<YAML
apiVersion: ploy.mig/v1alpha1
kind: MigRunSpec
steps:
  - image: alpine:3.20
    command: >-
      sh -c '
        set -e;
        echo "{\"status\":\"completed\",\"items\":42}" > /out/report.json;
        cat /out/report.json;
        echo "OK: wrote /out/report.json"
      '
    out:
      - ${OUT_SEED}:/out/report.json
YAML

ARTIFACT_DL_DIR="${E2E_ARTIFACT_DIR}/downloaded"
mkdir -p "$ARTIFACT_DL_DIR"

RUN_JSON="$(e2e_mig_run_json \
  --repo-url "$REPO" \
  --repo-base-ref "$BASE_REF" \
  --repo-target-ref "$TARGET_REF" \
  --spec "$SPEC_FILE" \
  --follow \
  --artifact-dir "$ARTIFACT_DL_DIR")"

printf '%s\n' "$RUN_JSON" >"${E2E_ARTIFACT_DIR}/run-out-upload.json"

FAILED=0

REPO_STATUS="$(printf '%s' "$RUN_JSON" | jq -r '.repos[0].status // empty')"
if [[ "$REPO_STATUS" == "Success" ]]; then
  echo "  + repo status: Success"
else
  echo "  ! repo status: expected Success, got '${REPO_STATUS}'" >&2
  FAILED=1
fi

# Check that the artifact was downloaded.
if [[ -f "$ARTIFACT_DL_DIR/manifest.json" ]]; then
  echo "  + manifest.json: present"
else
  echo "  ! manifest.json: missing in artifact dir" >&2
  FAILED=1
fi

# Check artifact content if present.
REPORT_FILES="$(find "$ARTIFACT_DL_DIR" -name '*report*' -type f 2>/dev/null || echo "")"
if [[ -n "$REPORT_FILES" ]]; then
  echo "  + report artifact files found:"
  echo "$REPORT_FILES" | while read -r f; do echo "    - $f"; done
else
  echo "  ! no report artifact found in downloaded artifacts" >&2
  FAILED=1
fi

if ((FAILED > 0)); then
  echo ""
  echo "FAIL: scenario-hydra-out-upload — assertions failed."
  echo "Artifacts saved to: ${E2E_ARTIFACT_DIR}"
  exit 1
fi

echo ""
echo "OK: scenario-hydra-out-upload (/out write + artifact upload + download verified)."
echo "Artifacts saved to: ${E2E_ARTIFACT_DIR}"
