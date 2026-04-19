#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE' >&2
Usage:
  ./openrewrite-plan.sh \
    --report <path/to/report.json> \
    --out-dir <path> \
    [--catalog-dir <path>] \
    [--refresh-catalog] \
    [--catalog-scope official-oss] \
    [--ranker deterministic|hybrid]

Description:
  Builds an OpenRewrite recipe plan from java/report.json.

Outputs in --out-dir:
  - plan.json
  - coverage.json
  - rewrite.yml
  - manual-gaps.md

Environment:
  ORW_HYBRID_RERANKER_CMD   Optional command used when --ranker hybrid.
                            Command reads plan JSON from stdin and must emit
                            a valid plan JSON object to stdout.
USAGE
}

if [[ $# -eq 0 ]]; then
  usage
  exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MDEP_PLUGIN="org.apache.maven.plugins:maven-dependency-plugin:3.8.1"

REPORT_FILE=""
OUT_DIR=""
CATALOG_DIR="${HOME}/.cache/ploy/openrewrite"
REFRESH_CATALOG="false"
CATALOG_SCOPE="official-oss"
RANKER_MODE="hybrid"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --report)
      REPORT_FILE="${2:-}"
      shift 2
      ;;
    --out-dir)
      OUT_DIR="${2:-}"
      shift 2
      ;;
    --catalog-dir)
      CATALOG_DIR="${2:-}"
      shift 2
      ;;
    --refresh-catalog)
      REFRESH_CATALOG="true"
      shift 1
      ;;
    --catalog-scope)
      CATALOG_SCOPE="${2:-}"
      shift 2
      ;;
    --ranker)
      RANKER_MODE="${2:-}"
      shift 2
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      echo "error: unknown argument: $1" >&2
      usage
      exit 1
      ;;
  esac
done

if [[ -z "$REPORT_FILE" ]]; then
  echo "error: --report is required" >&2
  exit 1
fi
if [[ -z "$OUT_DIR" ]]; then
  echo "error: --out-dir is required" >&2
  exit 1
fi
if [[ "$CATALOG_SCOPE" != "official-oss" ]]; then
  echo "error: unsupported --catalog-scope: $CATALOG_SCOPE (expected: official-oss)" >&2
  exit 1
fi
if [[ "$RANKER_MODE" != "deterministic" && "$RANKER_MODE" != "hybrid" ]]; then
  echo "error: unsupported --ranker: $RANKER_MODE (expected: deterministic|hybrid)" >&2
  exit 1
fi

for bin in jq yq mvn unzip find sort awk sed mktemp date python3; do
  if ! command -v "$bin" >/dev/null 2>&1; then
    echo "error: required tool not found: $bin" >&2
    exit 1
  fi
done

if [[ ! -f "$REPORT_FILE" ]]; then
  echo "error: report file not found: $REPORT_FILE" >&2
  exit 1
fi

if ! jq -e 'type == "object" and (.target|type)=="object" and (.updates|type)=="array"' "$REPORT_FILE" >/dev/null; then
  echo "error: report does not match expected shape (missing target/updates)" >&2
  exit 1
fi

REPORT_FILE="$(cd "$(dirname "$REPORT_FILE")" && pwd)/$(basename "$REPORT_FILE")"
mkdir -p "$OUT_DIR"
OUT_DIR="$(cd "$OUT_DIR" && pwd)"
mkdir -p "$CATALOG_DIR/index"
CATALOG_DIR="$(cd "$CATALOG_DIR" && pwd)"

CATALOG_INDEX_JSON="$CATALOG_DIR/index/recipes.json"
CATALOG_MANIFEST_JSON="$CATALOG_DIR/index/manifest.json"
CATALOG_WARNINGS_JSON="$CATALOG_DIR/index/warnings.json"

