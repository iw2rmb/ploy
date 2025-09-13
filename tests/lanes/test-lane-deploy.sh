#!/usr/bin/env bash
# Lane-aware wrapper for tests/e2e/test-deploy-github-app.sh
# Requires: HELLO_APP_REPO, optional LANE (A-G), APP_NAME, BRANCH, HEALTH_PATH

set -euo pipefail

BLUE='\033[0;34m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; RED='\033[0;31m'; NC='\033[0m'
info(){ echo -e "${BLUE}$*${NC}"; }
ok(){ echo -e "${GREEN}$*${NC}"; }
warn(){ echo -e "${YELLOW}$*${NC}"; }
err(){ echo -e "${RED}$*${NC}"; }

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/../.." && pwd)

if [[ -z "${HELLO_APP_REPO:-}" ]]; then
  err "HELLO_APP_REPO is required (e.g., https://github.com/iw2rmb/ploy-lane-a-go.git)"
  exit 1
fi

LANE=${LANE:-}
if [[ -n "$LANE" ]]; then
  case "$LANE" in
    A|B|C|D|E|F|G) ;;
    *) err "Invalid LANE: $LANE (expected A-G)"; exit 1;;
  esac
  export LANE
  info "Forcing lane: $LANE"
fi

export HEALTH_PATH=${HEALTH_PATH:-/healthz}

info "Invoking core E2E deploy script"
"$REPO_ROOT/tests/e2e/test-deploy-github-app.sh"

ok "Lane $LANE deployment script completed"

