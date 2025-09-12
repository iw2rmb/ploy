#!/usr/bin/env bash
set -euo pipefail

# Prepare scenario branches in a single Git repo:
#  - e2e/success              (no changes; compiles after orw-apply)
#  - e2e/fail-missing-symbol  (adds a class referencing an unknown symbol)
#  - e2e/fail-java17-specific (adds a class referencing Nashorn API removed in 17)
#
# Usage:
#   ./prepare-branches.sh <repo_url> [base_branch] [--verify]
#
# Notes:
# - Requires local git credentials with push access.
# - For HTTPS with PAT, you can embed the token in the URL or rely on credential helpers.
# - Optionally set GIT_AUTHOR_NAME and GIT_AUTHOR_EMAIL to override default user.

REPO_URL="${1:-}"
BASE_BRANCH="${2:-main}"
VERIFY="${VERIFY:-0}"

if [[ "${3:-}" == "--verify" ]]; then
  VERIFY=1
fi

if [[ -z "$REPO_URL" ]]; then
  echo "Usage: $0 <repo_url> [base_branch]" >&2
  exit 1
fi

WORKDIR="$(mktemp -d -t ploy-e2e-branches-XXXXXXXX)"
echo "Working directory: $WORKDIR"
trap 'rm -rf "$WORKDIR"' EXIT

git -c advice.detachedHead=false clone --depth 1 --branch "$BASE_BRANCH" "$REPO_URL" "$WORKDIR/repo"
cd "$WORKDIR/repo"

# Configure author if provided
if [[ -n "${GIT_AUTHOR_NAME:-}" ]]; then git config user.name "$GIT_AUTHOR_NAME"; fi
if [[ -n "${GIT_AUTHOR_EMAIL:-}" ]]; then git config user.email "$GIT_AUTHOR_EMAIL"; fi

# Helper to create/force-push a branch
create_branch() {
  local name="$1"; shift
  git checkout -B "$name"
  git commit --allow-empty -m "chore(e2e): initialize $name" >/dev/null 2>&1 || true
  git push -u origin "$name" -f
}

# 1) e2e/success: baseline branch (no intentional failure)
create_branch "e2e/success"

# Optional: verify success branch compiles under current JDK (expected: success)
if [[ "$VERIFY" == "1" ]]; then
  echo "[verify] Building e2e/success (expected: success)"
  set +e
  mvn -q -DskipTests -DskipITs -B compile
  RC=$?
  set -e
  if [[ $RC -ne 0 ]]; then
    echo "[verify] ERROR: e2e/success did not compile (rc=$RC)" >&2
    exit 2
  else
    echo "[verify] OK: e2e/success compiled"
  fi
fi

# 2) e2e/fail-missing-symbol: add a trivial compile error
git checkout -B "e2e/fail-missing-symbol"
mkdir -p src/main/java/e2e
cat > src/main/java/e2e/FailMissingSymbol.java <<'EOF'
package e2e;

public class FailMissingSymbol {
    public static void main(String[] args) {
        // Intentional reference to an unknown symbol (compile error)
        UnknownClass obj = new UnknownClass();
        System.out.println(obj);
    }
}
EOF
git add src/main/java/e2e/FailMissingSymbol.java
git commit -m "test(e2e): add missing symbol compile failure" || true
git push -u origin e2e/fail-missing-symbol -f

# Optional: verify fail branch does NOT compile (expected: failure)
if [[ "$VERIFY" == "1" ]]; then
  echo "[verify] Building e2e/fail-missing-symbol (expected: failure)"
  set +e
  mvn -q -DskipTests -DskipITs -B compile
  RC=$?
  set -e
  if [[ $RC -eq 0 ]]; then
    echo "[verify] WARNING: e2e/fail-missing-symbol compiled successfully; expected failure" >&2
  else
    echo "[verify] OK: e2e/fail-missing-symbol failed to compile as expected (rc=$RC)"
  fi
fi

# 3) e2e/fail-java17-specific: add Nashorn reference (removed from JDK 15+, fails on 17)
git checkout -B "e2e/fail-java17-specific"
mkdir -p src/main/java/e2e
cat > src/main/java/e2e/FailJava17Specific.java <<'EOF'
package e2e;

import jdk.nashorn.api.scripting.NashornScriptEngineFactory; // Removed in JDK 15+, compile fails on 17

import javax.script.ScriptEngine;

public class FailJava17Specific {
    public static void main(String[] args) {
        NashornScriptEngineFactory factory = new NashornScriptEngineFactory();
        ScriptEngine engine = factory.getScriptEngine();
        System.out.println(engine);
    }
}
EOF
git add src/main/java/e2e/FailJava17Specific.java
git commit -m "test(e2e): add Java 17 specific compile failure (Nashorn)" || true
git push -u origin e2e/fail-java17-specific -f

# Optional: verify java17-specific branch does NOT compile under JDK 17 (expected: failure)
if [[ "$VERIFY" == "1" ]]; then
  echo "[verify] Building e2e/fail-java17-specific (expected: failure on JDK 17)"
  set +e
  mvn -q -DskipTests -DskipITs -B compile
  RC=$?
  set -e
  if [[ $RC -eq 0 ]]; then
    echo "[verify] WARNING: e2e/fail-java17-specific compiled successfully; expected failure on JDK 17" >&2
  else
    echo "[verify] OK: e2e/fail-java17-specific failed to compile as expected (rc=$RC)"
  fi
fi

echo "Done. Created/updated branches:"
echo "  - e2e/success"
echo "  - e2e/fail-missing-symbol"
echo "  - e2e/fail-java17-specific"
echo "Repo: $REPO_URL"
echo "Base: $BASE_BRANCH"
