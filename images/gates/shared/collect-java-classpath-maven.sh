#!/usr/bin/env bash
set -euo pipefail

workspace="${PLOY_SBOM_WORKSPACE:-/workspace}"
sbom_output="/share/sbom.spdx.json"
classpath_output="/share/java.classpath"
pom_path="${PLOY_SBOM_MAVEN_POM_PATH:-$workspace/pom.xml}"
workspace_prefix="${workspace%/}/"

mkdir -p "$(dirname "$sbom_output")" "$(dirname "$classpath_output")"

if [[ ! -f "$pom_path" ]]; then
  echo "missing $pom_path" >&2
  exit 1
fi

deps_raw="$(mktemp)"
deps_pairs="$(mktemp)"
mvn -B -q -f "$pom_path" -DoutputFile="$deps_raw" dependency:list
mvn -B -q -f "$pom_path" -DskipTests compile >/dev/null 2>&1 || :
mvn -B -q -f "$pom_path" -DskipTests test-compile >/dev/null 2>&1 || :

awk '
{
  for (i = 1; i <= NF; i++) {
    token = $i
    gsub(/^[[:space:]]+/, "", token)
    gsub(/[[:space:]]+$/, "", token)
    gsub(/[,;]$/, "", token)
    n = split(token, parts, ":")
    if (n >= 5) {
      group = parts[1]
      artifact = parts[2]
      version = parts[n-1]
      scope = parts[n]
      if (group ~ /^[A-Za-z0-9_.-]+$/ && artifact ~ /^[A-Za-z0-9_.-]+$/ && version ~ /^[A-Za-z0-9][A-Za-z0-9+_.-]*$/ && scope != "") {
        print group ":" artifact "\t" version
      }
    }
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
{
  awk -F ':' '
    {
      for (i = 1; i <= NF; i++) {
        entry = $i
        sub(/^[[:space:]]+/, "", entry)
        sub(/[[:space:]]+$/, "", entry)
        if (entry != "") {
          print entry
        }
      }
    }
  ' "$cp_compile" "$cp_runtime" "$cp_test"
  awk 'NF > 0 { print }' "$workspace_cp"
} | awk 'NF > 0 && !seen[$0]++ { print $0 }' > "$classpath_output"

if ! awk -v prefix="$workspace_prefix" 'NF > 0 && index($0, prefix) == 1 { found = 1; exit } END { exit(found ? 0 : 1) }' "$classpath_output"; then
  if [[ -s "$workspace_cp" ]]; then
    echo "sbom classpath invariant violated: workspace outputs exist but are missing from java.classpath" >&2
    exit 1
  fi
fi

rm -f "$cp_compile" "$cp_runtime" "$cp_test" "$workspace_cp"
