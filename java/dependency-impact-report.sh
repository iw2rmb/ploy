#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE' >&2
Usage:
  ./dependency-impact-report.sh \
    --target <groupId:artifactId@toVersion | groupId:artifactId@fromVersion..toVersion> \
    --classpath-file <path> \
    [--repo <path>] \
    [--output <path>] \
    [--work-dir <path>] \
    [--repo-url <url>]

Notes:
  - If --target uses @from..to, build-file version lookup is skipped.
  - If --target uses @to only, current version is resolved from direct declarations
    in pom.xml / build.gradle / build.gradle.kts (no property/catalog indirection).
USAGE
}

if [[ $# -eq 0 ]]; then
  usage
  exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TOOL_POM="$SCRIPT_DIR/pom.xml"
MDEP_PLUGIN="org.apache.maven.plugins:maven-dependency-plugin:3.8.1"

TARGET=""
CLASSPATH_FILE=""
REPO_PATH="$(pwd)"
OUTPUT_FILE=""
WORK_DIR=""
REPO_URL=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --target)
      TARGET="${2:-}"
      shift 2
      ;;
    --classpath-file)
      CLASSPATH_FILE="${2:-}"
      shift 2
      ;;
    --repo)
      REPO_PATH="${2:-}"
      shift 2
      ;;
    --output)
      OUTPUT_FILE="${2:-}"
      shift 2
      ;;
    --work-dir)
      WORK_DIR="${2:-}"
      shift 2
      ;;
    --repo-url)
      REPO_URL="${2:-}"
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

if [[ -z "$TARGET" ]]; then
  echo "error: --target is required" >&2
  exit 1
fi
if [[ -z "$CLASSPATH_FILE" ]]; then
  echo "error: --classpath-file is required" >&2
  exit 1
fi
if [[ ! -f "$CLASSPATH_FILE" ]]; then
  echo "error: classpath file does not exist: $CLASSPATH_FILE" >&2
  exit 1
fi
if [[ ! -d "$REPO_PATH" ]]; then
  echo "error: repo path does not exist: $REPO_PATH" >&2
  exit 1
fi

for bin in jq rg mvn xmllint java; do
  if ! command -v "$bin" >/dev/null 2>&1; then
    echo "error: required tool not found: $bin" >&2
    exit 1
  fi
done

REPO_PATH="$(cd "$REPO_PATH" && pwd)"
CLASSPATH_FILE="$(cd "$(dirname "$CLASSPATH_FILE")" && pwd)/$(basename "$CLASSPATH_FILE")"

if [[ -z "$WORK_DIR" ]]; then
  WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/java-impact-report.XXXXXX")"
else
  mkdir -p "$WORK_DIR"
  WORK_DIR="$(cd "$WORK_DIR" && pwd)"
fi

TMP_DIR="$WORK_DIR/tmp"
mkdir -p "$TMP_DIR"

GA="${TARGET%@*}"
VERSION_SPEC="${TARGET#*@}"
if [[ "$GA" == "$TARGET" || -z "$GA" || -z "$VERSION_SPEC" ]]; then
  echo "error: invalid --target format, expected groupId:artifactId@version or @from..to" >&2
  exit 1
fi

GROUP_ID="${GA%%:*}"
ARTIFACT_ID="${GA#*:}"
if [[ -z "$GROUP_ID" || -z "$ARTIFACT_ID" || "$GROUP_ID" == "$GA" ]]; then
  echo "error: invalid groupId:artifactId in --target: $GA" >&2
  exit 1
fi

warnings_file="$TMP_DIR/warnings.ndjson"
: > "$warnings_file"

record_warning() {
  local message="$1"
  jq -nc --arg message "$message" '$message' >> "$warnings_file"
}

escape_regex() {
  local value="$1"
  printf '%s' "$value" | sed -E 's/[][(){}.^$*+?|\\]/\\&/g'
}

