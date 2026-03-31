#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<USAGE
mig-amata [run /in/amata.yaml [--set key=value ...]]

Environment:
  CODEX_HOME        Codex home directory for auth/config files.
  CODEX_MODEL       Optional model override (written into manifest metadata).
  CODEX_AUTH_JSON   Inline JSON or file path for auth; if set, written to \$CODEX_HOME/auth.json.
  CODEX_CONFIG_TOML Inline TOML for config; if set, written to \$CODEX_HOME/config.toml.
  CRUSH_JSON        Inline JSON or file path for Crush config; if set, written to ~/.config/crush/crush.json.
  CCR_CONFIG_JSON   Inline JSON or file path for Claude Code Router config; if set, written to ~/.claude-code-router/config.json.

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

materialize_env_value_or_file() {
  local env_name="$1"
  local target_path="$2"
  local value="${!env_name:-}"
  if [[ -z "$value" ]]; then
    return 0
  fi

  mkdir -p "$(dirname "$target_path")"
  if [[ -f "$value" && -r "$value" ]]; then
    install -m 600 "$value" "$target_path"
    return 0
  fi

  (
    umask 077
    printf "%s" "$value" > "$target_path"
  )
}

materialize_env_configs() {
  mkdir -p "$codex_config_dir"
  materialize_env_value_or_file CODEX_AUTH_JSON "$codex_config_dir/auth.json"
  materialize_env_value_or_file CODEX_CONFIG_TOML "$codex_config_dir/config.toml"
  materialize_env_value_or_file CRUSH_JSON "$crush_config_file"
  materialize_env_value_or_file CCR_CONFIG_JSON "$ccr_config_file"
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
materialize_env_configs
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
