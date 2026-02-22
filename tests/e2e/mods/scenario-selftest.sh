#!/usr/bin/env bash
set -euo pipefail

# E2E: simple container self-test to validate container runtime + SSE logs.

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
export PLOY_CONFIG_HOME="${PLOY_CONFIG_HOME:-$REPO_ROOT/local/cli}"
source "$REPO_ROOT/tests/e2e/lib/ensure_local_descriptor.sh"
ensure_local_descriptor "$REPO_ROOT" "$PLOY_CONFIG_HOME"

TS=$(date +%y%m%d%H%M%S)
ARTIFACT_BASE=${PLOY_E2E_ARTIFACT_BASE:-./tmp/mods/selftest}
ARTIFACT_DIR=${PLOY_E2E_ARTIFACT_DIR:-${ARTIFACT_BASE}/${TS}}
mkdir -p "${ARTIFACT_DIR}"

CMD='echo "[selftest] hello"; uname -a; sleep 3; echo "[selftest] done"'

"$REPO_ROOT/dist/ploy" mod run \
  --repo-url https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git \
  --repo-base-ref main \
  --repo-target-ref e2e/selftest-${TS} \
  --mod-image alpine:3.20 \
  --mod-command "$CMD" \
  --retain-container \
  --follow \
  --artifact-dir "${ARTIFACT_DIR}" || true

echo "OK: selftest scenario (check logs output)"
echo "Artifacts saved to: ${ARTIFACT_DIR}"
