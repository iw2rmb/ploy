#!/usr/bin/env bash
set -euo pipefail

# E2E: ORW apply Java 11->17; expect passing Build Gate.

REPO=${PLOY_E2E_REPO_OVERRIDE:-https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git}
# Use a known-good remote ref for the passing scenario.
# "mods-upgrade-java17" may not exist by default; e2e/success does.
TARGET_REF=e2e/success

RECIPE_GROUP=org.openrewrite.recipe
RECIPE_ARTIFACT=rewrite-migrate-java
RECIPE_VERSION=3.20.0
RECIPE_CLASSNAME=org.openrewrite.java.migrate.UpgradeToJava17
MAVEN_PLUGIN_VERSION=6.18.0

# Artifacts directory: default to ./tmp/mods/mod-orw/<YYMMDDHHmmss>/
# override with PLOY_E2E_ARTIFACT_DIR or PLOY_E2E_ARTIFACT_BASE.
TS=$(date +%y%m%d%H%M%S)
ARTIFACT_BASE=${PLOY_E2E_ARTIFACT_BASE:-./tmp/mods/mod-orw}
ARTIFACT_DIR=${PLOY_E2E_ARTIFACT_DIR:-${ARTIFACT_BASE}/${TS}}
mkdir -p "${ARTIFACT_DIR}"

dist/ploy mod run \
  --repo-url "$REPO" \
  --repo-base-ref main \
  --repo-target-ref "$TARGET_REF" \
  --mod-image "docker.io/${DOCKERHUB_USERNAME:-}/mods-openrewrite:latest" \
  --mod-env RECIPE_GROUP="$RECIPE_GROUP" \
  --mod-env RECIPE_ARTIFACT="$RECIPE_ARTIFACT" \
  --mod-env RECIPE_VERSION="$RECIPE_VERSION" \
  --mod-env RECIPE_CLASSNAME="$RECIPE_CLASSNAME" \
  --mod-env MAVEN_PLUGIN_VERSION="$MAVEN_PLUGIN_VERSION" \
  --mr-success \
  --follow \
  --artifact-dir "${ARTIFACT_DIR}"

echo "OK: orw-pass scenario"
echo "Artifacts saved to: ${ARTIFACT_DIR}"