official_catalog_artifacts() {
  cat <<'ARTIFACTS'
org.openrewrite:rewrite-core
org.openrewrite:rewrite-java
org.openrewrite:rewrite-javascript
org.openrewrite:rewrite-json
org.openrewrite:rewrite-kotlin
org.openrewrite:rewrite-gradle
org.openrewrite:rewrite-maven
org.openrewrite:rewrite-properties
org.openrewrite:rewrite-toml
org.openrewrite:rewrite-xml
org.openrewrite:rewrite-yaml
org.openrewrite.meta:rewrite-analysis
org.openrewrite.recipe:rewrite-apache
org.openrewrite.recipe:rewrite-github-actions
org.openrewrite.recipe:rewrite-hibernate
org.openrewrite.recipe:rewrite-jackson
org.openrewrite.recipe:rewrite-java-dependencies
org.openrewrite.recipe:rewrite-logging-frameworks
org.openrewrite.recipe:rewrite-micrometer
org.openrewrite.recipe:rewrite-micronaut
org.openrewrite.recipe:rewrite-migrate-java
org.openrewrite.recipe:rewrite-netty
org.openrewrite.recipe:rewrite-okhttp
org.openrewrite.recipe:rewrite-openapi
org.openrewrite.recipe:rewrite-quarkus
org.openrewrite.recipe:rewrite-rewrite
org.openrewrite.recipe:rewrite-spring
org.openrewrite.recipe:rewrite-spring-to-quarkus
org.openrewrite.recipe:rewrite-static-analysis
org.openrewrite.recipe:rewrite-testing-frameworks
org.openrewrite.recipe:rewrite-third-party
ARTIFACTS
}

warn_json_line() {
  local message="$1"
  jq -nc --arg message "$message" '$message'
}

latest_jar_from_local_repo() {
  local group="$1"
  local artifact="$2"
  local group_path
  group_path="$(printf '%s' "$group" | tr '.' '/')"
  local repo_dir="${HOME}/.m2/repository/${group_path}/$artifact"

  if [[ ! -d "$repo_dir" ]]; then
    return 1
  fi

  find "$repo_dir" -mindepth 2 -maxdepth 2 -type f -name "${artifact}-*.jar" \
    ! -name "*-sources.jar" ! -name "*-javadoc.jar" \
    | awk -F'/' '{print $(NF-1) "\t" $0}' \
    | sort -V -k1,1 \
    | tail -n1
}

resolve_recipe_jar_release() {
  local ga="$1"
  local group artifact

  IFS=':' read -r group artifact <<<"$ga"
  if [[ -z "$group" || -z "$artifact" ]]; then
    return 1
  fi

  if ! mvn -q "${MDEP_PLUGIN}:get" "-Dartifact=${group}:${artifact}:RELEASE" -Dtransitive=false >/dev/null 2>&1; then
    return 1
  fi

  local latest_line
  latest_line="$(latest_jar_from_local_repo "$group" "$artifact" || true)"
  if [[ -z "$latest_line" ]]; then
    return 1
  fi

  local version jar_path
  version="${latest_line%%$'\t'*}"
  jar_path="${latest_line#*$'\t'}"

  if [[ ! -f "$jar_path" ]]; then
    return 1
  fi

  printf '%s\t%s\t%s\t%s\n' "$group" "$artifact" "$version" "$jar_path"
}

resolve_recipe_sources_jar() {
  local group="$1"
  local artifact="$2"
  local version="$3"
  local group_path
  group_path="$(printf '%s' "$group" | tr '.' '/')"
  local jar_path="${HOME}/.m2/repository/${group_path}/${artifact}/${version}/${artifact}-${version}-sources.jar"

  if ! mvn -q "${MDEP_PLUGIN}:get" "-Dartifact=${group}:${artifact}:${version}:jar:sources" -Dtransitive=false >/dev/null 2>&1; then
    return 1
  fi

  if [[ -f "$jar_path" ]]; then
    printf '%s\n' "$jar_path"
    return 0
  fi

  return 1
}

