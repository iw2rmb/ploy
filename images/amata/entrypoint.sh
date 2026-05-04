#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<USAGE
amata [run /in/amata.yaml [--set key=value ...]]

Environment:
  CODEX_HOME        Codex home directory for auth/config files.

File delivery (Hydra):
  Config files (auth.json, config.toml, crush.json, ccr config.json) are delivered
  via Hydra home mounts to their expected paths under \$HOME.

Behavior:
  - Always executes the amata binary.
  - If invoked without args and /in/amata.yaml exists, runs: amata run /in/amata.yaml
USAGE
}

home_dir="${HOME:-/root}"
codex_config_dir="${CODEX_HOME:-$home_dir/.codex}"
export CODEX_HOME="$codex_config_dir"
crush_config_file="$home_dir/.config/crush/crush.json"
ccr_config_file="$home_dir/.claude-code-router/config.json"

# Hydra file delivery: config files are materialized by Hydra home mounts at
# their expected paths. No env-based inline materialization is supported.
check_hydra_configs() {
  mkdir -p "$codex_config_dir"
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
mkdir -p "$out_dir" "$codex_config_dir"
check_hydra_configs
activate_ccr_if_configured

echo "[amata] starting amata run" >&2
set +e
amata --out jsonl "$@"
status=$?
set -e

exit "${status:-0}"
