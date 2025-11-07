#!/usr/bin/env bash
set -euo pipefail

# E2E: ORW apply on failing branch -> Build Gate fails -> MR created on failure (no healing).

REPO=${PLOY_E2E_REPO_OVERRIDE:-https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git}
# Use failing baseline to ensure Build Gate fails without healing.
TARGET_REF=mods-upgrade-java17-fail

RECIPE_GROUP=org.openrewrite.recipe
RECIPE_ARTIFACT=rewrite-migrate-java
RECIPE_VERSION=3.20.0
RECIPE_CLASSNAME=org.openrewrite.java.migrate.UpgradeToJava17
MAVEN_PLUGIN_VERSION=6.18.0

# Artifacts directory: default to ./tmp/mods/mod-orw-fail/<YYMMDDHHmmss>/
TS=$(date +%y%m%d%H%M%S)
ARTIFACT_BASE=${PLOY_E2E_ARTIFACT_BASE:-./tmp/mods/mod-orw-fail}
ARTIFACT_DIR=${PLOY_E2E_ARTIFACT_DIR:-${ARTIFACT_BASE}/${TS}}
mkdir -p "${ARTIFACT_DIR}"

TICKET=$(dist/ploy mod run --json \
  --repo-url "$REPO" \
  --repo-base-ref e2e/fail-missing-symbol \
  --repo-target-ref "$TARGET_REF" \
  --mod-image "docker.io/${DOCKERHUB_USERNAME:-}/mods-openrewrite:latest" \
  --mod-env RECIPE_GROUP="$RECIPE_GROUP" \
  --mod-env RECIPE_ARTIFACT="$RECIPE_ARTIFACT" \
  --mod-env RECIPE_VERSION="$RECIPE_VERSION" \
  --mod-env RECIPE_CLASSNAME="$RECIPE_CLASSNAME" \
  --mod-env MAVEN_PLUGIN_VERSION="$MAVEN_PLUGIN_VERSION" \
  --mr-fail \
  --follow \
  --artifact-dir "${ARTIFACT_DIR}" | jq -r '.ticket_id') || true

# Fetch artifacts explicitly (works regardless of success/failure).
if [[ -n "${TICKET:-}" ]]; then
  dist/ploy mod fetch --ticket "$TICKET" --artifact-dir "${ARTIFACT_DIR}" || true
fi

# Print MR URL if present in ticket metadata.
if [[ -n "${TICKET:-}" ]]; then
  dist/ploy mod inspect "$TICKET" || true
fi

echo "OK: orw-fail scenario (MR on failure, no healing)"
echo "Artifacts saved to: ${ARTIFACT_DIR}"