extract_recipes_from_jar() {
  local jar_path="$1"
  local group="$2"
  local artifact="$3"
  local version="$4"
  local out_ndjson="$5"
  local extract_root="$6"

  local extract_dir="${extract_root}/${group//./_}-${artifact}-${version}"
  rm -rf "$extract_dir"
  mkdir -p "$extract_dir"

  if ! unzip -qq "$jar_path" 'META-INF/rewrite/*.yml' 'META-INF/rewrite/recipes.csv' -d "$extract_dir" >/dev/null 2>&1; then
    return 0
  fi

  local csv_file="$extract_dir/META-INF/rewrite/recipes.csv"
  if [[ -f "$csv_file" ]]; then
    python3 - "$csv_file" "$group" "$artifact" "$version" <<'PY' >> "$out_ndjson"
import csv
import json
import sys

csv_path, group, artifact, version = sys.argv[1:5]

def clean(value: str):
    value = (value or "").strip()
    return value if value else None

def get_field(row, columns, key):
    idx = columns.get(key)
    if idx is None or idx >= len(row):
        return ""
    return row[idx]

with open(csv_path, "r", encoding="utf-8", errors="replace", newline="") as handle:
    reader = csv.reader(handle)
    try:
        header = next(reader)
    except StopIteration:
        sys.exit(0)

    columns = {name: i for i, name in enumerate(header)}

    for row in reader:
        if not row:
            continue

        name = clean(get_field(row, columns, "name"))
        if not name:
            continue

        recipe_count_raw = clean(get_field(row, columns, "recipeCount")) or "0"
        try:
            recipe_count = int(recipe_count_raw)
        except ValueError:
            recipe_count = 0

        record = {
            "name": name,
            "displayName": clean(get_field(row, columns, "displayName")),
            "description": clean(get_field(row, columns, "description")),
            "recipeListNames": [],
            "methodRefs": [],
            "composite": recipe_count > 1,
            "artifact": {
                "group": group,
                "artifact": artifact,
                "version": version,
            },
            "source": "META-INF/rewrite/recipes.csv",
            "csvMeta": {
                "ecosystem": clean(get_field(row, columns, "ecosystem")),
                "packageName": clean(get_field(row, columns, "packageName")),
                "recipeCount": recipe_count,
                "category1": clean(get_field(row, columns, "category1")),
                "category2": clean(get_field(row, columns, "category2")),
                "category3": clean(get_field(row, columns, "category3")),
                "category4": clean(get_field(row, columns, "category4")),
                "category5": clean(get_field(row, columns, "category5")),
                "category6": clean(get_field(row, columns, "category6")),
                "options": clean(get_field(row, columns, "options")),
                "dataTables": clean(get_field(row, columns, "dataTables")),
            },
        }
        print(json.dumps(record, ensure_ascii=True))
PY
  fi

  while IFS= read -r file; do
    [[ -z "$file" ]] && continue
    local source="${file#${extract_dir}/}"
    local method_refs_json
    method_refs_json="$(python3 - "$file" <<'PY'
import json
import re
import sys

path = sys.argv[1]
pattern = re.compile(r'((?:[A-Za-z_$][A-Za-z0-9_$]*\.)+[A-Za-z_$][A-Za-z0-9_$]*)\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*\(')
with open(path, "r", encoding="utf-8", errors="replace") as f:
    text = f.read()

refs = sorted({(owner + "#" + name).lower() for owner, name in pattern.findall(text)})
print(json.dumps(refs, ensure_ascii=True))
PY
)"

    local docs
    docs="$(yq -o=json -I=0 'select(.type == "specs.openrewrite.org/v1beta/recipe")' "$file" 2>/dev/null || true)"

    [[ -z "$docs" ]] && continue

    while IFS= read -r doc; do
      [[ -z "$doc" ]] && continue
      printf '%s\n' "$doc" | jq -c \
        --arg g "$group" \
        --arg a "$artifact" \
        --arg v "$version" \
        --arg s "$source" \
        --argjson refs "$method_refs_json" '
        def child_names:
          [(.recipeList // [])[] |
            if type == "string" then .
            elif type == "object" then (to_entries[0].key // empty)
            else empty
            end
          ];
        (child_names) as $children
        | {
            name: (.name // empty),
            displayName: (.displayName // null),
            description: (.description // null),
            recipeListNames: $children,
            methodRefs: ($refs // []),
            composite: (($children | length) > 0),
            artifact: {
              group: $g,
              artifact: $a,
              version: $v
            },
            source: $s,
            csvMeta: null
          }
        | select(.name != "")
      ' >> "$out_ndjson"
    done <<< "$docs"
  done < <(find "$extract_dir/META-INF/rewrite" -type f -name '*.yml' | sort)
}

extract_source_method_refs_from_sources_jar() {
  local sources_jar="$1"
  local out_ndjson="$2"
  local extract_root="$3"
  local group="$4"
  local artifact="$5"
  local version="$6"

  local extract_dir="${extract_root}/${group//./_}-${artifact}-${version}-sources"
  rm -rf "$extract_dir"
  mkdir -p "$extract_dir"

  if ! unzip -qq "$sources_jar" '*.java' -d "$extract_dir" >/dev/null 2>&1; then
    return 0
  fi

  python3 - "$extract_dir" <<'PY' >> "$out_ndjson"
import json
import os
import re
import sys

root = sys.argv[1]
pattern = re.compile(r'((?:[A-Za-z_$][A-Za-z0-9_$]*\.)+[A-Za-z_$][A-Za-z0-9_$]*)\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*\(')

for dirpath, _, filenames in os.walk(root):
    for filename in filenames:
        if not filename.endswith(".java"):
            continue
        path = os.path.join(dirpath, filename)
        rel = os.path.relpath(path, root).replace(os.sep, "/")
        if rel.startswith("META-INF/"):
            continue
        fqcn = rel[:-5].replace("/", ".")
        if fqcn.endswith("package-info") or fqcn.endswith("module-info"):
            continue
        try:
            with open(path, "r", encoding="utf-8", errors="replace") as handle:
                text = handle.read()
        except OSError:
            continue

        refs = sorted({(owner + "#" + name).lower() for owner, name in pattern.findall(text)})
        if refs:
            print(json.dumps({"name": fqcn, "methodRefs": refs}, ensure_ascii=True))
PY
}

sync_catalog() {
  local tmp_dir
  tmp_dir="$(mktemp -d "${TMPDIR:-/tmp}/orw-catalog-sync.XXXXXX")"

  local recipes_ndjson="$tmp_dir/recipes.ndjson"
  local source_refs_ndjson="$tmp_dir/source-refs.ndjson"
  local manifest_ndjson="$tmp_dir/manifest.ndjson"
  local warnings_ndjson="$tmp_dir/warnings.ndjson"
  local extract_root="$tmp_dir/extract"
  : > "$recipes_ndjson"
  : > "$source_refs_ndjson"
  : > "$manifest_ndjson"
  : > "$warnings_ndjson"
  mkdir -p "$extract_root"

  while IFS= read -r ga; do
    [[ -z "$ga" ]] && continue

    local resolved
    resolved="$(resolve_recipe_jar_release "$ga" || true)"
    if [[ -z "$resolved" ]]; then
      warn_json_line "catalog: unable to resolve recipe artifact release for $ga" >> "$warnings_ndjson"
      continue
    fi

    local group artifact version jar_path
    group="$(printf '%s' "$resolved" | cut -f1)"
    artifact="$(printf '%s' "$resolved" | cut -f2)"
    version="$(printf '%s' "$resolved" | cut -f3)"
    jar_path="$(printf '%s' "$resolved" | cut -f4-)"

    jq -nc \
      --arg group "$group" \
      --arg artifact "$artifact" \
      --arg version "$version" \
      --arg jar "$jar_path" \
      '{group: $group, artifact: $artifact, version: $version, jar: $jar}' >> "$manifest_ndjson"

    extract_recipes_from_jar "$jar_path" "$group" "$artifact" "$version" "$recipes_ndjson" "$extract_root"

    local sources_jar_path
    sources_jar_path="$(resolve_recipe_sources_jar "$group" "$artifact" "$version" || true)"
    if [[ -n "$sources_jar_path" && -f "$sources_jar_path" ]]; then
      extract_source_method_refs_from_sources_jar "$sources_jar_path" "$source_refs_ndjson" "$extract_root" "$group" "$artifact" "$version"
    fi
  done < <(official_catalog_artifacts)

  jq -n \
    --slurpfile recipes "$recipes_ndjson" \
    --slurpfile source_refs "$source_refs_ndjson" '
    def not_blank:
      . != null and ((type == "string" and . != "") or (type != "string"));
    def merge_text($a; $b):
      if ($a | not_blank) then $a
      elif ($b | not_blank) then $b
      else null
      end;
    def merge_record($old; $new):
      (($old.recipeListNames // []) + ($new.recipeListNames // []) | unique) as $children
      | (($old.methodRefs // []) + ($new.methodRefs // []) | unique) as $method_refs
      | {
          name: ($old.name // $new.name),
          displayName: merge_text($old.displayName; $new.displayName),
          description: merge_text($old.description; $new.description),
          recipeListNames: $children,
          methodRefs: $method_refs,
          composite: (($old.composite // false) or ($new.composite // false) or (($children | length) > 0)),
          artifact: ($old.artifact // $new.artifact),
          sources: (($old.sources // [($old.source // empty)]) + ($new.sources // [($new.source // empty)]) | map(select(. != null and . != "")) | unique),
          source: (($old.sources // [($old.source // empty)]) + ($new.sources // [($new.source // empty)]) | map(select(. != null and . != "")) | unique | join(",")),
          csvMeta: ($old.csvMeta // $new.csvMeta)
        };

    ($recipes // []) as $recipe_records
    | ($source_refs // []) as $source_records
    | ($recipe_records | map(select(.name != null and .name != ""))) as $records
    | ($records | reduce .[] as $r ({}; .[$r.name] = merge_record((.[$r.name] // {}); $r))) as $by_name
    | ($source_records | reduce .[] as $s ($by_name;
        if (.[$s.name] // null) == null then
          .
        else
          .[$s.name].methodRefs = ((.[$s.name].methodRefs // []) + ($s.methodRefs // []) | unique)
        end
      )) as $by_name
    | [$by_name[]]
    | map(
        . + {
          searchText: ([
            .name,
            (.displayName // ""),
            (.description // ""),
            (.source // ""),
            ((.recipeListNames // []) | join(" ")),
            ((.csvMeta.packageName // "")),
            ((.csvMeta.category1 // "")),
            ((.csvMeta.category2 // "")),
            ((.csvMeta.category3 // "")),
            ((.csvMeta.category4 // "")),
            ((.csvMeta.category5 // "")),
            ((.csvMeta.category6 // "")),
            ((.methodRefs // []) | join(" "))
          ] | join(" ") | ascii_downcase)
        }
        | . + {
            searchTokens: (.searchText | gsub("[^a-z0-9]+"; " ") | split(" ") | map(select(length > 1)) | unique)
        }
      )
    | sort_by(.name)
  ' > "$CATALOG_INDEX_JSON"

  jq -s 'sort_by(.group, .artifact)' "$manifest_ndjson" > "$CATALOG_MANIFEST_JSON"
  jq -s '.' "$warnings_ndjson" > "$CATALOG_WARNINGS_JSON"

  rm -rf "$tmp_dir"
}

catalog_refreshed="false"
if [[ "$REFRESH_CATALOG" == "true" || ! -f "$CATALOG_INDEX_JSON" ]]; then
  sync_catalog
  catalog_refreshed="true"
fi

if [[ ! -f "$CATALOG_INDEX_JSON" ]]; then
  echo "error: catalog index missing at $CATALOG_INDEX_JSON" >&2
  exit 1
fi

if ! jq -e 'type == "array"' "$CATALOG_INDEX_JSON" >/dev/null; then
  echo "error: catalog index is invalid: $CATALOG_INDEX_JSON" >&2
  exit 1
fi

tmp_dir="$(mktemp -d "${TMPDIR:-/tmp}/orw-plan.XXXXXX")"
changes_json="$tmp_dir/changes.json"
target_json="$tmp_dir/target.json"
plan_base_json="$tmp_dir/plan.base.json"
plan_final_json="$OUT_DIR/plan.json"
coverage_json="$OUT_DIR/coverage.json"
rewrite_yaml="$OUT_DIR/rewrite.yml"
manual_gaps_md="$OUT_DIR/manual-gaps.md"

jq '.target' "$REPORT_FILE" > "$target_json"

jq '
  def words:
    (. // "" | tostring | ascii_downcase | gsub("[^a-z0-9]+"; " ") | split(" ") | map(select(length > 2)));
  def method_name($member):
    if ($member == null or $member == "") then ""
    else (($member | capture("^(?<n>[^\\(]+)").n?) // $member)
    end;
  [
    (.updates // []) | to_entries[] as $u
    | ($u.value.ga // "") as $uga
    | ($u.value.changes // []) | to_entries[] as $c
    | ($c.value.class // "") as $class
    | ($c.value.member // "") as $member
    | (method_name($member)) as $method
    | {
        changeId: ("u" + ($u.key|tostring) + "c" + ($c.key|tostring)),
        updateGA: $uga,
        kind: ($c.value.kind // "unknown"),
        class: $class,
        simpleClass: (($class | split(".")) | last // ""),
        member: ($c.value.member // null),
        methodName: $method,
        type: ($c.value.type // ""),
        javadocNote: ($c.value.deprecation_note // ""),
        tokens: ((
          ($uga | words)
          + ($class | words)
          + (($c.value.member // "") | words)
          + (($c.value.type // "") | words)
          + (($c.value.deprecation_note // "") | words)
        ) | unique)
      }
  ]
' "$REPORT_FILE" > "$changes_json"

generated_at="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"

jq -n \
  --slurpfile changes "$changes_json" \
  --slurpfile recipes "$CATALOG_INDEX_JSON" \
  --slurpfile target "$target_json" \
  --arg report_file "$REPORT_FILE" \
  --arg generated_at "$generated_at" \
  --arg catalog_dir "$CATALOG_DIR" \
  --arg catalog_scope "$CATALOG_SCOPE" \
  --arg ranker_mode "$RANKER_MODE" \
  --argjson catalog_refreshed "$( [[ "$catalog_refreshed" == "true" ]] && echo true || echo false )" '
  def contains_text($text; $needle):
    ($needle | length) > 0 and ($text | contains($needle));

  def has_token($tokens; $token):
    ($token | length) > 0 and (($tokens | index($token)) != null);

  def score_change($c; $r):
    ($r.searchText // "") as $text
    | ($r.searchTokens // []) as $search_tokens
    | ($r.methodRefs // []) as $method_refs
    | ($c.kind // "" | ascii_downcase) as $kind
    | ($c.class // "" | ascii_downcase) as $class
    | ($c.simpleClass // "" | ascii_downcase) as $simple
    | ($c.methodName // "" | ascii_downcase) as $method
    | (($class + "#" + $method) | ascii_downcase) as $method_ref
    | ($c.updateGA // "" | ascii_downcase) as $ga
    | (($ga | split("@")[0] | split(":") | .[1]?) // "") as $artifact
    | if ($kind == "method" and (($class | length) == 0 or ($method | length) == 0 or (($method_refs | index($method_ref)) == null))) then
        0
      else
        (if contains_text($text; $class) then 160 else 0 end) as $class_score
        | (if has_token($search_tokens; $simple) then 70 else 0 end) as $simple_score
        | (if has_token($search_tokens; $method) then 90 else 0 end) as $method_score
        | (if has_token($search_tokens; $artifact) then 55 else 0 end) as $artifact_score
        | ($class_score + $simple_score + $method_score + $artifact_score) as $anchor_score
        | if $anchor_score == 0 then
            0
          else
            $anchor_score
            + (reduce ($c.tokens // [])[] as $t (0;
                . + (if (($t | length) > 2 and has_token($search_tokens; $t)) then 2 else 0 end)
              ))
          end
      end;

  def coverage_by_score($s):
    if $s >= 110 then "covered_fully"
    elif $s >= 60 then "covered_partially"
    else "uncovered"
    end;

  def priority($name):
    if ($name | test("UpgradeSpringBoot_|MigrateToSpringBoot_")) then 1
    elif ($name | test("Upgrade.*Framework|Migrate.*Framework|UpgradeToJava|MigrateToJava|MigrateToQuarkus|UpgradeToQuarkus")) then 2
    elif ($name | test("BestPractices")) then 4
    else 3
    end;

  def expand($by; $name):
    ($by[$name] // {recipeListNames: []}) as $node
    | if (($node.recipeListNames // []) | length) == 0 then
        [$name]
      else
        [($node.recipeListNames[] | expand($by; .))[]]
      end;

  def dedupe_preserve:
    reduce .[] as $item ([];
      if index($item) == null then . + [$item] else . end
    );

  ($changes[0] // []) as $changes_arr
  | ($recipes[0] // []) as $recipes_arr
  | ($target[0] // {}) as $target_obj
  | (reduce $recipes_arr[] as $r ({}; .[$r.name] = $r)) as $recipe_by_name
  | ($changes_arr | map(
      . as $c
      | {
          changeId,
          updateGA,
          kind,
          class,
          member,
          type,
          javadocNote,
          topCandidates: (
            [ $recipes_arr[] as $r
              | {
                  name: $r.name,
                  score: score_change($c; $r),
                  composite: ($r.composite // false),
                  displayName: ($r.displayName // null),
                  artifact: ($r.artifact // null)
                }
              | select(.score > 0)
            ]
            | sort_by(-.score, .name)
            | .[0:12]
          )
        }
      | .bestScore = ((.topCandidates[0].score) // 0)
      | .coverage = coverage_by_score(.bestScore)
      | .selectedRecipe = (if .bestScore >= 60 then (.topCandidates[0].name // null) else null end)
    )) as $change_results
  | ($change_results | map(.selectedRecipe) | map(select(. != null)) | unique) as $selected_from_changes
  | (($target_obj.ga // "") | ascii_downcase) as $target_ga
  | (($target_obj.to // "") | tostring) as $target_to
  | (
      if ($target_ga == "org.springframework.boot:spring-boot-dependencies") then
        (
          ($target_to | capture("^(?<maj>[0-9]+)\\.(?<min>[0-9]+)")?) as $vm
          | if ($vm == null) then []
            else
              (
                "org.openrewrite.java.spring.boot3.UpgradeSpringBoot_\($vm.maj)_\($vm.min)"
              ) as $candidate
              | if ($recipe_by_name[$candidate] != null) then [$candidate]
                elif ($recipe_by_name["org.openrewrite.java.spring.boot3.UpgradeSpringBoot_3_0"] != null) then ["org.openrewrite.java.spring.boot3.UpgradeSpringBoot_3_0"]
                else []
                end
            end
        )
      else
        []
      end
    ) as $selected_extras
  | (($selected_from_changes + $selected_extras) | unique) as $selected
  | ($selected | map(select(($recipe_by_name[.]?.composite // false)))) as $selected_composites
  | ($selected_composites | sort_by(priority(.), .)) as $composite_order
  | ($selected | map(select((($recipe_by_name[.]?.composite // false) | not)))) as $selected_non_composites
  | (
      ([ $composite_order[] | expand($recipe_by_name; .)[] ]
       + [ $selected_non_composites[] | expand($recipe_by_name; .)[] ])
      | dedupe_preserve
    ) as $atomic_order
  | (
      if ($composite_order | length) > 0 then
        ($composite_order + $selected_non_composites | dedupe_preserve)
      else
        $atomic_order
      end
    ) as $rewrite_recipe_list
  | {
      generatedAt: $generated_at,
      inputReport: $report_file,
      catalog: {
        dir: $catalog_dir,
        scope: $catalog_scope,
        recipeCount: ($recipes_arr | length),
        refreshed: $catalog_refreshed
      },
      ranker: {
        mode: $ranker_mode,
        fallback: (if $ranker_mode == "hybrid" then true else false end),
        fallbackReason: (if $ranker_mode == "hybrid" then "no ORW_HYBRID_RERANKER_CMD configured; deterministic ranking used" else null end)
      },
      target: $target_obj,
      changes: $change_results,
      selected: {
        composite_order: $composite_order,
        atomic_order: $atomic_order,
        rewrite_recipe_list: $rewrite_recipe_list
      }
    }
' > "$plan_base_json"

# Optional hybrid rerank hook.
if [[ "$RANKER_MODE" == "hybrid" && -n "${ORW_HYBRID_RERANKER_CMD:-}" ]]; then
  reranked_json="$tmp_dir/plan.reranked.json"
  if bash -lc "$ORW_HYBRID_RERANKER_CMD" < "$plan_base_json" > "$reranked_json" 2>/dev/null; then
    if jq -e 'type == "object" and (.changes|type)=="array" and (.selected|type)=="object"' "$reranked_json" >/dev/null; then
      jq '.ranker.fallback = false | .ranker.fallbackReason = null' "$reranked_json" > "$plan_final_json"
    else
      cp "$plan_base_json" "$plan_final_json"
    fi
  else
    cp "$plan_base_json" "$plan_final_json"
  fi
else
  cp "$plan_base_json" "$plan_final_json"
fi

jq '
  {
    generatedAt: .generatedAt,
    inputReport: .inputReport,
    summary: {
      totalChanges: (.changes | length),
      coveredFully: ([.changes[] | select(.coverage == "covered_fully")] | length),
      coveredPartially: ([.changes[] | select(.coverage == "covered_partially")] | length),
      uncovered: ([.changes[] | select(.coverage == "uncovered")] | length)
    },
    changes: [
      .changes[]
      | {
          changeId,
          updateGA,
          class,
          member,
          type,
          coverage,
          selectedRecipe,
          bestScore,
          topCandidates: (.topCandidates[0:3])
        }
    ]
  }
' "$plan_final_json" > "$coverage_json"

rewrite_recipe_list=()
while IFS= read -r recipe; do
  [[ -z "$recipe" ]] && continue
  rewrite_recipe_list+=("$recipe")
done < <(jq -r '.selected.rewrite_recipe_list[]?' "$plan_final_json")

{
  echo "type: specs.openrewrite.org/v1beta/recipe"
  echo "name: ploy.openrewrite.GeneratedPlan"
  echo "displayName: Generated OpenRewrite plan"
  echo "description: Generated from report-based migration analysis."
  echo "recipeList:"
  if [[ ${#rewrite_recipe_list[@]} -eq 0 ]]; then
    echo "  []"
  else
    for recipe in "${rewrite_recipe_list[@]}"; do
      echo "  - $recipe"
    done
  fi
} > "$rewrite_yaml"

uncovered_changes=()
while IFS= read -r row; do
  [[ -z "$row" ]] && continue
  uncovered_changes+=("$row")
done < <(jq -c '.changes[] | select(.coverage == "uncovered")' "$plan_final_json")

{
  echo "# Manual Migration Gaps"
  echo
  printf 'Generated from: `%s`\n' "$REPORT_FILE"
  echo
  echo "Uncovered changes: ${#uncovered_changes[@]}"
  echo

  if [[ ${#uncovered_changes[@]} -eq 0 ]]; then
    echo "All changes are covered fully or partially by detected official OSS recipes."
    echo
  fi

  idx=1
  for row in "${uncovered_changes[@]}"; do
    change_id="$(printf '%s' "$row" | jq -r '.changeId')"
    update_ga="$(printf '%s' "$row" | jq -r '.updateGA')"
    class_name="$(printf '%s' "$row" | jq -r '.class')"
    member_name="$(printf '%s' "$row" | jq -r '.member // ""')"
    change_type="$(printf '%s' "$row" | jq -r '.type')"
    deprecation_note="$(printf '%s' "$row" | jq -r '.javadocNote // ""')"

    safe_id="$(printf '%s' "$change_id" | tr -cd '[:alnum:]')"
    method_name=""
    if [[ -n "$member_name" ]]; then
      method_name="$(printf '%s' "$member_name" | sed -E 's/\(.*$//')"
    fi

    echo "## Gap ${idx}: ${change_id}"
    echo
    printf -- '- Dependency: `%s`\n' "$update_ga"
    printf -- '- Type: `%s`\n' "$change_type"
    printf -- '- Class: `%s`\n' "$class_name"
    if [[ -n "$member_name" ]]; then
      printf -- '- Member: `%s`\n' "$member_name"
    fi
    if [[ -n "$deprecation_note" ]]; then
      echo "- Deprecation note: ${deprecation_note}"
    fi
    echo
    echo "Top low-confidence candidates:"
    jq -r '.topCandidates[0:3][]? | "- `\(.name)` (score=\(.score))"' <<< "$row"
    echo
    echo "Suggested custom recipe stub:"
    echo '```yaml'
    echo '---'
    echo 'type: specs.openrewrite.org/v1beta/recipe'
    echo "name: com.yourorg.rewrite.Todo${safe_id}"
    echo "displayName: TODO migrate ${class_name}${member_name:+#$member_name}"
    echo "description: Manual migration required for ${class_name}${member_name:+#$member_name} (${change_type})."
    echo 'recipeList:'
    if [[ -n "$member_name" && -n "$method_name" ]]; then
      echo '  - org.openrewrite.java.search.FindMethods:'
      echo "      methodPattern: \"${class_name} ${method_name}(..)\""
    else
      echo '  - org.openrewrite.java.search.FindTypes:'
      echo "      fullyQualifiedTypeName: \"${class_name}\""
    fi
    echo '```'
    echo

    idx=$((idx + 1))
  done
} > "$manual_gaps_md"

rm -rf "$tmp_dir"

echo "wrote: $plan_final_json"
echo "wrote: $coverage_json"
echo "wrote: $rewrite_yaml"
echo "wrote: $manual_gaps_md"
