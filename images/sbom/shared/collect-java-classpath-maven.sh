#!/usr/bin/env bash
set -euo pipefail

workspace="${PLOY_SBOM_WORKSPACE:-/workspace}"
raw_output="${PLOY_SBOM_DEPENDENCY_OUTPUT:-/out/sbom.dependencies.txt}"
classpath_output="${PLOY_SBOM_JAVA_CLASSPATH_OUTPUT:-/out/java.classpath}"
pom_path="${PLOY_SBOM_MAVEN_POM_PATH:-$workspace/pom.xml}"
workspace_prefix="${workspace%/}/"

mkdir -p "$(dirname "$raw_output")" "$(dirname "$classpath_output")"

if [[ ! -f "$pom_path" ]]; then
  echo "missing $pom_path" >&2
  exit 1
fi

mvn -B -q -f "$pom_path" -DoutputFile="$raw_output" dependency:list
if ! mvn -B -q -f "$pom_path" -DskipTests compile >/dev/null 2>&1; then
  printf "\n# ploy: compile preparation unavailable\n" >> "$raw_output"
fi
if ! mvn -B -q -f "$pom_path" -DskipTests test-compile >/dev/null 2>&1; then
  printf "\n# ploy: test-compile preparation unavailable\n" >> "$raw_output"
fi

cp_compile="$(mktemp)"
cp_runtime="$(mktemp)"
cp_test="$(mktemp)"
workspace_cp="$(mktemp)"

mvn -B -q -f "$pom_path" -Dmdep.outputFile="$cp_compile" -DincludeScope=compile dependency:build-classpath
mvn -B -q -f "$pom_path" -Dmdep.outputFile="$cp_runtime" -DincludeScope=runtime dependency:build-classpath
mvn -B -q -f "$pom_path" -Dmdep.outputFile="$cp_test" -DincludeScope=test dependency:build-classpath
find "$workspace" -type d \( -path '*/target/classes' -o -path '*/target/resources' -o -path '*/target/test-classes' -o -path '*/target/test-resources' \) \
  | awk 'NF > 0' \
  | sort -u > "$workspace_cp"
cat "$cp_compile" "$cp_runtime" "$cp_test" "$workspace_cp" | tr ':' '\n' | awk 'NF > 0 && !seen[$0]++ { print $0 }' > "$classpath_output"

if ! awk -v prefix="$workspace_prefix" 'NF > 0 && index($0, prefix) == 1 { found = 1; exit } END { exit(found ? 0 : 1) }' "$classpath_output"; then
  printf "\n# ploy: workspace classpath entries unavailable\n" >> "$raw_output"
  if [[ -s "$workspace_cp" ]]; then
    echo "sbom classpath invariant violated: workspace outputs exist but are missing from java.classpath" >&2
    exit 1
  fi
fi

rm -f "$cp_compile" "$cp_runtime" "$cp_test" "$workspace_cp"
