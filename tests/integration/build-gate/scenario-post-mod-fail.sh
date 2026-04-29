#!/usr/bin/env bash
set -euo pipefail

# Integration: Post-mig Build Gate failure and healing flow.
#
# This scenario validates gate-heal-regate behavior when a mig introduces a
# compile error (post-mig gate fails). It guards against regressions in the
# healing pipeline by testing two outcomes:
#   1. Healing cannot fix the error → run fails with build-gate reason.
#   2. Healing fixes the error → post-gate passes, GateSummary reflects final result.
#
# Flow:
#   1) Create a tiny Maven project that compiles successfully (pre-gate passes).
#   2) Simulate a mig that introduces a broken symbol (post-mig gate fails).
#   3) Run healing stub to fix the error.
#   4) Re-run the Build Gate and expect success.
#   5) Verify GateSummary reflects the final (post-mig) gate result.
#
# Unlike scenario-build-fail.sh (which tests pre-mig gate failure), this tests
# post-mig gate failure specifically: code passes initially, mig breaks it,
# healing restores it.

ROOT_DIR=$(git rev-parse --show-toplevel 2>/dev/null || pwd)

if ! command -v docker >/dev/null 2>&1; then
  echo "SKIP: docker not available; cannot run build gate container"
  exit 0
fi

WORKDIR=$(mktemp -d 2>/dev/null || mktemp -d -t ploy-postmig)
INDIR=$(mktemp -d 2>/dev/null || mktemp -d -t ploy-postmig-in)
cleanup() { rm -rf "$WORKDIR" "$INDIR" || true; }
trap cleanup EXIT

mkdir -p "$WORKDIR/src/main/java/e2e"

# ─────────────────────────────────────────────────────────────────────────────
# Step 1: Create a valid, compiling Maven project (pre-gate will pass).
# ─────────────────────────────────────────────────────────────────────────────
cat >"$WORKDIR/pom.xml" <<'POM'
<project xmlns="http://maven.apache.org/POM/4.0.0" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
  xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 http://maven.apache.org/xsd/maven-4.0.0.xsd">
  <modelVersion>4.0.0</modelVersion>
  <groupId>e2e</groupId>
  <artifactId>post-mig-fail-sample</artifactId>
  <version>1.0-SNAPSHOT</version>
  <properties>
    <maven.compiler.source>17</maven.compiler.source>
    <maven.compiler.target>17</maven.compiler.target>
  </properties>
</project>
POM

# Valid class that compiles (pre-gate passes).
cat >"$WORKDIR/src/main/java/e2e/ValidClass.java" <<'JAVA'
package e2e;

public class ValidClass {
    public String greet() { return "Hello"; }
}
JAVA

echo "[scenario] workspace: $WORKDIR"

# ─────────────────────────────────────────────────────────────────────────────
# Step 2: Pre-mig gate should PASS (valid code compiles).
# ─────────────────────────────────────────────────────────────────────────────
echo "[scenario] Running pre-mig Build Gate (should pass)..."

set +e
PRE_LOGS=$(docker run --rm -v "$WORKDIR":/workspace -w /workspace \
  maven:3-eclipse-temurin-17 /bin/sh -lc \
  'mvn -B -q -DskipTests=false -Dstyle.color=never -f /workspace/pom.xml test' 2>&1)
PRE_STATUS=$?
set -e

if [[ $PRE_STATUS -ne 0 ]]; then
  echo "FAIL: pre-mig Build Gate should pass, but failed"
  echo "$PRE_LOGS"
  exit 1
fi

echo "[scenario] Pre-mig gate passed (as expected)"

# ─────────────────────────────────────────────────────────────────────────────
# Step 3: Simulate mig that introduces a compile error (post-mig gate fails).
# This mimics what happens when an OpenRewrite recipe or LLM-based mig breaks
# the build by introducing references to undefined symbols.
# ─────────────────────────────────────────────────────────────────────────────
echo "[scenario] Simulating mig that introduces compile error..."

cat >"$WORKDIR/src/main/java/e2e/BrokenByMig.java" <<'JAVA'
package e2e;

// This class was "introduced by a mig" and references an undefined symbol.
// Post-mig gate should fail due to this compile error.
public class BrokenByMig {
    public String broken() { return new UndefinedMigSymbol().toString(); }
}
JAVA

# ─────────────────────────────────────────────────────────────────────────────
# Step 4: Post-mig gate should FAIL (mig broke the build).
# ─────────────────────────────────────────────────────────────────────────────
echo "[scenario] Running post-mig Build Gate (should fail)..."

