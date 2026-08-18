#!/usr/bin/env bash
set -euo pipefail

# E2E: Hydra mount enforcement — /in is read-only, /out is writable.
#
# Validates that the Hydra contract enforces mount semantics:
#   1. Files delivered via `in` records are mounted read-only under /in/.
#      A container that attempts to write to /in/ should fail.
#   2. Files delivered via `out` records are mounted writable under /out/.
#      A container can write to /out/ and the results are uploaded after completion.
#
# Part 1 — in read-only enforcement:
#   1. Build a spec with an `in` record pointing to a fixture file.
#   2. The container tries to write to /in/config.json.
#   3. The command succeeds only when the write is rejected.
#
# Part 2 — out write success:
#   1. Build a spec with an `out` record.
#   2. The container writes to /out/result.txt.
#   3. Assert the run succeeds and the output is captured.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=tests/e2e/lib/harness.sh
source "${SCRIPT_DIR}/../../lib/harness.sh"

e2e_init "${BASH_SOURCE[0]}"
e2e_artifacts_init "$REPO_ROOT/tmp/migs/scenario-hydra-mount-enforcement"

REPO="${PLOY_E2E_REPO_OVERRIDE:-https://github.com/octocat/Hello-World.git}"
BASE_REF="${PLOY_E2E_BASE_REF:-master}"
E2E_IMAGE="$(e2e_runtime_image)"

echo "=========================================="
echo "Hydra Mount Enforcement E2E Scenario"
echo "=========================================="
echo "Repo:       $(e2e_repo_selector "$REPO")"
echo "Base ref:   $BASE_REF"
echo "Artifacts:  $E2E_ARTIFACT_DIR"
echo "=========================================="

# --- Part 1: /in read-only enforcement ---

# Create a fixture file for the in record.
IN_FIXTURE="$(mktemp "${TMPDIR:-/tmp}/ploy-e2e-mount-in.XXXXXX.json")"
echo '{"test": true}' >"$IN_FIXTURE"
trap 'rm -f "$IN_FIXTURE"' EXIT

SPEC_RO="$(mktemp "${TMPDIR:-/tmp}/ploy-e2e-mount-ro.XXXXXX.yaml")"
trap 'rm -f "$IN_FIXTURE" "$SPEC_RO"' EXIT

cat >"$SPEC_RO" <<YAML
apiVersion: ploy.mig/v1alpha1
kind: MigRunSpec
build_gate:
  disabled: true
steps:
  - image: ${E2E_IMAGE}
    command: >-
      sh -c '
        echo "attempting write to /in/config.json...";
        if echo "modified" > /in/config.json 2>/dev/null; then
          echo "FAIL: write to /in succeeded unexpectedly";
          exit 1;
        fi;
        echo "OK: write to /in correctly rejected (read-only mount enforced)";
        exit 0
      '
    in:
      - ${IN_FIXTURE}:config.json
YAML

echo ""
echo "--- Part 1: /in read-only mount enforcement ---"

RO_JSON=""
set +e
RO_JSON="$(e2e_mig_run_json \
  "$SPEC_RO" \
  "$(e2e_repo_selector "$REPO" "$BASE_REF")" \
  --follow)"
RO_EXIT=$?
set -e

printf '%s\n' "$RO_JSON" >"${E2E_ARTIFACT_DIR}/run-mount-ro.json"

FAILED=0

RO_STATUS="$(printf '%s' "$RO_JSON" | jq -r '.repos[0].status // empty' 2>/dev/null || echo "")"
if [[ "$RO_STATUS" == "Success" ]]; then
  echo "  + /in write attempt: write was rejected (read-only mount enforced)"
else
  echo "  ! /in write attempt: command detected a writable mount; status='${RO_STATUS}' exit=${RO_EXIT}" >&2
  FAILED=1
fi

# --- Part 2: /out write success ---

echo ""
echo "--- Part 2: /out write success ---"

SPEC_RW="$(mktemp "${TMPDIR:-/tmp}/ploy-e2e-mount-rw.XXXXXX.yaml")"
trap 'rm -f "$IN_FIXTURE" "$SPEC_RO" "$SPEC_RW"' EXIT

# Create a seed file for the out record.
OUT_SEED="$(mktemp "${TMPDIR:-/tmp}/ploy-e2e-mount-seed.XXXXXX.txt")"
echo "seed" >"$OUT_SEED"
trap 'rm -f "$IN_FIXTURE" "$SPEC_RO" "$SPEC_RW" "$OUT_SEED"' EXIT

cat >"$SPEC_RW" <<YAML
apiVersion: ploy.mig/v1alpha1
kind: MigRunSpec
build_gate:
  disabled: true
steps:
  - image: ${E2E_IMAGE}
    command: >-
      sh -c '
        set -e;
        echo "writing to /out/result.txt...";
        echo "output-data" > /out/result.txt;
        cat /out/result.txt;
        echo "OK: /out write succeeded"
      '
    out:
      - ${OUT_SEED}:result.txt
YAML

RW_JSON="$(e2e_mig_run_json \
  "$SPEC_RW" \
  "$(e2e_repo_selector "$REPO" "$BASE_REF")" \
  --follow)" || true

printf '%s\n' "$RW_JSON" >"${E2E_ARTIFACT_DIR}/run-mount-rw.json"

RW_STATUS="$(printf '%s' "$RW_JSON" | jq -r '.repos[0].status // empty' 2>/dev/null || echo "")"
if [[ "$RW_STATUS" == "Success" ]]; then
  echo "  + /out write: run succeeded (writable mount confirmed)"
else
  echo "  ! /out write: expected Success, got '${RW_STATUS}'" >&2
  FAILED=1
fi

if ((FAILED > 0)); then
  echo ""
  echo "FAIL: scenario-hydra-mount-enforcement — assertions failed."
  echo "Artifacts saved to: ${E2E_ARTIFACT_DIR}"
  exit 1
fi

echo ""
echo "OK: scenario-hydra-mount-enforcement (in=read-only, out=writable)."
echo "Artifacts saved to: ${E2E_ARTIFACT_DIR}"
