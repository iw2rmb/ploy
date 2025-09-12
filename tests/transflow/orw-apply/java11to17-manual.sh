#!/usr/bin/env bash
set -euo pipefail

# Manual Java 11 → 17 upgrade using OpenRewrite on VPS
# Usage:
#   TARGET_HOST=45.12.75.241 ./java11to17-manual.sh
# Optional env:
#   RECIPE_CLASS=org.openrewrite.java.migrate.UpgradeToJava17
#   RECIPE_COORDS=org.openrewrite.recipe:rewrite-migrate-java:3.17.0
#   MAVEN_PLUGIN_VERSION=6.18.0
#   REPO_URL=https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git

TARGET_HOST=${TARGET_HOST:-45.12.75.241}
RECIPE_CLASS=${RECIPE_CLASS:-org.openrewrite.java.migrate.UpgradeToJava17}
RECIPE_COORDS=${RECIPE_COORDS:-org.openrewrite.recipe:rewrite-migrate-java:3.17.0}
MAVEN_PLUGIN_VERSION=${MAVEN_PLUGIN_VERSION:-6.18.0}
REPO_URL=${REPO_URL:-https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git}

echo "Connecting to ${TARGET_HOST} and running OpenRewrite..."
ssh -o StrictHostKeyChecking=no root@"${TARGET_HOST}" RECIPE_CLASS="${RECIPE_CLASS}" RECIPE_COORDS="${RECIPE_COORDS}" MAVEN_PLUGIN_VERSION="${MAVEN_PLUGIN_VERSION}" REPO_URL="${REPO_URL}" bash -s <<'EOSH'
set -euo pipefail

# Env passed via ssh invocation: RECIPE_CLASS, RECIPE_COORDS, MAVEN_PLUGIN_VERSION, REPO_URL

su ploy <<'EOF'
set -euo pipefail
BASE="$HOME"
mkdir -p "$BASE/tmp"
WORKDIR=$(mktemp -d "$BASE/tmp/java11to17.XXXXXX")
echo "Using WORKDIR=$WORKDIR"
cd "$WORKDIR"

echo "Cloning repo..."
GIT_SSL_NO_VERIFY=true git clone --depth 1 "$REPO_URL" repo
cd repo

echo "Java version:"; (java -version || true) 2>&1 | head -n2
echo "Maven version:"; (mvn -v || true) 2>&1 | head -n1

echo "Running OpenRewrite Maven plugin - UpgradeToJava17..."
mvn -ntp -U \
  org.openrewrite.maven:rewrite-maven-plugin:"$MAVEN_PLUGIN_VERSION":run \
  -Drewrite.recipeArtifactCoordinates="$RECIPE_COORDS" \
  -Drewrite.activeRecipes="$RECIPE_CLASS" \
  -Drewrite.failOnInvalidActiveRecipes=false || true

echo "STATUS:"; git status --porcelain
echo "SHORTSTAT:"; git diff --shortstat || true
echo "Saving diff to $BASE/java11to17-latest.diff"
git diff > "$BASE/java11to17-latest.diff" || true
echo -n "DIFF BYTES: "; wc -c < "$BASE/java11to17-latest.diff"
EOF
EOSH

echo "Done. Retrieve diff from VPS: /home/ploy/java11to17-latest.diff"