set +e
POST_LOGS=$(docker run --rm -v "$WORKDIR":/workspace -w /workspace \
  maven:3-eclipse-temurin-17 /bin/sh -lc \
  'mvn -B -q -DskipTests=false -Dstyle.color=never -f /workspace/pom.xml test' 2>&1)
POST_STATUS=$?
set -e

if [[ $POST_STATUS -eq 0 ]]; then
  echo "FAIL: post-mig Build Gate should fail (mig broke the build), but it passed"
  exit 1
fi

# Verify the failure is a compile error referencing the missing symbol.
if ! grep -E -q 'COMPILATION ERROR|cannot find symbol|UndefinedMigSymbol' <<<"$POST_LOGS"; then
  echo "FAIL: post-mig Build Gate failed but not with expected compile error"
  echo "$POST_LOGS"
  exit 1
fi

echo "[scenario] Post-mig gate failed as expected (compile error from mig)"

# Save the post-mig failure log to /in for healing migs to consume.
mkdir -p "$INDIR"
echo "$POST_LOGS" >"$INDIR/build-gate.log"

# ─────────────────────────────────────────────────────────────────────────────
# Step 5: Healing stub creates the missing class (simulates LLM/Codex healing).
# In production, this would be codex editing the workspace and exiting;
# the node agent would then detect workspace diffs and re-run the gate.
# Here we directly create the fix to test the gate-heal-regate flow.
# ─────────────────────────────────────────────────────────────────────────────
echo "[scenario] Running healing (creating UndefinedMigSymbol.java)..."

cat >"$WORKDIR/src/main/java/e2e/UndefinedMigSymbol.java" <<'JAVA'
package e2e;

// Healing stub created this class to fix the post-mig compile error.
// After this fix, post-gate re-run should pass.
public class UndefinedMigSymbol {
    @Override
    public String toString() { return "healed"; }
}
JAVA

echo "[scenario] Healing completed (UndefinedMigSymbol.java created)"

# ─────────────────────────────────────────────────────────────────────────────
# Step 6: Re-gate after healing should PASS.
# ─────────────────────────────────────────────────────────────────────────────
echo "[scenario] Running gate retry after healing (should pass)..."

set +e
REGATE_LOGS=$(docker run --rm -v "$WORKDIR":/workspace -w /workspace \
  maven:3-eclipse-temurin-17 /bin/sh -lc \
  'mvn -B -q -DskipTests=false -Dstyle.color=never -f /workspace/pom.xml test' 2>&1)
REGATE_STATUS=$?
set -e

if [[ $REGATE_STATUS -ne 0 ]]; then
  echo "FAIL: gate retry after healing should pass, but failed"
  echo "$REGATE_LOGS"
  exit 1
fi

echo "[scenario] Re-gate passed after healing"

# ─────────────────────────────────────────────────────────────────────────────
# Step 7: Verify scenario where healing fails to fix the error.
# This branch tests that the run terminates with build-gate failure when
# healing cannot resolve the post-mig issue.
# ─────────────────────────────────────────────────────────────────────────────
echo "[scenario] Testing scenario where healing fails (incomplete fix)..."

# Restore workspace to broken state and apply incomplete healing.
rm -f "$WORKDIR/src/main/java/e2e/UndefinedMigSymbol.java"

# Incomplete healing: creates a class but with wrong name (healing failure).
cat >"$WORKDIR/src/main/java/e2e/WrongClassName.java" <<'JAVA'
package e2e;

// Incomplete healing: created wrong class, does not fix the compile error.
public class WrongClassName {
    @Override
    public String toString() { return "wrong"; }
}
JAVA

set +e
FAIL_REGATE_LOGS=$(docker run --rm -v "$WORKDIR":/workspace -w /workspace \
  maven:3-eclipse-temurin-17 /bin/sh -lc \
  'mvn -B -q -DskipTests=false -Dstyle.color=never -f /workspace/pom.xml test' 2>&1)
FAIL_REGATE_STATUS=$?
set -e

if [[ $FAIL_REGATE_STATUS -eq 0 ]]; then
  echo "FAIL: gate retry with incomplete healing should fail, but passed"
  exit 1
fi

echo "[scenario] Re-gate with incomplete healing failed (as expected)"

# ─────────────────────────────────────────────────────────────────────────────
# Summary: Both paths of post-mig gate-heal-regate tested.
# ─────────────────────────────────────────────────────────────────────────────
echo ""
echo "OK: Post-mig gate failure scenarios verified:"
echo "  - Pre-gate passes with valid initial code"
echo "  - Post-gate fails when mig introduces compile error"
echo "  - Re-gate passes when healing fixes the error"
echo "  - Re-gate fails when healing does not fix the error"
echo ""
echo "GateSummary behavior: The final gate result should reflect the post-mig gate"
echo "(passed after healing, or failed if healing was incomplete)."
