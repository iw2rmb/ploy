#!/usr/bin/env bash
set -euo pipefail

# Example Stack Gate detector: Java release version + build tool detection.
#
# Scope:
# - Maven (pom.xml)
# - Gradle (build.gradle / build.gradle.kts), including Gradle Kotlin DSL
# - Kotlin JVM hint via kotlinOptions.jvmTarget (best-effort)
#
# Design goals:
# - Filesystem-only detection (no running Maven/Gradle).
# - Deterministic output for a given workspace.
# - Strict: if ambiguous, exit 11 (unknown).
#
# Output (JSON to stdout):
# {
#   "language": "java",
#   "tool": "maven|gradle",
#   "release": 11,
#   "evidence": [{"path":"...","key":"...","value":"..."}]
# }
#
# Exit codes:
# - 0  success (release detected)
# - 11 unknown/ambiguous (cannot reliably detect)

workspace="${1:-/workspace}"

pom="$workspace/pom.xml"
gradle_groovy="$workspace/build.gradle"
gradle_kts="$workspace/build.gradle.kts"

json_escape() {
  local s="${1}"
  s="${s//\\/\\\\}"
  s="${s//\"/\\\"}"
  s="${s//$'\n'/\\n}"
  printf '%s' "$s"
}

emit_unknown() {
  local why="${1:-unknown}"
  printf '{'
  printf '"language":"java","tool":null,"release":null,'
  printf '"reason":"%s","evidence":[]' "$(json_escape "$why")"
  printf '}\n'
  exit 11
}

emit_result() {
  local tool="$1"
  local release="$2"
  shift 2
  local -a evidence_json=("$@")

  printf '{'
  printf '"language":"java","tool":"%s","release":%s,' "$(json_escape "$tool")" "$(json_escape "$release")"
  printf '"evidence":['
  local first=1
  for e in "${evidence_json[@]}"; do
    if [[ $first -eq 0 ]]; then printf ','; fi
    first=0
    printf '%s' "$e"
  done
  printf ']'
  printf '}\n'
  exit 0
}

evidence_item() {
  local path="$1"
  local key="$2"
  local value="$3"
  printf '{"path":"%s","key":"%s","value":"%s"}' \
    "$(json_escape "$path")" "$(json_escape "$key")" "$(json_escape "$value")"
}

normalize_jvm_target() {
  # Accept "17" or "11" or legacy "1.8"/"1_8"
  local v="${1}"
  v="${v//_/.}"
  if [[ "$v" == 1.* ]]; then
    printf '%s' "${v#1.}"
    return 0
  fi
  printf '%s' "$v"
}

extract_first_xml_tag_value_flat() {
  # Heuristic XML extraction:
  # - strips comments
  # - flattens to one line
  # - extracts first <tag>VALUE</tag> occurrence
  local file="$1"
  local tag="$2"
  local flat
  flat="$(sed 's/<!--.*-->//g' "$file" | tr '\n' ' ')"
  # shellcheck disable=SC2001
  echo "$flat" | sed -n "s:.*<${tag}>\\([^<]*\\)</${tag}>.*:\\1:p" | head -n 1 | sed 's/^ *//;s/ *$//'
}

extract_maven_property() {
  # Extract <properties><name>VALUE</name></properties> value (best-effort).
  local file="$1"
  local name="$2"
  awk -v key="$name" '
    BEGIN { inprops=0 }
    /<properties>/ { inprops=1 }
    /<\/properties>/ { inprops=0 }
    inprops==1 {
      # match: <key>value</key>
      # allow dots/dashes/underscores in key
      if ($0 ~ "<"key">") {
        gsub(/^.*<[^>]+>/, "", $0)
        gsub(/<\/[^>]+>.*$/, "", $0)
        gsub(/^[ \t]+|[ \t]+$/, "", $0)
        print $0
        exit
      }
    }
  ' "$file" | head -n 1
}

