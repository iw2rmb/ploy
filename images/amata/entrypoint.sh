#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<USAGE
amata [run /in/amata.yaml [--set key=value ...]]

Environment:
  CODEX_HOME        Codex home directory for auth/config files.
  CODEX_MODEL       Optional model override (written into manifest metadata).

File delivery (Hydra):
  Config files (auth.json, config.toml, crush.json, ccr config.json) are delivered
  via Hydra home mounts to their expected paths under \$HOME.

Behavior:
  - Always executes the amata binary.
  - If invoked without args and /in/amata.yaml exists, runs: amata run /in/amata.yaml
  - Imports extra CA certs from /etc/ploy/ca when present.
  - Writes codex-compatible artifacts to /out: codex.log, codex-last.txt, codex-run.json.
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

install_ploy_ca_bundle() {
  local ca_dir="/etc/ploy/ca"
  if [[ ! -d "$ca_dir" ]]; then
    return 0
  fi

  local tmp_dir
  tmp_dir="$(mktemp -d)"
  local cert_count=0
  local file
  for file in "$ca_dir"/*; do
    [[ -f "$file" ]] || continue
    local before
    before=$(find "$tmp_dir" -maxdepth 1 -name 'cert*.crt' 2>/dev/null | wc -l | tr -d ' ')
    # Split potential bundle files into individual certs.
    awk '/-----BEGIN CERTIFICATE-----/{n++} {print > (d"/cert" n ".crt")}' d="$tmp_dir" "$file"
    local after
    after=$(find "$tmp_dir" -maxdepth 1 -name 'cert*.crt' 2>/dev/null | wc -l | tr -d ' ')
    if [[ "$after" -gt "$before" ]]; then
      cert_count=$((cert_count + (after - before)))
    fi
  done

  if [[ "$cert_count" -eq 0 ]]; then
    rm -rf "$tmp_dir"
    return 0
  fi

  if command -v update-ca-certificates >/dev/null 2>&1; then
    mkdir -p /usr/local/share/ca-certificates/ploy
    for file in "$tmp_dir"/*.crt; do
      [[ -f "$file" ]] || continue
      cp "$file" /usr/local/share/ca-certificates/ploy/ || true
    done
    update-ca-certificates >/dev/null 2>&1 || true
  fi

  # Fallback for runtimes that honor explicit CA bundle env vars.
  local first_ca
  first_ca="$(ls "$tmp_dir"/*.crt 2>/dev/null | head -1 || true)"
  if [[ -n "$first_ca" ]]; then
    export SSL_CERT_FILE="$first_ca"
    export CURL_CA_BUNDLE="$first_ca"
    export GIT_SSL_CAINFO="$first_ca"
  fi

  rm -rf "$tmp_dir"
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
install_ploy_ca_bundle
activate_ccr_if_configured

logfile="$out_dir/codex.log"
manifest_file="$out_dir/codex-run.json"

echo "[amata] starting amata run" | tee "$logfile" >&2
set +e
amata "$@" 2>&1 | tee -a "$logfile" >&2
status=${PIPESTATUS[0]}
set -e

if [[ ! -s "$logfile" ]]; then
  echo "[amata] no output captured from amata" | tee -a "$logfile" >&2
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