resolve_current_version_from_build_files() {
  local repo="$1"
  local group_id="$2"
  local artifact_id="$3"

  local tmp_matches="$TMP_DIR/version.matches.tsv"
  : > "$tmp_matches"

  while IFS= read -r pom; do
    local version
    version="$(xmllint --xpath \
      "string((//*[local-name()='dependency'][*[local-name()='groupId' and normalize-space(text())='${group_id}'] and *[local-name()='artifactId' and normalize-space(text())='${artifact_id}'] and *[local-name()='version']])[1]/*[local-name()='version'][1])" \
      "$pom" 2>/dev/null || true)"
    version="$(printf '%s' "$version" | tr -d '\r' | xargs || true)"
    if [[ -n "$version" ]] && [[ "$version" != *'${'* ]]; then
      printf '%s\t%s\t%s\n' "$pom" "xml-dependency" "$version" >> "$tmp_matches"
    fi
  done < <(find "$repo" -type f -name 'pom.xml' | sort)

  local group_re artifact_re
  group_re="$(escape_regex "$group_id")"
  artifact_re="$(escape_regex "$artifact_id")"

  while IFS= read -r gradle_file; do
    while IFS= read -r line; do
      local match version
      match="${line##*:}"
      version="$match"
      version="${version#\'}"
      version="${version#\"}"
      version="${version%\'}"
      version="${version%\"}"
      if [[ -n "$version" ]] && [[ "$version" != *'${'* ]] && [[ "$version" != *'$'* ]]; then
        printf '%s\t%s\t%s\n' "$gradle_file" "gradle-gav-string" "$version" >> "$tmp_matches"
      fi
    done < <(rg -n --no-heading -o "\"${group_re}:${artifact_re}:[^\"]+\"|'${group_re}:${artifact_re}:[^']+'" "$gradle_file" || true)

    while IFS= read -r line; do
      local match version
      match="${line##*:}"
      version="$match"
      version="${version#\'}"
      version="${version#\"}"
      version="${version%\'}"
      version="${version%\"}"
      if [[ -n "$version" ]] && [[ "$version" != *'${'* ]] && [[ "$version" != *'$'* ]]; then
        printf '%s\t%s\t%s\n' "$gradle_file" "gradle-map-style" "$version" >> "$tmp_matches"
      fi
    done < <(rg -n --no-heading -o "group\\s*[:=]\\s*['\"]${group_re}['\"][^\\n]*name\\s*[:=]\\s*['\"]${artifact_re}['\"][^\\n]*version\\s*[:=]\\s*['\"]([^'\"$]+)['\"]" -r '$1' "$gradle_file" || true)
  done < <(find "$repo" -type f \( -name 'build.gradle' -o -name 'build.gradle.kts' \) | sort)

  if [[ ! -s "$tmp_matches" ]]; then
    return 1
  fi

  sort -u "$tmp_matches" > "$TMP_DIR/version.matches.sorted.tsv"

  local unique_versions
  unique_versions="$(cut -f3 "$TMP_DIR/version.matches.sorted.tsv" | sort -u)"
  local unique_count
  unique_count="$(printf '%s\n' "$unique_versions" | sed '/^$/d' | wc -l | tr -d ' ')"
  if [[ "$unique_count" -gt 1 ]]; then
    record_warning "multiple direct versions found in build files for ${group_id}:${artifact_id}; selecting deterministic first match (sorted by file path)."
  fi

  # Known risk: we intentionally do not resolve multi-module conflicts now.
  record_warning "multi-module conflict resolution is not implemented; direct-first selection may be inaccurate when modules use different versions."

  local first
  first="$(head -n1 "$TMP_DIR/version.matches.sorted.tsv")"
  printf '%s' "$first" | cut -f3
}

FROM_VERSION=""
TO_VERSION=""
if [[ "$VERSION_SPEC" == *".."* ]]; then
  FROM_VERSION="${VERSION_SPEC%%..*}"
  TO_VERSION="${VERSION_SPEC#*..}"
  if [[ -z "$FROM_VERSION" || -z "$TO_VERSION" || "$TO_VERSION" == "$FROM_VERSION" ]]; then
    echo "error: invalid version range in --target: $VERSION_SPEC" >&2
    exit 1
  fi
else
  TO_VERSION="$VERSION_SPEC"
  if [[ -z "$TO_VERSION" ]]; then
    echo "error: empty target version in --target" >&2
    exit 1
  fi
  FROM_VERSION="$(resolve_current_version_from_build_files "$REPO_PATH" "$GROUP_ID" "$ARTIFACT_ID" || true)"
  if [[ -z "$FROM_VERSION" ]]; then
    echo "error: current version for ${GA} was not found in pom.xml/build.gradle/build.gradle.kts direct declarations" >&2
    exit 1
  fi
fi

if [[ "$FROM_VERSION" == "$TO_VERSION" ]]; then
  record_warning "from and to versions are equal (${FROM_VERSION}); resulting updates are expected to be empty."
fi

tool_classpath_file="$TMP_DIR/tool.classpath"
mvn -q -f "$TOOL_POM" -DskipTests compile
mvn -q -f "$TOOL_POM" "$MDEP_PLUGIN:build-classpath" -DincludeScope=runtime "-Dmdep.outputFile=${tool_classpath_file}"
TOOL_RUNTIME_CP="$(cat "$tool_classpath_file"):$SCRIPT_DIR/target/classes"

resolve_managed_dependencies() {
  local ga="$1"
  local version="$2"
  local out_file="$3"

  local coordinate="${ga}@${version}"
  local cmd=(java -cp "$TOOL_RUNTIME_CP" DependencyBomResolver "$coordinate")
  if [[ -n "$REPO_URL" ]]; then
    cmd+=( "$REPO_URL" )
  fi

  if ! "${cmd[@]}" > "$out_file"; then
    echo "error: failed to resolve managed dependencies for ${coordinate}" >&2
    exit 1
  fi

  local dep_count
  dep_count="$(jq '(.dependencies // []) | length' "$out_file")"
  if [[ "$dep_count" -eq 0 ]]; then
    jq -n \
      --arg coordinate "$coordinate" \
      --arg groupId "$GROUP_ID" \
      --arg artifactId "$ARTIFACT_ID" \
      --arg version "$version" \
      '{
        bomCoordinate: $coordinate,
        dependencies: [
          {groupId: $groupId, artifactId: $artifactId, version: $version}
        ]
      }' > "$out_file"
    record_warning "target ${coordinate} has no dependencyManagement entries; using non-BOM fallback with the target dependency itself."
  fi
}

