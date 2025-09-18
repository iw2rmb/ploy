#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

log() {
  printf '\n[INFO] %s\n' "$1"
}

ok() {
  printf '[OK] %s\n' "$1"
}

fail() {
  printf '[ERROR] %s\n' "$1" >&2
  exit 1
}

log "Running lane D image-size utilities unit tests"
if ! go test "$PROJECT_ROOT/internal/utils" -run TestGetImageSize -count=1; then
  fail "GetImageSize unit test failed"
fi
ok "GetImageSize covers Docker lane artifact sizing"

if ! go test "$PROJECT_ROOT/internal/utils" -run TestGetFileSize -count=1; then
  fail "getFileSize unit test failed"
fi
ok "getFileSize unit test passed"

log "Ensuring Docker lane default is used in size helpers"
SIZE_HELPER="$PROJECT_ROOT/internal/utils/image_size.go"
if grep -Eq 'Lane.*"[A-CE-Z]"' "$SIZE_HELPER"; then
  fail "Found references to removed lanes in image_size.go"
fi
ok "image_size.go only references Docker lane values"

log "Lane D size cap unit checks complete"