resolve_maven_value() {
  # Resolve a value that may be a ${property}.
  local file="$1"
  local raw="$2"
  if [[ "$raw" =~ ^\\$\\{([^}]+)\\}$ ]]; then
    local prop="${BASH_REMATCH[1]}"
    local pv
    pv="$(extract_maven_property "$file" "$prop" || true)"
    if [[ -n "$pv" ]]; then
      printf '%s' "$pv"
      return 0
    fi
  fi
  printf '%s' "$raw"
}

detect_maven_release() {
  local file="$1"
  local -a evidence=()

  local r s t jv
  r="$(extract_first_xml_tag_value_flat "$file" "maven.compiler.release" || true)"
  if [[ -n "$r" ]]; then
    r="$(resolve_maven_value "$file" "$r")"
    evidence+=("$(evidence_item "$file" "maven.compiler.release" "$r")")
    if [[ "$r" =~ ^[0-9]+$ ]]; then
      emit_result "maven" "$r" "${evidence[@]}"
      return 0
    fi
    emit_unknown "maven.compiler.release is not a simple integer literal"
  fi

  s="$(extract_first_xml_tag_value_flat "$file" "maven.compiler.source" || true)"
  t="$(extract_first_xml_tag_value_flat "$file" "maven.compiler.target" || true)"
  if [[ -n "$s" || -n "$t" ]]; then
    s="$(resolve_maven_value "$file" "$s")"
    t="$(resolve_maven_value "$file" "$t")"
    [[ -n "$s" ]] && evidence+=("$(evidence_item "$file" "maven.compiler.source" "$s")")
    [[ -n "$t" ]] && evidence+=("$(evidence_item "$file" "maven.compiler.target" "$t")")
    if [[ -n "$s" && -n "$t" && "$s" == "$t" && "$s" =~ ^[0-9]+$ ]]; then
      emit_result "maven" "$s" "${evidence[@]}"
      return 0
    fi
    emit_unknown "maven.compiler.source/target are missing, not equal, or not simple integers"
  fi

  jv="$(extract_first_xml_tag_value_flat "$file" "java.version" || true)"
  if [[ -n "$jv" ]]; then
    jv="$(resolve_maven_value "$file" "$jv")"
    evidence+=("$(evidence_item "$file" "java.version" "$jv")")
    if [[ "$jv" =~ ^[0-9]+$ ]]; then
      emit_result "maven" "$jv" "${evidence[@]}"
      return 0
    fi
    emit_unknown "java.version is not a simple integer literal"
  fi

  emit_unknown "no supported Maven Java version keys found (release/source/target/java.version)"
}