old_md_json="$WORK_DIR/managed-old.json"
new_md_json="$WORK_DIR/managed-new.json"
resolve_managed_dependencies "$GA" "$FROM_VERSION" "$old_md_json"
resolve_managed_dependencies "$GA" "$TO_VERSION" "$new_md_json"

compare_json="$WORK_DIR/managed-compare.json"
"$SCRIPT_DIR/compare.sh" "$old_md_json" "$new_md_json" > "$compare_json"

du_json="$WORK_DIR/dependency-usage.nofilter.json"
"$SCRIPT_DIR/extract-usage.sh" \
  --repo "$REPO_PATH" \
  --classpath-file "$CLASSPATH_FILE" \
  --no-target-filter \
  --output "$du_json"

du_by_ga_json="$WORK_DIR/dependency-usage.by-ga.json"
jq '
  reduce (.usages // [])[] as $u (
    {};
    if (($u.groupId // "unknown") == "unknown" or ($u.artifactId // "unknown") == "unknown") then
      .
    else
      .[$u.groupId + ":" + $u.artifactId] =
        ((.[$u.groupId + ":" + $u.artifactId] // []) + ($u.symbols // []))
    end
  )
  | with_entries(.value |= (unique | sort))
' "$du_json" > "$du_by_ga_json"

compare_used_json="$WORK_DIR/managed-compare.used.json"
jq --argjson du "$(cat "$du_by_ga_json")" '
  .changed = [(.changed // [])[] | select($du[.ga] != null)]
  | .removed = [(.removed // [])[] | select($du[.ga] != null)]
  | .added = (.added // [])
' "$compare_json" > "$compare_used_json"

japicmp_index_json="$WORK_DIR/japicmp-report-index.json"
"$SCRIPT_DIR/japicmp-compare.sh" "$compare_used_json" "$japicmp_index_json" "$WORK_DIR/japicmp-work"

reports_ndjson="$TMP_DIR/japicmp-reports.ndjson"
: > "$reports_ndjson"
while IFS= read -r report_file; do
  if [[ -n "$report_file" && -f "$report_file" ]]; then
    jq -c '.' "$report_file" >> "$reports_ndjson"
  fi
done < <(jq -r '.changed_artifacts[]? | select(.status == "ok") | .report_file' "$japicmp_index_json")

reports_json="$WORK_DIR/japicmp-reports.json"
if [[ -s "$reports_ndjson" ]]; then
  jq -s '.' "$reports_ndjson" > "$reports_json"
else
  echo '[]' > "$reports_json"
fi

warnings_json="$WORK_DIR/warnings.json"
if [[ -s "$warnings_file" ]]; then
  jq -s 'unique' "$warnings_file" > "$warnings_json"
else
  echo '[]' > "$warnings_json"
fi

report_json="$WORK_DIR/report.json"
jq -n \
  --arg target "$TARGET" \
  --arg ga "$GA" \
  --arg from "$FROM_VERSION" \
  --arg to "$TO_VERSION" \
  --arg repo "$REPO_PATH" \
  --arg classpath_file "$CLASSPATH_FILE" \
  --slurpfile compareFile "$compare_used_json" \
  --slurpfile duFile "$du_by_ga_json" \
  --slurpfile reportsFile "$reports_json" \
  --slurpfile warningsFile "$warnings_json" '
  ($compareFile[0] // {}) as $compare
  | ($duFile[0] // {}) as $du
  | ($reportsFile[0] // []) as $reports
  | ($warningsFile[0] // []) as $warnings
  |
  def ctor_member($class; $member):
    if $member == null then null
    elif ($member | startswith("<init>(")) then $member
    else
      ($class | split(".") | last) as $simple
      | if ($member | startswith($simple + "(")) then
          "<init>(" + ($member | sub("^" + $simple + "\\("; ""))
        else
          $member
        end
    end;

  def change_candidates($c):
    if ($c.kind == "class") then
      [ $c.class ]
    elif ($c.kind == "interface") then
      [ $c.class, ($c.member // "") ]
    elif ($c.kind == "superclass") then
      [ $c.class ]
    elif ($c.kind == "field") then
      [ $c.class + "#" + ($c.member // "") ]
    elif ($c.kind == "method") then
      [ $c.class + "#" + ($c.member // "") ]
    elif ($c.kind == "constructor") then
      [ $c.class + "#" + ($c.member // ""), $c.class + "#" + (ctor_member($c.class; $c.member) // "") ]
    else
      [ $c.class ]
    end
    | map(select(. != null and . != ""));

  def is_change_used($c; $symbols):
    if ($c.kind == "class" or $c.kind == "interface" or $c.kind == "superclass") then
      any($symbols[]?; (. == $c.class) or startswith($c.class + "#"))
    else
      any(change_candidates($c)[]; . as $candidate | any($symbols[]?; . == $candidate))
    end;

  def removal_javadoc($change; $enriched_removals):
    (
      $enriched_removals
      | map(
          select((.kind // "") == ($change.kind // ""))
          | select((.class // "") == ($change.class // ""))
          | select((.type // "") == ($change.type // ""))
          | select((.member // null) == ($change.member // null))
        )
      | .[0]
    ) as $match
    | {
        note: ($match.javadoc_last_note // null),
        version: ($match.javadoc_last_ver // null)
      };

  {
    target: {
      request: $target,
      ga: $ga,
      from: $from,
      to: $to
    },
    inputs: {
      repo: $repo,
      classpath_file: $classpath_file
    },
    updates: (
      [
        $reports[] as $report
        | ($du[$report.ga] // []) as $symbols
        | ($report.removals // []) as $enriched_removals
        | {
            ga: ($report.ga + "@" + $report.from + "..." + $report.to),
            changes: (
              [
                ($report.incompatible_changes // [])[]
                | select((.type // "") | endswith("_ADDED") | not)
                | select(is_change_used(.; $symbols))
                | {
                    kind: (.kind // "unknown"),
                    class: (.class // "unknown"),
                    member: (.member // null),
                    type: (.type // "unknown"),
                    javadoc: removal_javadoc(.; $enriched_removals)
                  }
              ]
              | unique_by(.kind, .class, .member, .type, .javadoc.note, .javadoc.version)
              | sort_by(.kind, .class, (.member // ""), .type)
            )
          }
        | select((.changes | length) > 0)
      ]
      | sort_by(.ga)
    ),
    removals: (
      [
        ($compare.removed // [])[]
        | . as $removed
        | ($du[$removed.ga] // []) as $usage
        | select(($usage | length) > 0)
        | {
            ga: ($removed.ga + "@" + ($removed.version // "unknown")),
            usage: $usage
          }
      ]
      | sort_by(.ga)
    ),
    warnings: $warnings
  }
' > "$report_json"

if [[ -n "$OUTPUT_FILE" ]]; then
  mkdir -p "$(dirname "$OUTPUT_FILE")"
  cp "$report_json" "$OUTPUT_FILE"
else
  cat "$report_json"
fi
