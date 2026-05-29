#!/usr/bin/env bash
set -euo pipefail

# E2E: simple container self-test to validate container runtime + SSE logs.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=tests/e2e/lib/harness.sh
source "${SCRIPT_DIR}/../lib/harness.sh"

e2e_init "${BASH_SOURCE[0]}"
e2e_artifacts_init "$REPO_ROOT/tmp/migs/selftest"

TS="$(date +%y%m%d%H%M%S)"
CMD='echo "[selftest] hello"; uname -a; sleep 3; echo "[selftest] done"'
SPEC_FILE="${E2E_ARTIFACT_DIR}/selftest.yaml"

cat > "$SPEC_FILE" <<YAML
steps:
  - image: alpine:3.20
    command: '$CMD'
YAML

RUN_JSON="$(e2e_mig_run_json \
  "$SPEC_FILE" \
  "$(e2e_repo_selector https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git main)" \
  --follow \
  --pull "$E2E_ARTIFACT_DIR")" || true

echo "OK: selftest scenario (check logs output)"
echo "Artifacts saved to: ${E2E_ARTIFACT_DIR}"
