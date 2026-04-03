#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<USAGE
mig-amata [run /in/amata.yaml [--set key=value ...]]

Environment:
  CODEX_HOME        Codex home directory for auth/config files.
  CODEX_MODEL       Optional model override (written into manifest metadata).

File delivery (Hydra):
  Config files (auth.json, config.toml, crush.json, ccr config.json) are delivered
  via Hydra home mounts to their expected paths under \$HOME.

Behavior:
  - Always executes the amata binary.
  - If invoked without args and /in/amata.yaml exists, runs: amata run /in/amata.yaml
  - Writes codex-compatible artifacts to /out: codex.log, codex-last.txt, codex-run.json.
USAGE
}

home_dir="${HOME:-/root}"
codex_config_dir="${CODEX_HOME:-$home_dir/.codex}"
export CODEX_HOME="$codex_config_dir"
crush_config_file="$home_dir/.config/crush/crush.json"
ccr_config_file="$home_dir/.claude-code-router/config.json"

# Hydra file delivery: config files are materialized by Hydra home mounts at
# their expected paths. Legacy env-based inline materialization is removed.
# If legacy env vars are still set, emit a warning to assist migration.
warn_legacy_env() {
  local env_name="$1"
  local target_path="$2"
  local value="${!env_name:-}"
  if [[ -n "$value" && ! -f "$target_path" ]]; then
    echo "warning: $env_name is set but $target_path was not materialized by Hydra; migrate to typed home field" >&2
  fi
}

check_hydra_configs() {
  mkdir -p "$codex_config_dir"
  warn_legacy_env CODEX_AUTH_JSON "$codex_config_dir/auth.json"
  warn_legacy_env CODEX_CONFIG_TOML "$codex_config_dir/config.toml"
  warn_legacy_env CRUSH_JSON "$crush_config_file"
  warn_legacy_env CCR_CONFIG_JSON "$ccr_config_file"
}

activate_ccr_if_configured() {
  if [[ -f "$ccr_config_file" ]]; then
    ccr start
    eval "$(ccr activate)"
  fi
}

case "${1:-}" in
  -h|--help)
    usage
    exit 0
    ;;
esac

if [[ $# -eq 0 && -s "/in/amata.yaml" ]]; then
  set -- run /in/amata.yaml
fi

out_dir="${OUTDIR:-/out}"
model="${CODEX_MODEL:-}"
mkdir -p "$out_dir" "$codex_config_dir"
check_hydra_configs
activate_ccr_if_configured

logfile="$out_dir/codex.log"
manifest_file="$out_dir/codex-run.json"

echo "[mig-amata] starting amata run" | tee "$logfile" >&2
set +e
amata "$@" 2>&1 | tee -a "$logfile" >&2
status=${PIPESTATUS[0]}
set -e

if [[ ! -s "$logfile" ]]; then
  echo "[mig-amata] no output captured from amata" | tee -a "$logfile" >&2
fi
if [[ ! -s "$out_dir/codex-last.txt" ]]; then
  if [[ -s "$logfile" ]]; then
    grep -v '^\s*$' "$logfile" | tail -1 > "$out_dir/codex-last.txt" || true
  fi
  [[ -s "$out_dir/codex-last.txt" ]] || touch "$out_dir/codex-last.txt"
fi

ts=$(date -u +%Y-%m-%dT%H:%M:%SZ)
printf '{"ts":"%s","exit_code":%s,"model":"%s","input":"%s","session_id":"%s","resumed":%s}\n' \
  "$ts" "${status:-0}" "${model}" "${WORKSPACE:-/workspace}" "" "false" > "$manifest_file"

exit "${status:-0}"
