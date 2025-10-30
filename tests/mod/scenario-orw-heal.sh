#!/usr/bin/env bash
set -euo pipefail

# E2E: ORW apply on failing branch -> initial Build Gate fails -> healing -> gate passes.

REPO=${PLOY_E2E_REPO_OVERRIDE:-https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git}
TARGET_REF=mods-upgrade-java17-heal

dist/ploy mod run \
  --repo-url "$REPO" \
  --repo-base-ref e2e/fail-missing-symbol \
  --repo-target-ref "$TARGET_REF" \
  --follow

echo "OK: orw-heal scenario"

