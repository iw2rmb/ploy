#!/bin/bash

# OpenRewrite Workspace Setup Script (SeaweedFS-only IO)
# - Always download input.tar from SeaweedFS via INPUT_URL
# - Extract into /workspace/project
# - Delegate transformation and diff upload to /usr/local/bin/openrewrite

set -euo pipefail

echo "[SETUP] Starting OpenRewrite workspace setup (SeaweedFS mode)..."
echo "[SETUP] Current directory: $(pwd)"
echo "[SETUP] User: $(whoami)"

WORKSPACE_DIR="${WORKSPACE_DIR:-/workspace}"
SKIP_EXEC_OPENREWRITE="${SKIP_EXEC_OPENREWRITE:-}"

mkdir -p "${WORKSPACE_DIR}" "${WORKSPACE_DIR}/project"

if [ -z "${INPUT_URL:-}" ]; then
  echo "[SETUP] ERROR: INPUT_URL not provided; cannot download input.tar"
  exit 1
fi

echo "[SETUP] Downloading input.tar from INPUT_URL=${INPUT_URL}..."
set +e
RESP=$(curl -sSL --connect-timeout 30 --max-time 300 -w "HTTP_CODE:%{http_code}" -o "${WORKSPACE_DIR}/input.tar" "${INPUT_URL}")
RC=$?
set -e
echo "[SETUP] INPUT_URL download result: rc=${RC} ${RESP}"
if [ $RC -ne 0 ] || ! echo "$RESP" | grep -q "HTTP_CODE:200"; then
  echo "[SETUP] ERROR: Failed to download input.tar from INPUT_URL"
  exit 1
fi
ls -lh "${WORKSPACE_DIR}/input.tar" || true

echo "[SETUP] Extracting input.tar into ${WORKSPACE_DIR}/project..."
rm -rf "${WORKSPACE_DIR}/project" && mkdir -p "${WORKSPACE_DIR}/project"
tar -xf "${WORKSPACE_DIR}/input.tar" -C "${WORKSPACE_DIR}/project" || {
  echo "[SETUP] ERROR: Failed to extract input.tar"
  exit 1
}

echo "[SETUP] Project directory contents (top-level):"
ls -la "${WORKSPACE_DIR}/project" | head -50 || true

chown -R $(whoami):$(whoami) "${WORKSPACE_DIR}/" 2>/dev/null || true
chmod -R 755 "${WORKSPACE_DIR}/" 2>/dev/null || true

echo "[SETUP] Workspace setup complete! Starting OpenRewrite transformation..."
if [ "$SKIP_EXEC_OPENREWRITE" = "1" ]; then exit 0; fi
exec /usr/local/bin/openrewrite
