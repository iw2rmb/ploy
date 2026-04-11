#!/usr/bin/env bash
set -euo pipefail

workspace=""
outdir="/out"
args=()

usage() {
  cat <<'USAGE'
heal-orw --apply --dir <workspace> --out <outdir>

Wraps `orw-cli` and sets ORW_BUILD_SYSTEM deterministically.

Detection order:
1) ORW_BUILD_SYSTEM env when already set (`gradle|maven`)
2) workspace markers (`build.gradle(.kts)` or `pom.xml`)

On ambiguous or missing detection, writes /out/report.json and exits non-zero.
USAGE
}

json_escape() {
  printf '%s' "$1" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g' -e ':a;N;$!ba;s/\n/\\n/g'
}

write_failure_report() {
  local error_kind="$1"
  local reason="$2"
  local message="$3"
  local message_esc
  local reason_esc
  message_esc=$(json_escape "$message")
  reason_esc=$(json_escape "$reason")
  if [[ -n "$reason" ]]; then
    cat >"$outdir/report.json" <<JSON
{"success":false,"error_kind":"${error_kind}","reason":"${reason_esc}","message":"${message_esc}"}
JSON
  else
    cat >"$outdir/report.json" <<JSON
{"success":false,"error_kind":"${error_kind}","message":"${message_esc}"}
JSON
  fi
}

normalize_build_system() {
  local raw="${1:-}"
  case "$(echo "$raw" | tr '[:upper:]' '[:lower:]' | xargs)" in
    gradle) echo "gradle" ;;
    maven) echo "maven" ;;
    *) echo "" ;;
  esac
}

build_system_from_stack() {
  case "${PLOY_DETECTED_STACK:-}" in
    java-gradle) echo "gradle" ;;
    java-maven) echo "maven" ;;
    *) echo "" ;;
  esac
}

detect_build_system() {
  local from_env from_stack has_gradle has_maven
  from_env="$(normalize_build_system "${ORW_BUILD_SYSTEM:-}")"
  if [[ -n "$from_env" ]]; then
    echo "$from_env"
    return 0
  fi
  from_stack="$(build_system_from_stack)"

  has_gradle=0
  has_maven=0
  [[ -f "$workspace/build.gradle" || -f "$workspace/build.gradle.kts" ]] && has_gradle=1
  [[ -f "$workspace/pom.xml" ]] && has_maven=1

  if [[ $has_gradle -eq 1 && $has_maven -eq 1 ]]; then
    if [[ -n "$from_stack" ]]; then
      echo "$from_stack"
      return 0
    fi
    return 1
  fi
  if [[ $has_gradle -eq 1 ]]; then
    echo "gradle"
    return 0
  fi
  if [[ $has_maven -eq 1 ]]; then
    echo "maven"
    return 0
  fi
  if [[ -n "$from_stack" ]]; then
    echo "$from_stack"
    return 0
  fi
  return 2
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --dir)
      workspace="${2:-}"
      args+=("$1" "${2:-}")
      shift 2
      ;;
    --out)
      outdir="${2:-}"
      args+=("$1" "${2:-}")
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      args+=("$1")
      shift
      ;;
  esac
done

mkdir -p "$outdir"

if [[ -z "$workspace" ]]; then
  write_failure_report "input" "" "--dir <workspace> is required for heal-orw"
  exit 2
fi
if [[ ! -d "$workspace" ]]; then
  write_failure_report "input" "" "workspace directory does not exist: $workspace"
  exit 2
fi

if ! command -v orw-cli >/dev/null 2>&1; then
  write_failure_report "internal" "" "orw-cli binary not found in healing image"
  exit 127
fi

build_system=""
if build_system="$(detect_build_system)"; then
  :
else
  case $? in
    1)
      write_failure_report "input" "" "ambiguous build system (both pom.xml and build.gradle); set ORW_BUILD_SYSTEM explicitly"
      ;;
    *)
      write_failure_report "input" "" "unable to detect build system (no pom.xml/build.gradle and no stack hint)"
      ;;
  esac
  exit 4
fi

export ORW_BUILD_SYSTEM="$build_system"
exec orw-cli "${args[@]}"
