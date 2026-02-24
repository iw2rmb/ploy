#!/usr/bin/env bash
set -euo pipefail

# E2E: ORW apply Java 11->17; expect passing Build Gate.

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
export PLOY_CONFIG_HOME="${PLOY_CONFIG_HOME:-$REPO_ROOT/deploy/local/cli}"
source "$REPO_ROOT/tests/e2e/lib/ensure_local_descriptor.sh"
ensure_local_descriptor "$REPO_ROOT" "$PLOY_CONFIG_HOME"
: "${PLOY_CONTAINER_REGISTRY:?PLOY_CONTAINER_REGISTRY is required (example: ghcr.io/iw2rmb)}"

REPO=${PLOY_E2E_REPO_OVERRIDE:-https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git}
# Use a known-good remote ref for the passing scenario.
# "migs-upgrade-java17" may not exist by default; e2e/success does.
TARGET_REF=e2e/success

RECIPE_GROUP=org.openrewrite.recipe
RECIPE_ARTIFACT=rewrite-migrate-java
RECIPE_VERSION=3.20.0
RECIPE_CLASSNAME=org.openrewrite.java.migrate.UpgradeToJava17
MAVEN_PLUGIN_VERSION=6.18.0

# Artifacts directory: default to ./tmp/migs/orw-maven/<YYMMDDHHmmss>/
# override with PLOY_E2E_ARTIFACT_DIR or PLOY_E2E_ARTIFACT_BASE.
TS=$(date +%y%m%d%H%M%S)
ARTIFACT_BASE=${PLOY_E2E_ARTIFACT_BASE:-./tmp/migs/orw-maven}
ARTIFACT_DIR=${PLOY_E2E_ARTIFACT_DIR:-${ARTIFACT_BASE}/${TS}}
mkdir -p "${ARTIFACT_DIR}"


# Optional per-run GitLab PAT/domain flags
EXTRA_FLAGS=()
if [[ -n "${PLOY_GITLAB_PAT:-}" ]]; then
  EXTRA_FLAGS+=(--gitlab-pat "${PLOY_GITLAB_PAT}")
fi
if [[ -n "${PLOY_GITLAB_DOMAIN:-}" ]]; then
  EXTRA_FLAGS+=(--gitlab-domain "${PLOY_GITLAB_DOMAIN}")
fi

RUN=("$REPO_ROOT/dist/ploy" mig run --json \
  --repo-url "$REPO" \
  --repo-base-ref main \
  --repo-target-ref "$TARGET_REF" \
  --job-image "${PLOY_CONTAINER_REGISTRY}/migs-orw-maven:latest" \
  --job-env RECIPE_GROUP="$RECIPE_GROUP" \
  --job-env RECIPE_ARTIFACT="$RECIPE_ARTIFACT" \
  --job-env RECIPE_VERSION="$RECIPE_VERSION" \
  --job-env RECIPE_CLASSNAME="$RECIPE_CLASSNAME" \
  --job-env MAVEN_PLUGIN_VERSION="$MAVEN_PLUGIN_VERSION" \
  --mr-success \
  --follow \
  --artifact-dir "${ARTIFACT_DIR}" \
  "${EXTRA_FLAGS[@]}")

RUN=$("${RUN[@]}" | jq -r '.run_id')

# Print run summary if present; inspect subcommand has been removed.
if [[ -n "${RUN:-}" ]]; then
  "$REPO_ROOT/dist/ploy" run status "$RUN" || true
fi

echo "OK: orw-pass scenario"
echo "Artifacts saved to: ${ARTIFACT_DIR}"
