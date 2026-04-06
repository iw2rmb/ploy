#!/usr/bin/env bash
set -euo pipefail
source /usr/local/lib/ploy/install_ploy_ca_bundle.sh

usage() {
  cat <<'USAGE'
orw-cli --apply --dir <workspace> --out <outdir>

Required env:
  RECIPE_GROUP
  RECIPE_ARTIFACT
  RECIPE_VERSION
  RECIPE_CLASSNAME

Optional env:
  ORW_REPOS                  Comma-separated Maven repo URLs
  ORW_REPO_USERNAME          Repo username (must pair with ORW_REPO_PASSWORD)
  ORW_REPO_PASSWORD          Repo password (must pair with ORW_REPO_USERNAME)
  ORW_ACTIVE_RECIPES         Comma-separated active recipe overrides
  ORW_FAIL_ON_UNSUPPORTED    true|false (default: true)
  ORW_EXCLUDE_PATHS          Comma-separated glob patterns excluded from parsing (e.g. **/*.proto)
  ORW_CLI_BIN                Executable name/path for OpenRewrite CLI (default: rewrite)
  PLOY_CA_CERT_PATH          PEM CA bundle file path imported into trust stores (Hydra mount: /etc/ploy/certs/ca.crt)
USAGE
}

workspace=""
outdir="/out"
action=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --apply)
      action="apply"
      shift
      ;;
    --dir)
      workspace="${2:-}"
      shift 2
      ;;
    --out)
      outdir="${2:-}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "error: unknown arg: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

mkdir -p "$outdir"
transform_log="$outdir/transform.log"
: >"$transform_log"

json_escape() {
  printf '%s' "$1" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g' -e ':a;N;$!ba;s/\n/\\n/g'
}

write_success_report() {
  local message="$1"
  local msg_esc
  msg_esc=$(json_escape "$message")
  cat >"$outdir/report.json" <<JSON
{"success":true,"message":"${msg_esc}"}
JSON
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

parse_bool_default_true() {
  local raw="${1:-}"
  local norm
  norm="$(echo "$raw" | tr '[:upper:]' '[:lower:]' | xargs)"
  case "$norm" in
    ""|1|true|yes|on)
      return 0
      ;;
    0|false|no|off)
      return 1
      ;;
    *)
      echo "invalid boolean value: ${raw}" >&2
      return 2
      ;;
  esac
}

if [[ "${MIGS_SELF_TEST:-}" == "1" ]]; then
  write_success_report "orw-cli self-test passed"
  exit 0
fi

if [[ "$action" != "apply" ]]; then
  write_failure_report "input" "" "action flag required: --apply"
  usage >&2
  exit 2
fi

if [[ -z "$workspace" ]]; then
  write_failure_report "input" "" "--dir <workspace> is required"
  usage >&2
  exit 2
fi

if [[ ! -d "$workspace" ]]; then
  write_failure_report "input" "" "workspace directory does not exist: $workspace"
  exit 2
fi

group="${RECIPE_GROUP:-}"
artifact="${RECIPE_ARTIFACT:-}"
version="${RECIPE_VERSION:-}"
classname="${RECIPE_CLASSNAME:-}"

if [[ -z "$group" || -z "$artifact" || -z "$version" || -z "$classname" ]]; then
  write_failure_report "input" "" "RECIPE_GROUP/RECIPE_ARTIFACT/RECIPE_VERSION/RECIPE_CLASSNAME are required"
  exit 4
fi

repo_username="${ORW_REPO_USERNAME:-}"
repo_password="${ORW_REPO_PASSWORD:-}"
if [[ -n "$repo_username" && -z "$repo_password" ]] || [[ -z "$repo_username" && -n "$repo_password" ]]; then
  write_failure_report "input" "" "ORW_REPO_USERNAME and ORW_REPO_PASSWORD must be provided together"
  exit 4
fi

PLOY_CA_IMPORT_JAVA=1 PLOY_CA_LOG_FILE="$transform_log" install_ploy_ca_bundle

fail_on_unsupported=true
if ! parse_bool_default_true "${ORW_FAIL_ON_UNSUPPORTED:-}"; then
  rc=$?
  if [[ $rc -eq 2 ]]; then
    write_failure_report "input" "" "ORW_FAIL_ON_UNSUPPORTED must be true/false"
    exit 4
  fi
  fail_on_unsupported=false
fi

active_recipes="${ORW_ACTIVE_RECIPES:-}"
if [[ -z "$active_recipes" ]]; then
  if [[ -f "$workspace/rewrite.yml" ]]; then
    active_recipes="$(awk '/^name:[[:space:]]*/{print $2; exit}' "$workspace/rewrite.yml" || true)"
  fi
fi
if [[ -z "$active_recipes" ]]; then
  active_recipes="$classname"
fi

cli_bin="${ORW_CLI_BIN:-rewrite}"
cli_name="$(basename "$cli_bin")"
case "$cli_name" in
  gradle|gradlew|mvn|mvnw)
    write_failure_report "input" "" "ORW_CLI_BIN must not be a build tool command"
    exit 4
    ;;
esac

if ! command -v "$cli_bin" >/dev/null 2>&1; then
  write_failure_report "internal" "" "OpenRewrite CLI binary not found: $cli_bin"
  exit 127
fi

coords="${group}:${artifact}:${version}"
args=(--apply --dir "$workspace" --recipe "$active_recipes" --coords "$coords")
if [[ -f "$workspace/rewrite.yml" ]]; then
  args+=(--config "$workspace/rewrite.yml")
fi
if [[ -n "${ORW_REPOS:-}" ]]; then
  IFS=',' read -r -a repo_list <<<"${ORW_REPOS}"
  for repo in "${repo_list[@]}"; do
    repo="$(echo "$repo" | xargs)"
    if [[ -n "$repo" ]]; then
      args+=(--repo "$repo")
    fi
  done
fi
if [[ -n "$repo_username" ]]; then
  args+=(--repo-username "$repo_username" --repo-password "$repo_password")
fi

echo "[orw-cli] Running OpenRewrite CLI" | tee -a "$transform_log"
echo "[orw-cli] Coords: $coords" | tee -a "$transform_log"
echo "[orw-cli] Active recipes: $active_recipes" | tee -a "$transform_log"

status=0
"$cli_bin" "${args[@]}" 2>&1 | tee -a "$transform_log" || status=$?

if [[ $status -eq 0 ]]; then
  write_success_report "OpenRewrite CLI apply completed"
  exit 0
fi

error_kind="execution"
reason=""
message="OpenRewrite CLI failed with exit ${status}"
if grep -Eiq 'type-attribution-unavailable|type attribution unavailable' "$transform_log"; then
  error_kind="unsupported"
  reason="type-attribution-unavailable"
  message="Type attribution is unavailable for this repository"
fi

if [[ "$error_kind" == "unsupported" && "$fail_on_unsupported" == "false" ]]; then
  write_success_report "Type attribution is unavailable but ORW_FAIL_ON_UNSUPPORTED=false"
  exit 0
fi

write_failure_report "$error_kind" "$reason" "$message"
exit "$status"
