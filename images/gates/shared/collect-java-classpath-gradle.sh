#!/usr/bin/env bash
set -euo pipefail

workspace="${PLOY_SBOM_WORKSPACE:-/workspace}"
raw_output="${PLOY_SBOM_DEPENDENCY_OUTPUT:-/share/sbom.dependencies.txt}"
classpath_output="${PLOY_SBOM_JAVA_CLASSPATH_OUTPUT:-/share/java.classpath}"
init_script="${PLOY_SBOM_GRADLE_INIT_SCRIPT:-/usr/local/lib/ploy/sbom/gradle-write-java-classpath.init.gradle}"
gradle_cmd="${PLOY_SBOM_GRADLE_CMD:-}"

mkdir -p "$(dirname "$raw_output")" "$(dirname "$classpath_output")"

if [[ -z "$gradle_cmd" ]]; then
  if [[ -x "$workspace/gradlew" ]]; then
    gradle_cmd="$workspace/gradlew"
  elif command -v gradle >/dev/null 2>&1; then
    gradle_cmd="gradle"
  else
    echo "gradle build detected but no gradle wrapper and no gradle binary available" >&2
    exit 1
  fi
fi

if [[ ! -f "$init_script" ]]; then
  echo "missing Gradle SBOM init script: $init_script" >&2
  exit 1
fi

"$gradle_cmd" -q -p "$workspace" dependencies > "$raw_output"
if ! "$gradle_cmd" -q -p "$workspace" buildEnvironment >> "$raw_output" 2>/dev/null; then
  printf "\n# ploy: buildEnvironment unavailable\n" >> "$raw_output"
fi
if ! "$gradle_cmd" -q -p "$workspace" classes >/dev/null 2>&1; then
  printf "\n# ploy: classes preparation unavailable\n" >> "$raw_output"
fi

PLOY_SBOM_JAVA_CLASSPATH_OUTPUT="$classpath_output" \
  "$gradle_cmd" -q -p "$workspace" -I "$init_script" ployWriteJavaClasspath

if [[ ! -s "$classpath_output" ]]; then
  echo "sbom classpath invariant violated: java.classpath is empty or missing" >&2
  exit 1
fi

workspace_cp="$(mktemp)"
missing_workspace="$(mktemp)"
find "$workspace" -type d \
  \( -path '*/build/classes/java/main' \
  -o -path '*/build/classes/kotlin/main' \
  -o -path '*/build/classes/groovy/main' \
  -o -path '*/build/resources/main' \
  -o -path '*/build/classes/java/test' \
  -o -path '*/build/classes/kotlin/test' \
  -o -path '*/build/classes/groovy/test' \
  -o -path '*/build/resources/test' \) \
  | awk 'NF > 0' | sort -u > "$workspace_cp"

if [[ -s "$workspace_cp" ]]; then
  awk 'NR == FNR { seen[$0] = 1; next } NF > 0 && !seen[$0] { print $0 }' \
    "$classpath_output" "$workspace_cp" > "$missing_workspace"
  if [[ -s "$missing_workspace" ]]; then
    printf "\n# ploy: workspace classpath entries unavailable\n" >> "$raw_output"
    echo "sbom classpath invariant violated: workspace outputs exist but are missing from java.classpath" >&2
    cat "$missing_workspace" >&2
    exit 1
  fi
fi

rm -f "$workspace_cp" "$missing_workspace"
