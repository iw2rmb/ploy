#!/usr/bin/env bash
# Multi-step migs E2E scenario runner
#
# This script demonstrates submission of a multi-step mig spec with sequential
# transformation steps (Java 8 → Java 11 → Java 17). The spec uses the migs[]
# array format with global build_gate and build_gate_healing policy.
#
# Prerequisites:
# - ploy binary available at dist/ploy or ./dist/ploy
# - Access to the target repository
# - Codex auth configured (if using healing with Codex)
#
# Usage:
#   cd tests/e2e/migs/scenario-multi-step
#   ./run.sh

set -euo pipefail

# Default to the local Docker cluster descriptor written by deploy/local/run.sh.
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
export PLOY_CONFIG_HOME="${PLOY_CONFIG_HOME:-$REPO_ROOT/deploy/local/cli}"
source "$REPO_ROOT/tests/e2e/lib/ensure_local_descriptor.sh"
ensure_local_descriptor "$REPO_ROOT" "$PLOY_CONFIG_HOME"

# Locate ploy binary (check both dist/ploy and ./dist/ploy)
PLOY_BIN=""
if [[ -x "../../../../dist/ploy" ]]; then
  PLOY_BIN="../../../../dist/ploy"
elif [[ -x "./dist/ploy" ]]; then
  PLOY_BIN="./dist/ploy"
else
  echo "Error: ploy binary not found at dist/ploy or ./dist/ploy" >&2
  exit 1
fi

# Configuration (override via environment variables)
REPO_URL="${REPO_URL:-https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git}"
REPO_BASE_REF="${REPO_BASE_REF:-main}"
REPO_TARGET_REF="${REPO_TARGET_REF:-java6-multy-mig}"
SPEC_FILE="$(dirname "$0")/mig.yaml"

echo "=========================================="
echo "Multi-step Mods E2E Scenario"
echo "=========================================="
echo "Repo URL:        $REPO_URL"
echo "Base ref:        $REPO_BASE_REF"
echo "Target ref:      $REPO_TARGET_REF"
echo "Spec file:       $SPEC_FILE"
echo "PLOY_CONFIG_HOME: $PLOY_CONFIG_HOME"
echo "=========================================="

# Submit the multi-step mig run with --follow to stream logs
"$PLOY_BIN" mig run \
  --repo-url "$REPO_URL" \
  --repo-base-ref "$REPO_BASE_REF" \
  --repo-target-ref "$REPO_TARGET_REF" \
  --spec "$SPEC_FILE" \
  --follow

echo ""
echo "Multi-step mig run completed."
echo "Check the logs above for per-step execution status and diffs."