detect_gradle_release() {
  local file="$1"
  local -a evidence=()

  # Prefer source/targetCompatibility (best-effort; must match if both present).
  local sc tc
  sc="$(rg -n --no-heading -o 'sourceCompatibility\\s*=\\s*(?:JavaVersion\\.)?VERSION_[0-9_]+' "$file" 2>/dev/null | head -n 1 | sed -n 's/.*VERSION_\\([0-9_][0-9_]*\\).*/\\1/p' || true)"
  tc="$(rg -n --no-heading -o 'targetCompatibility\\s*=\\s*(?:JavaVersion\\.)?VERSION_[0-9_]+' "$file" 2>/dev/null | head -n 1 | sed -n 's/.*VERSION_\\([0-9_][0-9_]*\\).*/\\1/p' || true)"
  if [[ -z "$sc" ]]; then
    sc="$(rg -n --no-heading -o 'sourceCompatibility\\s*=\\s*\"?[0-9]+(?:\\.[0-9]+)?\"?' "$file" 2>/dev/null | head -n 1 | sed -E 's/.*=\\s*\"?([0-9]+(?:\\.[0-9]+)?)\"?.*/\\1/' || true)"
  fi
  if [[ -z "$tc" ]]; then
    tc="$(rg -n --no-heading -o 'targetCompatibility\\s*=\\s*\"?[0-9]+(?:\\.[0-9]+)?\"?' "$file" 2>/dev/null | head -n 1 | sed -E 's/.*=\\s*\"?([0-9]+(?:\\.[0-9]+)?)\"?.*/\\1/' || true)"
  fi

  [[ -n "$sc" ]] && sc="$(normalize_jvm_target "$sc")"
  [[ -n "$tc" ]] && tc="$(normalize_jvm_target "$tc")"
  [[ -n "$sc" ]] && evidence+=("$(evidence_item "$file" "sourceCompatibility" "$sc")")
  [[ -n "$tc" ]] && evidence+=("$(evidence_item "$file" "targetCompatibility" "$tc")")
  if [[ -n "$sc" && -n "$tc" && "$sc" != "$tc" ]]; then
    emit_unknown "sourceCompatibility and targetCompatibility differ"
  fi
  if [[ -n "$sc" ]]; then
    emit_result "gradle" "$sc" "${evidence[@]}"
    return 0
  fi
  if [[ -n "$tc" ]]; then
    emit_result "gradle" "$tc" "${evidence[@]}"
    return 0
  fi

  # Kotlin JVM hint: kotlinOptions.jvmTarget (NOTE: not strictly Java language level).
  local kt
  kt="$(rg -n --no-heading -o 'kotlinOptions\\.jvmTarget\\s*=\\s*(?:JavaVersion\\.)?VERSION_[0-9_]+' "$file" 2>/dev/null | head -n 1 | sed -n 's/.*VERSION_\\([0-9_][0-9_]*\\).*/\\1/p' || true)"
  if [[ -z "$kt" ]]; then
    kt="$(rg -n --no-heading -o 'kotlinOptions\\.jvmTarget\\s*=\\s*\"[^\"]+\"' "$file" 2>/dev/null | head -n 1 | sed -E 's/.*\"([^\"]+)\".*/\\1/' || true)"
  fi
  if [[ -z "$kt" ]]; then
    kt="$(rg -n --no-heading -o 'jvmTarget\\s*=\\s*(?:JavaVersion\\.)?VERSION_[0-9_]+' "$file" 2>/dev/null | head -n 1 | sed -n 's/.*VERSION_\\([0-9_][0-9_]*\\).*/\\1/p' || true)"
  fi
  if [[ -z "$kt" ]]; then
    kt="$(rg -n --no-heading -o 'jvmTarget\\s*=\\s*\"[^\"]+\"' "$file" 2>/dev/null | head -n 1 | sed -E 's/.*\"([^\"]+)\".*/\\1/' || true)"
  fi
  if [[ -n "$kt" ]]; then
    local norm
    norm="$(normalize_jvm_target "$kt")"
    evidence+=("$(evidence_item "$file" "kotlinOptions.jvmTarget" "$norm")")
    if [[ "$norm" =~ ^[0-9]+$ ]]; then
      emit_result "gradle" "$norm" "${evidence[@]}"
      return 0
    fi
  fi

  emit_unknown "no supported Gradle Java/toolchain keys found"
}

has_pom=0
has_gradle=0
[[ -f "$pom" ]] && has_pom=1
[[ -f "$gradle_kts" || -f "$gradle_groovy" ]] && has_gradle=1

if [[ $has_pom -eq 1 && $has_gradle -eq 1 ]]; then
  emit_unknown "both pom.xml and build.gradle(.kts) present; tool selection is ambiguous"
fi

if [[ $has_pom -eq 1 ]]; then
  detect_maven_release "$pom"
fi

if [[ $has_gradle -eq 1 ]]; then
  if [[ -f "$gradle_kts" ]]; then
    detect_gradle_release "$gradle_kts"
  else
    detect_gradle_release "$gradle_groovy"
  fi
fi

emit_unknown "no Maven/Gradle build files found"
