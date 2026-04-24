#!/usr/bin/env bash
set -euo pipefail

ca_installer="/usr/local/lib/ploy/install_ploy_ca_bundle.sh"
if [[ -f "$ca_installer" ]]; then
  source "$ca_installer"
else
  script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  local_ca_installer="$(cd "$script_dir/../../.." && pwd)/install_ploy_ca_bundle.sh"
  if [[ -f "$local_ca_installer" ]]; then
    source "$local_ca_installer"
  else
    install_ploy_ca_bundle() { return 0; }
  fi
fi

usage() {
  cat <<'USAGE'
orw-cli --apply --dir <workspace> --out <outdir>

Required env:
  - Class-only mode (no rewrite config): RECIPE_GROUP, RECIPE_ARTIFACT, RECIPE_CLASSNAME
  - YAML mode (/out/rewrite.yml or ORW_CONFIG_PATH): recipe coords default automatically

Optional env:
  RECIPE_VERSION              Optional recipe artifact version (auto-resolved when unset)
  ORW_REPOS                  Comma-separated Maven repo URLs
  ORW_REPO_USERNAME          Repo username (must pair with ORW_REPO_PASSWORD)
  ORW_REPO_PASSWORD          Repo password (must pair with ORW_REPO_USERNAME)
  ORW_CONFIG_PATH            Optional path to rewrite YAML config (defaults: /out/rewrite.yml)
  ORW_ACTIVE_RECIPES         Comma-separated active recipe overrides
  ORW_FAIL_ON_UNSUPPORTED    true|false (default: true)
  ORW_EXCLUDE_PATHS          Comma-separated glob patterns excluded from parsing (e.g. **/*.proto)
  ORW_AUTO_EXCLUDE_GROOVY_PARSE_FAILURES
                             true|false (default: false); retry once after Groovy parse failures by auto-excluding failed files
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

orw_lib_path="/usr/local/lib/ploy/orw-lib.sh"
if [[ -f "$orw_lib_path" ]]; then
  source "$orw_lib_path"
else
  orw_script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  local_orw_lib="$(cd "$orw_script_dir/.." && pwd)/shared/orw-lib.sh"
  if [[ -f "$local_orw_lib" ]]; then
    source "$local_orw_lib"
  else
    echo "error: orw-lib.sh not found at $orw_lib_path or $local_orw_lib" >&2
    exit 1
  fi
fi

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

auto_exclude_groovy_parse_failures=false
if parse_bool_default_false "${ORW_AUTO_EXCLUDE_GROOVY_PARSE_FAILURES:-}"; then
  auto_exclude_groovy_parse_failures=true
else
  rc=$?
  if [[ $rc -eq 2 ]]; then
    write_failure_report "input" "" "ORW_AUTO_EXCLUDE_GROOVY_PARSE_FAILURES must be true/false"
    exit 4
  fi
fi

if [[ -n "${ORW_EXCLUDES:-}" || -n "${ORW_INCLUDES:-}" ]]; then
  echo "[orw-cli] Warning: ORW_EXCLUDES/ORW_INCLUDES are unsupported; use ORW_EXCLUDE_PATHS." | tee -a "$transform_log"
fi
export ORW_EXCLUDE_PATHS="${ORW_EXCLUDE_PATHS:-}"
proto_prescan_patterns="$(collect_preflight_proto_exclude_patterns "$workspace")"
proto_added_patterns="$(new_patterns_from_candidates "${ORW_EXCLUDE_PATHS:-}" "$proto_prescan_patterns")"
if [[ -n "$proto_added_patterns" ]]; then
  proto_added_csv="$(lines_to_csv "$proto_added_patterns")"
  merged_excludes="$(merge_exclude_patterns "${ORW_EXCLUDE_PATHS:-}" "$proto_added_patterns")"
  echo "[orw-cli] Proto pre-scan auto-exclude candidates: ${proto_added_csv}" | tee -a "$transform_log"
  echo "[orw-cli] Proto pre-scan merged ORW_EXCLUDE_PATHS=${merged_excludes}" | tee -a "$transform_log"
  export ORW_EXCLUDE_PATHS="$merged_excludes"
elif [[ -n "$proto_prescan_patterns" ]]; then
  echo "[orw-cli] Proto pre-scan found proto3/edition paths but all are already present in ORW_EXCLUDE_PATHS" | tee -a "$transform_log"
else
  echo "[orw-cli] Proto pre-scan found no proto3/edition files to exclude" | tee -a "$transform_log"
fi

config_path="${ORW_CONFIG_PATH:-}"
if [[ -z "$config_path" ]]; then
  if [[ -f "$outdir/rewrite.yml" ]]; then
    config_path="$outdir/rewrite.yml"
  elif [[ -f "/out/rewrite.yml" ]]; then
    config_path="/out/rewrite.yml"
  fi
fi
if [[ -n "$config_path" && ! -f "$config_path" ]]; then
  write_failure_report "input" "" "ORW_CONFIG_PATH does not exist: $config_path"
  exit 4
fi

# YAML mode defaults: avoid per-run boilerplate for generated rewrite.yml recipes.
used_yaml_defaults=false
if [[ -n "$config_path" ]]; then
  if [[ -z "$group" ]]; then
    group="org.openrewrite"
    used_yaml_defaults=true
  fi
  if [[ -z "$artifact" ]]; then
    artifact="rewrite-java"
    used_yaml_defaults=true
  fi
  if [[ -z "$classname" ]]; then
    classname="org.openrewrite.java.ChangeMethodName"
    used_yaml_defaults=true
  fi
fi

if [[ -z "$group" || -z "$artifact" || -z "$classname" ]]; then
  write_failure_report "input" "" "RECIPE_GROUP/RECIPE_ARTIFACT/RECIPE_CLASSNAME are required (unless YAML mode with /out/rewrite.yml defaults)"
  exit 4
fi

active_recipes="${ORW_ACTIVE_RECIPES:-}"
if [[ -z "$active_recipes" && -n "$config_path" ]]; then
  active_recipes="$(awk '/^name:[[:space:]]*/{print $2; exit}' "$config_path" || true)"
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

if [[ -n "$version" ]]; then
  coords="${group}:${artifact}:${version}"
else
  coords="${group}:${artifact}"
fi
classpath_file="/share/java.classpath"
args=(--apply --dir "$workspace" --recipe "$active_recipes" --coords "$coords" --classpath-file "$classpath_file")
if [[ -n "$config_path" ]]; then
  args+=(--config "$config_path")
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
echo "[orw-cli] Java classpath file: $classpath_file" | tee -a "$transform_log"
if [[ "$used_yaml_defaults" == "true" ]]; then
  echo "[orw-cli] Applied YAML-mode default recipe coordinates/classname" | tee -a "$transform_log"
fi
if [[ -z "${RECIPE_VERSION:-}" ]]; then
  echo "[orw-cli] RECIPE_VERSION is unset; resolving latest available version from repositories" | tee -a "$transform_log"
fi

status=0
run_rewrite_cli || status=$?

if [[ $status -ne 0 && "$auto_exclude_groovy_parse_failures" == "true" ]]; then
  if has_groovy_parse_failure "$transform_log"; then
    candidate_patterns="$(build_groovy_parse_exclude_patterns "$transform_log")"
    added_patterns="$(new_patterns_from_candidates "${ORW_EXCLUDE_PATHS:-}" "$candidate_patterns")"
    if [[ -n "$added_patterns" ]]; then
      added_csv="$(lines_to_csv "$added_patterns")"
      merged_excludes="$(merge_exclude_patterns "${ORW_EXCLUDE_PATHS:-}" "$added_patterns")"
      echo "[orw-cli] Auto-exclude candidates detected: ${added_csv}" | tee -a "$transform_log"
      echo "[orw-cli] Retrying OpenRewrite CLI with updated ORW_EXCLUDE_PATHS=${merged_excludes}" | tee -a "$transform_log"
      echo "[orw-cli] Auto-exclude applied paths: ${added_csv}" | tee -a "$transform_log"
      export ORW_EXCLUDE_PATHS="$merged_excludes"
      status=0
      run_rewrite_cli || status=$?
    else
      echo "[orw-cli] Auto-exclude enabled but no new exclude paths were derived from Groovy parse failures" | tee -a "$transform_log"
    fi
  fi
fi

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
