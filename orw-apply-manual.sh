#!/usr/bin/env bash
set -euo pipefail

# Manual OpenRewrite (orw-apply) runner on VPS
# - Always clones a fresh baseline (single-branch, shallow) and uploads a new input.tar
# - Runs the latest openrewrite-jvm image on the VPS
# - Prints DIFF_URL, headers, size, and a head preview if non-empty
#
# Usage:
#   TARGET_HOST=45.12.75.241 ./orw-apply-manual.sh
# Optional env overrides:
#   RECIPE_CLASS=org.openrewrite.java.migrate.UpgradeToJava17
#   RECIPE_GROUP=org.openrewrite.recipe
#   RECIPE_ARTIFACT=rewrite-migrate-java
#   RECIPE_VERSION=3.17.0
#   MAVEN_PLUGIN_VERSION=6.18.0
#   REPO_URL=https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git

TARGET_HOST=${TARGET_HOST:-45.12.75.241}
REPO_URL=${REPO_URL:-https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git}

RECIPE_CLASS=${RECIPE_CLASS:-org.openrewrite.java.migrate.UpgradeToJava17}
RECIPE_GROUP=${RECIPE_GROUP:-org.openrewrite.recipe}
RECIPE_ARTIFACT=${RECIPE_ARTIFACT:-rewrite-migrate-java}
RECIPE_VERSION=${RECIPE_VERSION:-3.17.0}
MAVEN_PLUGIN_VERSION=${MAVEN_PLUGIN_VERSION:-6.18.0}

ssh -o ServerAliveInterval=30 -o ServerAliveCountMax=4 -o StrictHostKeyChecking=no root@"${TARGET_HOST}" 'bash -s' <<'EOSH'
set -euo pipefail

SEAWEED=http://seaweedfs-filer.service.consul:8888
TS=$(date +%s)
WORKDIR="/home/ploy/tmp/manual-input-$TS"
INPUT_KEY="transflow/manual-input/$TS/input.tar"
INPUT_TAR_URL="$SEAWEED/artifacts/$INPUT_KEY"

echo "Preparing fresh baseline in $WORKDIR"
su - ploy -c "mkdir -p '$WORKDIR' && cd '$WORKDIR' && \
  GIT_SSL_NO_VERIFY=true git clone --single-branch --depth 1 ${REPO_URL} repo && \
  tar -C repo -cf input.tar ."

echo "Uploading input.tar to $INPUT_TAR_URL"
curl -sS -X PUT "$INPUT_TAR_URL" \
  --data-binary @"$WORKDIR/input.tar" \
  -H "Content-Type: application/octet-stream" \
  -w "HTTP_CODE:%{http_code}\n" -o /dev/null

echo "Verify input tar:"
curl -sI "$INPUT_TAR_URL" | head -n1

DIFF_KEY="transflow/manual-test/$TS/diff.patch"
IMG="registry.dev.ployman.app/openrewrite-jvm:latest"

echo "Running container: $IMG"
/usr/bin/docker run --rm --network host \
  -e INPUT_URL="$INPUT_TAR_URL" \
  -e RECIPE="${RECIPE_CLASS}" \
  -e RECIPE_GROUP="${RECIPE_GROUP}" \
  -e RECIPE_ARTIFACT="${RECIPE_ARTIFACT}" \
  -e RECIPE_VERSION="${RECIPE_VERSION}" \
  -e MAVEN_PLUGIN_VERSION="${MAVEN_PLUGIN_VERSION}" \
  -e SEAWEEDFS_URL="$SEAWEED" \
  -e DIFF_KEY="$DIFF_KEY" \
  "$IMG" >/tmp/manual-orw-force.log 2>&1 || true

URL="$SEAWEED/artifacts/$DIFF_KEY"
echo "DIFF_URL: $URL"
curl -sI "$URL" | sed -n '1,12p'
BYTES=$(curl -s "$URL" | wc -c)
echo "BYTES: $BYTES"

echo '--- diff lines ---'
grep -n "Generating unified diff patch\|diff size\|diff head preview\|diff.patch upload response" /tmp/manual-orw-force.log | tail -n 200 || true
if [ "$BYTES" -gt 0 ]; then
  echo '--- DIFF HEAD ---'
  curl -s "$URL" | sed -n '1,80p'
fi
EOSH

echo "Done. Check the VPS output above for DIFF_URL and logs."

