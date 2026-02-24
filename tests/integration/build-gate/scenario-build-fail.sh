#!/usr/bin/env bash
set -euo pipefail

# Integration: Build Gate failure produces an error artifact consumable by LLM mig.
# Flow:
#  1) Create a tiny Maven project that fails to compile (missing symbol).
#  2) Run the Build Gate container (maven:3-eclipse-temurin-17) and capture logs.
#  3) Save logs to /in/build-gate.log (artifact to pass to LLM; /in is read-only cross-phase input).
#  4) Run migs/mig-llm stub to heal the missing symbol.
#  5) Re-run the Build Gate and expect success.

ROOT_DIR=$(git rev-parse --show-toplevel 2>/dev/null || pwd)

if ! command -v docker >/dev/null 2>&1; then
  echo "SKIP: docker not available; cannot run build gate container"
  exit 0
fi

WORKDIR=$(mktemp -d 2>/dev/null || mktemp -d -t ploy-buildgate)
INDIR=$(mktemp -d 2>/dev/null || mktemp -d -t ploy-buildgate-in)
cleanup() { rm -rf "$WORKDIR" "$INDIR" || true; }
trap cleanup EXIT

mkdir -p "$WORKDIR/src/main/java/e2e"

cat >"$WORKDIR/pom.xml" <<'POM'
<project xmlns="http://maven.apache.org/POM/4.0.0" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
  xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 http://maven.apache.org/xsd/maven-4.0.0.xsd">
  <modelVersion>4.0.0</modelVersion>
  <groupId>e2e</groupId>
  <artifactId>build-fail-sample</artifactId>
  <version>1.0-SNAPSHOT</version>
  <properties>
    <maven.compiler.source>17</maven.compiler.source>
    <maven.compiler.target>17</maven.compiler.target>
  </properties>
</project>
POM

# Failing class referencing UnknownClass (healed by migs-llm stub)
cat >"$WORKDIR/src/main/java/e2e/FailMissingSymbol.java" <<'JAVA'
package e2e;

public class FailMissingSymbol {
    public String s() { return new UnknownClass().toString(); }
}
JAVA

echo "[scenario] workspace: $WORKDIR"

# Run Build Gate (Maven test) expecting failure; capture logs
set +e
LOGS=$(docker run --rm -v "$WORKDIR":/workspace -w /workspace \
  maven:3-eclipse-temurin-17 /bin/sh -lc \
  'mvn -B -q -DskipTests=false -Dstyle.color=never -f /workspace/pom.xml test' 2>&1)
STATUS=$?
set -e

if [[ $STATUS -eq 0 ]]; then
  echo "FAIL: expected Build Gate to fail, but it succeeded"
  exit 1
fi

mkdir -p "$INDIR"
echo "$LOGS" >"$INDIR/build-gate.log"

if ! grep -E -q 'COMPILATION ERROR|cannot find symbol' "$INDIR/build-gate.log"; then
  echo "FAIL: build-gate.log does not contain a compilation error"
  exit 1
fi

echo "[scenario] Build Gate failed as expected; artifact saved to /in/build-gate.log"

# Run LLM healer stub (migs/mig-llm) to create e2e/UnknownClass.java
OUTDIR="$WORKDIR/out"
mkdir -p "$OUTDIR"
bash "$ROOT_DIR/migs/mig-llm/mig-llm.sh" --execute --input "$WORKDIR" --out "$OUTDIR/plan.json"

if [[ ! -f "$WORKDIR/src/main/java/e2e/UnknownClass.java" ]]; then
  echo "FAIL: LLM healer did not create UnknownClass.java"
  exit 1
fi

echo "[scenario] LLM healing created e2e/UnknownClass.java"

# Re-run Build Gate; expect success
docker run --rm -v "$WORKDIR":/workspace -w /workspace \
  maven:3-eclipse-temurin-17 /bin/sh -lc \
  'mvn -B -q -DskipTests=false -Dstyle.color=never -f /workspace/pom.xml test' >/dev/null 2>&1

echo "OK: build-gate failure produces artifact and can be healed by LLM stub"
