#!/usr/bin/env bash
set -euo pipefail

REPO_URL="${1:-}"
BASE_BRANCH="${2:-main}"
VERIFY="${VERIFY:-0}"

if [[ "${3:-}" == "--verify" ]]; then
  VERIFY=1
fi

if [[ -z "$REPO_URL" ]]; then
  echo "Usage: $0 <repo_url> [base_branch] [--verify]" >&2
  exit 1
fi

WORKDIR="$(mktemp -d -t ploy-e2e-parallel-XXXXXXXX)"
echo "Working directory: $WORKDIR"
trap 'rm -rf "$WORKDIR"' EXIT

GIT_CRED_CONFIG=()
if [[ -n "${PLOY_GITLAB_PAT:-}" ]]; then
  export GIT_TERMINAL_PROMPT=0
  export GIT_HTTP_USERNAME="oauth2"
  export GIT_HTTP_PASSWORD="$PLOY_GITLAB_PAT"
  export GIT_ASKPASS="$WORKDIR/git-askpass.sh"
  GIT_CRED_CONFIG=(-c credential.helper=)
  cat >"$GIT_ASKPASS" <<'EOF'
#!/usr/bin/env bash
case "$1" in
  *Username*)
    echo "oauth2"
    ;;
  *Password*)
    if [[ -z "${PLOY_GITLAB_PAT:-}" ]]; then
      exit 1
    fi
    echo "${PLOY_GITLAB_PAT}"
    ;;
  *)
    exit 1
    ;;
esac
EOF
  chmod +x "$GIT_ASKPASS"
fi

git "${GIT_CRED_CONFIG[@]}" -c advice.detachedHead=false clone --depth 1 --branch "$BASE_BRANCH" "$REPO_URL" "$WORKDIR/repo"
cd "$WORKDIR/repo"

if [[ -n "${GIT_AUTHOR_NAME:-}" ]]; then git config user.name "$GIT_AUTHOR_NAME"; fi
if [[ -n "${GIT_AUTHOR_EMAIL:-}" ]]; then git config user.email "$GIT_AUTHOR_EMAIL"; fi

BRANCH="e2e/fail-parallel"

git checkout -B "$BRANCH"
mkdir -p src/main/java/e2e

cat <<'JAVA' > src/main/java/e2e/FailMissingSymbol.java
package e2e;

public class FailMissingSymbol {
    public static void main(String[] args) {
        // Intentional reference to an unknown symbol (compile error)
        UnknownClass obj = new UnknownClass();
        System.out.println(obj);
    }
}
JAVA

cat <<'JAVA' > src/main/java/e2e/FailJava17Specific.java
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
JAVA

cat <<'JAVA' > src/main/java/e2e/ParallelReadme.java
package e2e;

public final class ParallelReadme {
    private ParallelReadme() {}

    public static final String CONTEXT = "Scenario: parallel healing - missing symbol + Nashorn";
}
JAVA

python3 - <<'PY'
from pathlib import Path
pom = Path("pom.xml")
if pom.exists():
    text = pom.read_text()
    replacements = {
        '<maven.compiler.source>17</maven.compiler.source>': '<maven.compiler.source>11</maven.compiler.source>',
        '<maven.compiler.target>17</maven.compiler.target>': '<maven.compiler.target>11</maven.compiler.target>',
        '<maven.compiler.release>17</maven.compiler.release>': '<maven.compiler.release>11</maven.compiler.release>'
    }
    updated = text
    for old, new in replacements.items():
        updated = updated.replace(old, new)
    if updated != text:
        pom.write_text(updated)
PY

git add pom.xml src/main/java/e2e/FailMissingSymbol.java src/main/java/e2e/FailJava17Specific.java src/main/java/e2e/ParallelReadme.java

if ! git diff --cached --quiet; then
  git commit -m "test(e2e): create parallel failure branch" >/dev/null
else
  echo "No changes detected; branch already prepared" >&2
fi

git "${GIT_CRED_CONFIG[@]}" push -u origin "$BRANCH" -f

if [[ "$VERIFY" == "1" ]]; then
  echo "[verify] Building $BRANCH (expected: failure)"
  set +e
  mvn -q -DskipTests -DskipITs -B compile
  RC=$?
  set -e
  if [[ $RC -eq 0 ]]; then
    echo "[verify] WARNING: $BRANCH compiled successfully; expected failure" >&2
  else
    echo "[verify] OK: $BRANCH failed to compile (rc=$RC)"
  fi
fi

echo "Pushed $BRANCH to $REPO_URL (base $BASE_BRANCH)"
