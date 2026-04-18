#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<USAGE >&2
Usage: $0 <compare-json> <output-json> [work-dir]

Reads compare JSON (from compare.sh), runs japicmp for every dependency in .changed,
and writes machine-readable reports:
- index JSON at <output-json>
- per-artifact detailed JSON files in <work-dir>/artifacts/

Environment variables:
  JAPICMP_VERSION      Japicmp version (default: 0.23.1)
  JAPICMP_LIMIT        Process only first N changed artifacts (default: all)
  JAPICMP_GA_REGEX     Only process changed entries whose ga matches regex
  MAVEN_REPO_URL       Optional Maven repo URL (passed via -DremoteRepositories)
USAGE
}

if [[ $# -lt 2 || $# -gt 3 ]]; then
  usage
  exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TOOL_POM="$SCRIPT_DIR/pom.xml"
JAVADOC_ENRICHER_MAIN="JapicmpRemovalJavadocEnricherCli"

COMPARE_JSON="$1"
OUTPUT_JSON="$2"
WORK_DIR="${3:-$(dirname "$OUTPUT_JSON")/$(basename "$OUTPUT_JSON" .json)-work}"

if [[ ! -f "$COMPARE_JSON" ]]; then
  echo "error: compare file not found: $COMPARE_JSON" >&2
  exit 1
fi

for bin in jq yq mvn java xmllint; do
  if ! command -v "$bin" >/dev/null 2>&1; then
    echo "error: required tool not found: $bin" >&2
    exit 1
  fi
done

if ! jq -e 'type == "object" and (.changed | type) == "array"' "$COMPARE_JSON" >/dev/null; then
  echo "error: unsupported compare file format (expected object with .changed array)" >&2
  exit 1
fi

JAPICMP_VERSION="${JAPICMP_VERSION:-0.23.1}"
JAPICMP_LIMIT="${JAPICMP_LIMIT:-0}"
JAPICMP_GA_REGEX="${JAPICMP_GA_REGEX:-}"
MAVEN_REPO_URL="${MAVEN_REPO_URL:-}"

MDEP_PLUGIN="org.apache.maven.plugins:maven-dependency-plugin:3.8.1"
JAPICMP_JAR="$HOME/.m2/repository/com/github/siom79/japicmp/japicmp/${JAPICMP_VERSION}/japicmp-${JAPICMP_VERSION}-jar-with-dependencies.jar"

mkdir -p "$WORK_DIR" "$WORK_DIR/tmp" "$WORK_DIR/artifacts"
RESULTS_NDJSON="$WORK_DIR/results.ndjson"
: > "$RESULTS_NDJSON"
TOOL_CLASSPATH_FILE="$WORK_DIR/tmp/tool.classpath"
TOOL_RUNTIME_CP=""

mvn_base=(mvn -q)
if [[ -n "$MAVEN_REPO_URL" ]]; then
  mvn_base+=("-DremoteRepositories=${MAVEN_REPO_URL}")
fi

run_mvn() {
  "${mvn_base[@]}" "$@"
}

ensure_tool_runtime_cp() {
  if [[ -n "$TOOL_RUNTIME_CP" ]]; then
    return 0
  fi
  local tool_cp_out="$TOOL_CLASSPATH_FILE"
  if [[ "$tool_cp_out" != /* ]]; then
    tool_cp_out="$(pwd)/$tool_cp_out"
  fi
  run_mvn -f "$TOOL_POM" -DskipTests compile >/dev/null
  run_mvn -f "$TOOL_POM" "$MDEP_PLUGIN:build-classpath" -DincludeScope=runtime "-Dmdep.outputFile=${tool_cp_out}" >/dev/null
  TOOL_RUNTIME_CP="$(cat "$TOOL_CLASSPATH_FILE"):$SCRIPT_DIR/target/classes"
}

artifact_jar_path() {
  local group_id="$1"
  local artifact_id="$2"
  local version="$3"
  local group_path="${group_id//.//}"
  printf '%s/.m2/repository/%s/%s/%s/%s-%s.jar' "$HOME" "$group_path" "$artifact_id" "$version" "$artifact_id" "$version"
}

artifact_pom_path() {
  local group_id="$1"
  local artifact_id="$2"
  local version="$3"
  local group_path="${group_id//.//}"
  printf '%s/.m2/repository/%s/%s/%s/%s-%s.pom' "$HOME" "$group_path" "$artifact_id" "$version" "$artifact_id" "$version"
}

resolve_jar() {
  local ga="$1"
  local version="$2"
  local group_id artifact_id jar_path pom_path packaging

  IFS=':' read -r group_id artifact_id <<<"$ga"

  if run_mvn "$MDEP_PLUGIN:get" "-Dartifact=${group_id}:${artifact_id}:${version}" -Dtransitive=false >/dev/null 2>&1; then
    jar_path="$(artifact_jar_path "$group_id" "$artifact_id" "$version")"
    if [[ -f "$jar_path" ]]; then
      printf '%s\n' "$jar_path"
      return 0
    fi
  fi

  if run_mvn "$MDEP_PLUGIN:get" "-Dartifact=${group_id}:${artifact_id}:${version}:pom" -Dtransitive=false >/dev/null 2>&1; then
    pom_path="$(artifact_pom_path "$group_id" "$artifact_id" "$version")"
    if [[ -f "$pom_path" ]]; then
      packaging="$(xmllint --xpath 'string(/*[local-name()="project"]/*[local-name()="packaging"])' "$pom_path" 2>/dev/null || true)"
      if [[ -z "$packaging" ]]; then
        packaging="jar"
      fi
      if [[ "$packaging" != "jar" ]]; then
        echo "non-jar:${packaging}" >&2
        return 10
      fi
    fi
  fi

  return 1
}

build_classpath() {
  local group_id="$1"
  local artifact_id="$2"
  local version="$3"
  local out_file="$4"
  local out_file_abs
  local pom_file="$WORK_DIR/tmp/cp-${group_id//./_}-${artifact_id}-${version}.pom.xml"

  if [[ "$out_file" = /* ]]; then
    out_file_abs="$out_file"
  else
    out_file_abs="$(pwd)/$out_file"
  fi

  cat > "$pom_file" <<POM
<project xmlns="http://maven.apache.org/POM/4.0.0"
         xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
         xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 http://maven.apache.org/xsd/maven-4.0.0.xsd">
  <modelVersion>4.0.0</modelVersion>
  <groupId>tool.tmp</groupId>
  <artifactId>cp-${artifact_id}</artifactId>
  <version>1.0.0</version>
  <dependencies>
    <dependency>
      <groupId>${group_id}</groupId>
      <artifactId>${artifact_id}</artifactId>
      <version>${version}</version>
    </dependency>
  </dependencies>
</project>
POM

  run_mvn -f "$pom_file" "$MDEP_PLUGIN:build-classpath" -Dmdep.includeScope=runtime "-Dmdep.outputFile=${out_file_abs}" >/dev/null
}

ensure_japicmp() {
  if [[ -f "$JAPICMP_JAR" ]]; then
    return 0
  fi
  run_mvn "$MDEP_PLUGIN:get" "-Dartifact=com.github.siom79.japicmp:japicmp:${JAPICMP_VERSION}:jar:jar-with-dependencies" -Dtransitive=false >/dev/null
}

extract_changes() {
  local json_file="$1"
  jq '
    def arr:
      if . == null then []
      elif type == "array" then .
      else [.] end;

    def member_sig($m):
      ($m."+@name" // "unknown") + "(" + (($m.parameters.parameter | arr | map(."+@type" // "n.a.")) | join(",")) + ")";

    [
      (.japicmp.classes.class | arr[]) as $c
      | ($c."+@fullyQualifiedName" // "n.a.") as $class_name
      | (
          ($c.compatibilityChanges.compatibilityChange | arr[]
            | {
                kind: "class",
                class: $class_name,
                member: null,
                type: ."+@type",
                binary_compatible: (."+@binaryCompatible" == "true"),
                source_compatible: (."+@sourceCompatible" == "true")
              }
          ),
          (($c.fields.field | arr[]) as $f
            | ($f.compatibilityChanges.compatibilityChange | arr[]
              | {
                  kind: "field",
                  class: $class_name,
                  member: ($f."+@name" // "unknown"),
                  type: ."+@type",
                  binary_compatible: (."+@binaryCompatible" == "true"),
                  source_compatible: (."+@sourceCompatible" == "true")
                }
            )
          ),
          (($c.constructors.constructor | arr[]) as $ctor
            | ($ctor.compatibilityChanges.compatibilityChange | arr[]
              | {
                  kind: "constructor",
                  class: $class_name,
                  member: member_sig($ctor),
                  type: ."+@type",
                  binary_compatible: (."+@binaryCompatible" == "true"),
                  source_compatible: (."+@sourceCompatible" == "true")
                }
            )
          ),
          (($c.methods.method | arr[]) as $m
            | ($m.compatibilityChanges.compatibilityChange | arr[]
              | {
                  kind: "method",
                  class: $class_name,
                  member: member_sig($m),
                  type: ."+@type",
                  binary_compatible: (."+@binaryCompatible" == "true"),
                  source_compatible: (."+@sourceCompatible" == "true")
                }
            )
          ),
          (($c.interfaces.interface | arr[]) as $i
            | ($i.compatibilityChanges.compatibilityChange | arr[]
              | {
                  kind: "interface",
                  class: $class_name,
                  member: ($i."+@fullyQualifiedName" // "unknown"),
                  type: ."+@type",
                  binary_compatible: (."+@binaryCompatible" == "true"),
                  source_compatible: (."+@sourceCompatible" == "true")
                }
            )
          ),
          ($c.superclass.compatibilityChanges.compatibilityChange | arr[] as $sc
            | {
                kind: "superclass",
                class: $class_name,
                member: ($c.superclass."+@superclassOld" // $c.superclass."+@superclassNew" // "unknown"),
                type: $sc."+@type",
                binary_compatible: ($sc."+@binaryCompatible" == "true"),
                source_compatible: ($sc."+@sourceCompatible" == "true")
              }
          )
        )
    ]
  ' "$json_file"
}

ensure_japicmp
ensure_tool_runtime_cp

total_changed="$(jq '.changed | length' "$COMPARE_JSON")"
echo "processing changed dependencies: ${total_changed}" >&2

changed_stream_cmd=(jq -c '.changed[]' "$COMPARE_JSON")
if [[ -n "$JAPICMP_GA_REGEX" ]]; then
  changed_stream_cmd=(jq -c --arg re "$JAPICMP_GA_REGEX" '.changed[] | select(.ga | test($re))' "$COMPARE_JSON")
fi

processed=0
while IFS= read -r dep; do
  if [[ "$JAPICMP_LIMIT" =~ ^[0-9]+$ ]] && [[ "$JAPICMP_LIMIT" -gt 0 ]] && [[ "$processed" -ge "$JAPICMP_LIMIT" ]]; then
    break
  fi

  processed=$((processed + 1))

  ga="$(jq -r '.ga' <<<"$dep")"
  from_ver="$(jq -r '.from' <<<"$dep")"
  to_ver="$(jq -r '.to' <<<"$dep")"

  if [[ -z "$ga" || -z "$from_ver" || -z "$to_ver" || "$ga" == "null" || "$from_ver" == "null" || "$to_ver" == "null" ]]; then
    jq -nc --arg status "error" --arg error "invalid changed entry" --arg raw "$dep" '{status:$status,error:$error,raw:$raw}' >> "$RESULTS_NDJSON"
    continue
  fi

  echo "[$processed] $ga $from_ver -> $to_ver" >&2

  IFS=':' read -r group_id artifact_id <<<"$ga"
  slug="$(printf '%s__%s__%s__%s' "$group_id" "$artifact_id" "$from_ver" "$to_ver" | tr '/:' '_' | tr -c 'A-Za-z0-9._-' '_')"

  artifact_dir="$WORK_DIR/artifacts/$slug"
  mkdir -p "$artifact_dir"

  old_cp_file="$artifact_dir/old.classpath"
  new_cp_file="$artifact_dir/new.classpath"
  xml_file="$artifact_dir/japicmp.xml"
  xml_json_file="$artifact_dir/japicmp.xml.json"
  changes_file="$artifact_dir/changes.json"
  report_file="$artifact_dir/report.json"
  log_file="$artifact_dir/japicmp.log"

  old_jar=""
  new_jar=""

  old_jar="$(resolve_jar "$ga" "$from_ver" 2>"$artifact_dir/old.resolve.err")" || {
    rc=$?
    reason="$(tr -d '\n' < "$artifact_dir/old.resolve.err" 2>/dev/null || true)"
    if [[ "$rc" -eq 10 ]]; then
      jq -nc \
        --arg ga "$ga" \
        --arg from "$from_ver" \
        --arg to "$to_ver" \
        --arg status "skipped_non_jar" \
        --arg reason "old-version packaging is not jar (${reason#non-jar:})" \
        '{ga:$ga,from:$from,to:$to,status:$status,reason:$reason}' >> "$RESULTS_NDJSON"
    else
      jq -nc \
        --arg ga "$ga" \
        --arg from "$from_ver" \
        --arg to "$to_ver" \
        --arg status "error" \
        --arg error "failed to resolve old jar" \
        --arg detail "$reason" \
        '{ga:$ga,from:$from,to:$to,status:$status,error:$error,detail:$detail}' >> "$RESULTS_NDJSON"
    fi
    continue
  }

  new_jar="$(resolve_jar "$ga" "$to_ver" 2>"$artifact_dir/new.resolve.err")" || {
    rc=$?
    reason="$(tr -d '\n' < "$artifact_dir/new.resolve.err" 2>/dev/null || true)"
    if [[ "$rc" -eq 10 ]]; then
      jq -nc \
        --arg ga "$ga" \
        --arg from "$from_ver" \
        --arg to "$to_ver" \
        --arg status "skipped_non_jar" \
        --arg reason "new-version packaging is not jar (${reason#non-jar:})" \
        '{ga:$ga,from:$from,to:$to,status:$status,reason:$reason}' >> "$RESULTS_NDJSON"
    else
      jq -nc \
        --arg ga "$ga" \
        --arg from "$from_ver" \
        --arg to "$to_ver" \
        --arg status "error" \
        --arg error "failed to resolve new jar" \
        --arg detail "$reason" \
        '{ga:$ga,from:$from,to:$to,status:$status,error:$error,detail:$detail}' >> "$RESULTS_NDJSON"
    fi
    continue
  }

  if ! build_classpath "$group_id" "$artifact_id" "$from_ver" "$old_cp_file"; then
    jq -nc \
      --arg ga "$ga" \
      --arg from "$from_ver" \
      --arg to "$to_ver" \
      --arg status "error" \
      --arg error "failed to build old classpath" \
      '{ga:$ga,from:$from,to:$to,status:$status,error:$error}' >> "$RESULTS_NDJSON"
    continue
  fi

  if ! build_classpath "$group_id" "$artifact_id" "$to_ver" "$new_cp_file"; then
    jq -nc \
      --arg ga "$ga" \
      --arg from "$from_ver" \
      --arg to "$to_ver" \
      --arg status "error" \
      --arg error "failed to build new classpath" \
      '{ga:$ga,from:$from,to:$to,status:$status,error:$error}' >> "$RESULTS_NDJSON"
    continue
  fi

  old_cp="$(cat "$old_cp_file")"
  new_cp="$(cat "$new_cp_file")"

  if ! java -jar "$JAPICMP_JAR" \
      --old "$old_jar" \
      --new "$new_jar" \
      --old-classpath "$old_cp" \
      --new-classpath "$new_cp" \
      --only-modified \
      --only-incompatible \
      --ignore-missing-classes \
      --xml-file "$xml_file" >"$log_file" 2>&1; then
    jq -nc \
      --arg ga "$ga" \
      --arg from "$from_ver" \
      --arg to "$to_ver" \
      --arg status "error" \
      --arg error "japicmp failed" \
      --arg log_file "$(realpath "$log_file")" \
      '{ga:$ga,from:$from,to:$to,status:$status,error:$error,log_file:$log_file}' >> "$RESULTS_NDJSON"
    continue
  fi

  yq -p=xml -o=json '.' "$xml_file" > "$xml_json_file"
  extract_changes "$xml_json_file" > "$changes_file"
  removals_file="$artifact_dir/removals.json"
  enriched_removals_file="$artifact_dir/removals.enriched.json"
  javadoc_enrich_log="$artifact_dir/javadoc-enricher.log"
  jq '[.[] | select(.type | contains("_REMOVED"))]' "$changes_file" > "$removals_file"

  javadoc_enricher_cmd=(
    java -cp "$TOOL_RUNTIME_CP" "$JAVADOC_ENRICHER_MAIN"
    --ga "$ga"
    --from "$from_ver"
    --to "$to_ver"
    --removals-file "$removals_file"
    --output "$enriched_removals_file"
  )
  if [[ -n "$MAVEN_REPO_URL" ]]; then
    javadoc_enricher_cmd+=(--repo-url "$MAVEN_REPO_URL")
  fi

  if ! "${javadoc_enricher_cmd[@]}" >"$javadoc_enrich_log" 2>&1; then
    jq 'map(. + {javadoc_last_ver: null, javadoc_last_note: null})' "$removals_file" > "$enriched_removals_file"
  fi

  incompatible_count="$(jq 'length' "$changes_file")"
  removal_count="$(jq '[.[] | select(.type | contains("_REMOVED"))] | length' "$changes_file")"
  type_counts="$(jq 'map(.type) | group_by(.) | map({key:.[0], value:length}) | from_entries' "$changes_file")"
  removal_type_counts="$(jq '[.[] | select(.type | contains("_REMOVED")) | .type] | group_by(.) | map({key:.[0], value:length}) | from_entries' "$changes_file")"

  jq -n \
    --arg ga "$ga" \
    --arg from "$from_ver" \
    --arg to "$to_ver" \
    --arg old_jar "$(realpath "$old_jar")" \
    --arg new_jar "$(realpath "$new_jar")" \
    --arg xml_file "$(realpath "$xml_file")" \
    --arg xml_json_file "$(realpath "$xml_json_file")" \
    --arg log_file "$(realpath "$log_file")" \
    --arg javadoc_enrich_log "$(realpath "$javadoc_enrich_log")" \
    --argjson changes "$(cat "$changes_file")" \
    --argjson removals "$(cat "$enriched_removals_file")" \
    --argjson incompatible_count "$incompatible_count" \
    --argjson removal_count "$removal_count" \
    '{
      ga: $ga,
      from: $from,
      to: $to,
      status: "ok",
      old_jar: $old_jar,
      new_jar: $new_jar,
      japicmp_xml: $xml_file,
      japicmp_xml_json: $xml_json_file,
      japicmp_log: $log_file,
      javadoc_enricher_log: $javadoc_enrich_log,
      incompatible_change_count: $incompatible_count,
      removal_change_count: $removal_count,
      incompatible_changes: $changes,
      removals: $removals
    }' > "$report_file"

  jq -nc \
    --arg ga "$ga" \
    --arg from "$from_ver" \
    --arg to "$to_ver" \
    --arg status "ok" \
    --arg report_file "$(realpath "$report_file")" \
    --argjson incompatible_change_count "$incompatible_count" \
    --argjson removal_change_count "$removal_count" \
    --argjson change_type_counts "$type_counts" \
    --argjson removal_type_counts "$removal_type_counts" \
    '{
      ga:$ga,
      from:$from,
      to:$to,
      status:$status,
      report_file:$report_file,
      incompatible_change_count:$incompatible_change_count,
      removal_change_count:$removal_change_count,
      change_type_counts:$change_type_counts,
      removal_type_counts:$removal_type_counts
    }' >> "$RESULTS_NDJSON"
done < <("${changed_stream_cmd[@]}")

RESULTS_JSON="$WORK_DIR/results.json"
jq -s 'sort_by(.ga // "")' "$RESULTS_NDJSON" > "$RESULTS_JSON"

jq -n \
  --arg compare_file "$(realpath "$COMPARE_JSON")" \
  --arg generated_at "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  --arg work_dir "$(realpath "$WORK_DIR")" \
  --arg japicmp_version "$JAPICMP_VERSION" \
  --slurpfile compare "$COMPARE_JSON" \
  --slurpfile results "$RESULTS_JSON" '
    ($compare[0]) as $c
    | ($results[0]) as $r
    | {
        compare_file: $compare_file,
        generated_at: $generated_at,
        work_dir: $work_dir,
        japicmp_version: $japicmp_version,
        filters: {
          ga_regex: (env.JAPICMP_GA_REGEX // ""),
          limit: (env.JAPICMP_LIMIT // "0")
        },
        summary: {
          changed_total: (($c.changed // []) | length),
          processed_entries: ($r | length),
          ok: ($r | map(select(.status == "ok")) | length),
          skipped_non_jar: ($r | map(select(.status == "skipped_non_jar")) | length),
          errors: ($r | map(select(.status == "error")) | length),
          with_incompatible_changes: ($r | map(select(.status == "ok" and .incompatible_change_count > 0)) | length),
          with_removals: ($r | map(select(.status == "ok" and .removal_change_count > 0)) | length)
        },
        added_artifacts: ($c.added // []),
        removed_artifacts: ($c.removed // []),
        changed_artifacts: $r
      }
  ' > "$OUTPUT_JSON"

echo "done: $(realpath "$OUTPUT_JSON")" >&2
echo "artifact reports: $(realpath "$WORK_DIR/artifacts")" >&2
