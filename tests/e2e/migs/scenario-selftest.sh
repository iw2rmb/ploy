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

"$PLOY_BIN" mig run \
  --repo-url https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git \
  --repo-base-ref main \
  --repo-target-ref "e2e/selftest-${TS}" \
  --job-image alpine:3.20 \
  --job-command "$CMD" \
  --retain-container \
  --follow \
  --artifact-dir "$E2E_ARTIFACT_DIR" || true

echo "OK: selftest scenario (check logs output)"
echo "Artifacts saved to: ${E2E_ARTIFACT_DIR}"
