#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

info() {
  printf '\n[INFO] %s\n' "$1"
}

pass() {
  printf '[OK] %s\n' "$1"
}

fail() {
  printf '[ERROR] %s\n' "$1" >&2
  exit 1
}

info "Validating Docker lane resource caps"
RESOURCE_FILE="$PROJECT_ROOT/internal/build/resources.go"
if [[ ! -f "$RESOURCE_FILE" ]]; then
  fail "internal/build/resources.go not found"
fi

if ! grep -q 'case "D"' "$RESOURCE_FILE"; then
  fail "Lane D resource mapping missing in resources.go"
fi

if grep -Eq 'case\s+"[A-CE-Z]"' "$RESOURCE_FILE"; then
  fail "Detected stale lane mappings in resources.go; only Lane D should remain"
fi
pass "resources.go only defines Docker lane limits"

info "Running focused Go test for Lane D resources"
if ! go test "$PROJECT_ROOT/internal/build" -run TestLaneResourceForDocker -count=1; then
  fail "Go resource test failed"
fi
pass "Go resource test confirmed lane limits"

info "Verifying OPA policies reference Docker lane size cap"
OPA_FILE="$PROJECT_ROOT/api/opa/verify.go"
if [[ -f "$OPA_FILE" ]]; then
  if grep -q 'LaneSizeCaps' "$OPA_FILE"; then
    pass "OPA verify.go references lane size caps"
  else
    info "OPA verify.go does not reference lane size caps; skipping"
  fi
else
  info "OPA policy verification file not found; skipping"
fi

info "Docker lane size cap checks complete"
