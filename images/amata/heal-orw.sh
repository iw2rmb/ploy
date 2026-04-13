#!/usr/bin/env bash
set -euo pipefail

workspace=""
outdir="/out"
args=()

usage() {
  cat <<'USAGE'
heal-orw --apply --dir <workspace> --out <outdir>

Wraps `orw-cli` and enforces canonical stack tuple env inputs.

Required stack env:
  - PLOY_STACK_LANGUAGE
  - PLOY_STACK_TOOL
  - PLOY_STACK_RELEASE

Supported build tools:
  - maven
  - gradle

On invalid or missing stack tuple, writes /out/report.json and exits non-zero.
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

detect_build_system() {
  local language tool release normalized_tool normalized_language
  language="$(echo "${PLOY_STACK_LANGUAGE:-}" | tr '[:upper:]' '[:lower:]' | xargs)"
  tool="$(echo "${PLOY_STACK_TOOL:-}" | tr '[:upper:]' '[:lower:]' | xargs)"
  release="$(echo "${PLOY_STACK_RELEASE:-}" | xargs)"
  normalized_tool="$(normalize_build_system "$tool")"
  normalized_language="$language"

  if [[ -z "$normalized_language" || -z "$normalized_tool" || -z "$release" ]]; then
    return 1
  fi
  if [[ "$normalized_language" != "java" ]]; then
    return 2
  fi
  echo "$normalized_tool"
  return 0
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
      write_failure_report "input" "" "missing canonical stack tuple env (require PLOY_STACK_LANGUAGE, PLOY_STACK_TOOL, PLOY_STACK_RELEASE)"
      ;;
    *)
      write_failure_report "input" "" "unsupported stack tuple for OpenRewrite (require java+maven or java+gradle)"
      ;;
  esac
  exit 4
fi

exec orw-cli "${args[@]}"
