#!/usr/bin/env bash
set -euo pipefail

workspace="${PLOY_SBOM_WORKSPACE:-/workspace}"
sbom_output="/share/sbom.spdx.json"
classpath_output="/share/java.classpath"
init_script="${PLOY_SBOM_GRADLE_INIT_SCRIPT:-/usr/local/lib/ploy/sbom/gradle-write-java-classpath.init.gradle}"
gradle_cmd="${PLOY_SBOM_GRADLE_CMD:-}"

mkdir -p "$(dirname "$sbom_output")" "$(dirname "$classpath_output")"

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

deps_raw="$(mktemp)"
deps_pairs="$(mktemp)"
"$gradle_cmd" -q -p "$workspace" dependencies > "$deps_raw"
if ! "$gradle_cmd" -q -p "$workspace" buildEnvironment >> "$deps_raw" 2>/dev/null; then
  :
fi
if ! "$gradle_cmd" -q -p "$workspace" classes >/dev/null 2>&1; then
  :
fi

awk '
{
  line = $0
  while (match(line, /[A-Za-z0-9_.-]+:[A-Za-z0-9_.-]+:[A-Za-z0-9][A-Za-z0-9+_.-]*/)) {
    dep = substr(line, RSTART, RLENGTH)
    split(dep, parts, ":")
    name = parts[1] ":" parts[2]
    version = parts[3]
    override = line
    if (match(override, /->[[:space:]]*[A-Za-z0-9][A-Za-z0-9+_.-]*/)) {
      ov = substr(override, RSTART, RLENGTH)
      sub(/->[[:space:]]*/, "", ov)
      version = ov
    }
    print name "\t" version
    line = substr(line, RSTART + RLENGTH)
  }
}
' "$deps_raw" | sort -u > "$deps_pairs"

awk -F '\t' '
BEGIN {
  print "{"
  print "  \"spdxVersion\": \"SPDX-2.3\","
  print "  \"dataLicense\": \"CC0-1.0\","
  print "  \"SPDXID\": \"SPDXRef-DOCUMENT\","
  print "  \"name\": \"ploy-generated-sbom\","
  print "  \"documentNamespace\": \"https://ploy.dev/sbom/generated\","
  print "  \"creationInfo\": {\"created\":\"1970-01-01T00:00:00Z\",\"creators\":[\"Tool: ploy-nodeagent\"]},"
  print "  \"packages\": ["
  n = 0
}
NF == 2 {
  n++
  if (n > 1) {
    printf(",\n")
  }
  printf("    {\"SPDXID\":\"SPDXRef-Package-%06d\",\"name\":\"%s\",\"versionInfo\":\"%s\"}", n, $1, $2)
}
END {
  if (n > 0) {
    printf("\n")
  }
  print "  ]"
  print "}"
}
' "$deps_pairs" > "$sbom_output"

rm -f "$deps_raw" "$deps_pairs"

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
    echo "sbom classpath invariant violated: workspace outputs exist but are missing from java.classpath" >&2
    cat "$missing_workspace" >&2
    exit 1
  fi
fi

rm -f "$workspace_cp" "$missing_workspace"
