#!/usr/bin/env bash
set -euo pipefail

# E2E: ORW apply Java 11->17; expect passing Build Gate.

REPO=${PLOY_E2E_REPO_OVERRIDE:-https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git}
TARGET_REF=mods-upgrade-java17

dist/ploy mod run \
  --repo-url "$REPO" \
  --repo-base-ref main \
  --repo-target-ref "$TARGET_REF" \
  --follow

echo "OK: orw-pass scenario"

