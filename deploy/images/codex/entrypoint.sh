#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<USAGE
mig-codex [--input <dir>] [--out <dir>] [--auth <auth.json>] [--config <config.toml>] [--model <name>] [--prompt-file <file>]

Environment:
  CODEX_HOME        Codex home directory for auth/config files.
  CODEX_PROMPT      Inline prompt text (required in direct codex mode).
  CODEX_MODEL       Optional model override.
  CODEX_API_KEY     API key for Codex/OpenAI; passed through to codex exec.
  CODEX_AUTH_JSON   Inline JSON or file path for auth; if set, written to \$CODEX_HOME/auth.json.
  CODEX_CONFIG_TOML Inline TOML for config; if set, written to \$CODEX_HOME/config.toml.
  CRUSH_JSON        Inline JSON or file path for Crush config; if set, written to ~/.config/crush/crush.json.
  CCR_CONFIG_JSON   Inline JSON or file path for Claude Code Router config; if set, written to ~/.claude-code-router/config.json.
  CODEX_RESUME      If set to "1" and /in/codex-session.txt exists, resume the prior session.
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

materialize_env_configs
activate_ccr_if_configured

input_dir="${WORKSPACE:-/workspace}"
out_dir="${OUTDIR:-/out}"
auth_file=""
config_file=""
model="${CODEX_MODEL:-}"
prompt_file=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --input) input_dir="$2"; shift 2 ;;
    --out) out_dir="$2"; shift 2 ;;
    --auth) auth_file="$2"; shift 2 ;;
    --config) config_file="$2"; shift 2 ;;
    --model) model="$2"; shift 2 ;;
    --prompt-file) prompt_file="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown arg: $1" >&2; usage >&2; exit 1 ;;
  esac
done

mkdir -p "$out_dir" "$codex_config_dir"

if [[ -n "$auth_file" ]]; then
  install -m 600 "$auth_file" "$codex_config_dir/auth.json"
fi
if [[ -n "$config_file" ]]; then
  install -m 600 "$config_file" "$codex_config_dir/config.toml"
fi
materialize_env_configs

prompt=""
if [[ -n "$prompt_file" ]]; then
  prompt="$(cat "$prompt_file")"
elif [[ -n "${CODEX_PROMPT:-}" ]]; then
  prompt="$CODEX_PROMPT"
else
  echo "ERROR: prompt required (use --prompt-file or CODEX_PROMPT)" >&2
  exit 2
fi

resume_session=""
if [[ "${CODEX_RESUME:-}" == "1" && -f "/in/codex-session.txt" ]]; then
  resume_session="$(tr -d '\r\n' < /in/codex-session.txt)"
fi
if [[ -n "$resume_session" ]]; then
  resume_prefix="The previous Build Gate still failed (see /in/build-gate.log for the latest failure). Continue healing from the existing context by editing files under /workspace as needed to fix the error, then stop when you are done. Do not run build tools or tests inside this container; validation runs externally."
  prompt="$resume_prefix"$'\n\n'"$prompt"
fi

cmd=(codex exec)
help_out="$(codex exec --help 2>&1 || true)"
if grep -q -- "--yolo" <<<"$help_out"; then
  cmd+=(--yolo)
elif grep -q -- "--dangerously-bypass-approvals-and-sandbox" <<<"$help_out"; then
  cmd+=(--dangerously-bypass-approvals-and-sandbox)
fi

supports_add_dir=false
if grep -q -- "--add-dir" <<<"$help_out"; then
  supports_add_dir=true
fi
if [[ "$supports_add_dir" == true ]]; then
  cmd+=(--add-dir "$input_dir")
else
  echo "[mig-codex] codex exec does not support --add-dir; proceeding without explicit repo mount" >> "$out_dir/codex.log" 2>/dev/null || true
fi
if [[ -n "$model" ]]; then
  cmd+=(--model "$model")
fi
if [[ "$supports_add_dir" == true && -d "/in" ]]; then
  cmd+=(--add-dir "/in")
fi
if grep -q -- "--json" <<<"$help_out"; then
  cmd+=(--json)
fi
if grep -q -- "--output-last-message" <<<"$help_out"; then
  cmd+=(--output-last-message "$out_dir/codex-last.txt")
fi
if grep -q -- "--output-dir" <<<"$help_out"; then
  cmd+=(--output-dir "$out_dir/codex-transcript")
fi
if [[ -n "$resume_session" ]]; then
  cmd+=(resume "$resume_session")
fi
cmd+=( - )

logfile="$out_dir/codex.log"
manifest="$out_dir/codex-run.json"
jsonl="$out_dir/codex-events.jsonl"

echo "[mig-codex] starting codex exec with repo context" | tee "$logfile" >&2
if [[ -n "$resume_session" ]]; then
  echo "[mig-codex] resume mode enabled; session=$resume_session" | tee -a "$logfile" >&2
fi

set +e
printf "%s" "$prompt" | "${cmd[@]}" 2>&1 | tee -a "$logfile" | tee "$jsonl" >&2
status=${PIPESTATUS[1]}
set -e

if [[ ! -s "$logfile" ]]; then
  echo "[mig-codex] no output captured from codex" | tee -a "$logfile" >&2
fi

session_id=""
if command -v jq >/dev/null 2>&1 && [[ -s "$jsonl" ]]; then
  session_id="$(jq -r 'select(.type=="thread.started") | .thread_id // empty' "$jsonl" 2>/dev/null | head -1 || true)"
fi
if [[ -n "$session_id" ]]; then
  printf "%s\n" "$session_id" > "$out_dir/codex-session.txt"
fi

if [[ ! -s "$out_dir/codex-last.txt" ]]; then
  last_msg=""
  if command -v jq >/dev/null 2>&1 && [[ -s "$jsonl" ]]; then
    last_msg="$(jq -r 'select(.type=="message" and .role=="assistant") | .content // empty' "$jsonl" 2>/dev/null | tail -1 || true)"
  fi
  if [[ -z "$last_msg" && -s "$logfile" ]]; then
    last_msg="$(grep -v '^\s*$' "$logfile" | tail -1 || true)"
  fi
  if [[ -n "$last_msg" ]]; then
    printf "%s\n" "$last_msg" > "$out_dir/codex-last.txt"
  else
    touch "$out_dir/codex-last.txt"
  fi
fi

ts=$(date -u +%Y-%m-%dT%H:%M:%SZ)
was_resumed=false
if [[ -n "$resume_session" ]]; then
  was_resumed=true
fi
printf '{"ts":"%s","exit_code":%s,"model":"%s","input":"%s","session_id":"%s","resumed":%s}\n' \
  "$ts" "${status:-0}" "$model" "$input_dir" "${session_id}" "${was_resumed}" > "$manifest"

exit "${status:-0}"
