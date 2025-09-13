#!/bin/bash
# Smoke tests for /opt/hashicorp/bin/nomad-job-manager.sh
# Intended to run on the VPS (as user 'ploy' or via sudo root -> su - ploy)

set -e

BLUE='\033[0;34m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

pass() { echo -e "${GREEN}PASS${NC} - $1"; }
warn() { echo -e "${YELLOW}WARN${NC} - $1"; }
fail() { echo -e "${RED}FAIL${NC} - $1"; exit 1; }
info() { echo -e "${BLUE}$1${NC}"; }

WRAPPER=/opt/hashicorp/bin/nomad-job-manager.sh

info "Checking wrapper availability"
if [[ ! -x "$WRAPPER" ]]; then
  fail "Wrapper not found or not executable at $WRAPPER"
fi
pass "Wrapper exists and is executable"

info "Printing help"
if "$WRAPPER" help >/dev/null 2>&1; then
  pass "Help command works"
else
  fail "Help command failed"
fi

info "Validate command behavior"
# Expect failure on non-existent file
if "$WRAPPER" validate --file /does/not/exist.hcl >/dev/null 2>&1; then
  fail "Validate should fail for non-existent file"
else
  pass "Validate fails for non-existent file as expected"
fi

# Try validating a known HCL if present
JOB_FILE="tests/nomad-jobs/test-artifact-download.nomad"
if [[ -f "$JOB_FILE" ]]; then
  if "$WRAPPER" validate --file "$JOB_FILE" >/dev/null 2>&1; then
    pass "Validate passes for $JOB_FILE"
  else
    warn "Validate failed for $JOB_FILE (environment may lack Nomad CLI)"
  fi
else
  warn "Job file $JOB_FILE not found; skipping positive validate test"
fi

info "Status/allocs behavior (best-effort)"
# Query a job that probably exists in dev environments; do not fail the suite on absence
DEFAULT_JOB=${NOMAD_TEST_JOB:-ploy-api}
if "$WRAPPER" allocs --job "$DEFAULT_JOB" --format json >/dev/null 2>&1; then
  pass "allocs returned data for job $DEFAULT_JOB"
else
  warn "allocs failed or job $DEFAULT_JOB not present (acceptable in some environments)"
fi

info "All wrapper smoke tests completed"
exit 0

