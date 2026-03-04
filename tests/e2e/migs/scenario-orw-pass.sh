#!/usr/bin/env bash
set -euo pipefail

# E2E: ORW apply Java 11->17; expect passing Build Gate.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=tests/e2e/lib/harness.sh
source "${SCRIPT_DIR}/../lib/harness.sh"

e2e_init "${BASH_SOURCE[0]}"
e2e_artifacts_init "$REPO_ROOT/tmp/migs/orw-cli"

: "${PLOY_CONTAINER_REGISTRY:?PLOY_CONTAINER_REGISTRY is required (example: localhost:5000/ploy)}"

REPO="${PLOY_E2E_REPO_OVERRIDE:-https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git}"
TARGET_REF="e2e/success"

RECIPE_GROUP="org.openrewrite.recipe"
RECIPE_ARTIFACT="rewrite-migrate-java"
RECIPE_VERSION="3.20.0"
RECIPE_CLASSNAME="org.openrewrite.java.migrate.UpgradeToJava17"

RUN_JSON="$(e2e_mig_run_json \
  --repo-url "$REPO" \
  --repo-base-ref main \
  --repo-target-ref "$TARGET_REF" \
  --job-image "${PLOY_CONTAINER_REGISTRY}/orw-cli-maven:latest" \
  --job-env RECIPE_GROUP="$RECIPE_GROUP" \
  --job-env RECIPE_ARTIFACT="$RECIPE_ARTIFACT" \
  --job-env RECIPE_VERSION="$RECIPE_VERSION" \
  --job-env RECIPE_CLASSNAME="$RECIPE_CLASSNAME" \
  --mr-success \
  --follow)"
RUN_ID="$(e2e_mig_run_id "$RUN_JSON")"

if [[ -z "$RUN_ID" ]]; then
  echo "error: failed to parse run_id from mig run output" >&2
  printf '%s\n' "$RUN_JSON" >&2
  exit 1
fi

e2e_run_status_safe "$RUN_ID"

echo "OK: orw-pass scenario"
echo "Artifacts saved to: ${E2E_ARTIFACT_DIR}"
