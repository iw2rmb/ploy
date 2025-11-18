#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<USAGE
mod-codex [--input <dir>] [--out <dir>] [--auth <auth.json>] [--model <name>] [--prompt-file <file>]

Environment:
  CODEX_PROMPT     Inline prompt text (used when --prompt-file not provided).
  CODEX_MODEL      Optional model override (e.g., o4-mini, gpt-4.1-mini, etc.).
  CODEX_AUTH_JSON  Inline JSON for auth; if set, written to ~/.codex/auth.json.

Behavior:
  - Installs Codex CLI in the image (via npm @openai/codex).
  - Places auth at /root/.codex/auth.json when --auth is given or CODEX_AUTH_JSON is set.
  - Always adds repository directory with: codex exec --add-dir <input> ...
  - Writes logs to <out>/codex.log and a small run manifest to <out>/codex-run.json.
USAGE
}

input_dir="${WORKSPACE:-/workspace}"
out_dir="${OUTDIR:-/out}"
auth_file=""
model="${CODEX_MODEL:-}"
prompt_file=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --input) input_dir="$2"; shift 2 ;;
    --out) out_dir="$2"; shift 2 ;;
    --auth) auth_file="$2"; shift 2 ;;
    --model) model="$2"; shift 2 ;;
    --prompt-file) prompt_file="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown arg: $1" >&2; usage >&2; exit 1 ;;
  esac
done

mkdir -p "$out_dir" /root/.codex

# Auth via file path
if [[ -n "$auth_file" ]]; then
  install -m 600 "$auth_file" /root/.codex/auth.json
fi

# Auth via env content
if [[ -n "${CODEX_AUTH_JSON:-}" ]]; then
  umask 077
  printf "%s" "$CODEX_AUTH_JSON" > /root/.codex/auth.json
fi

# Resolve prompt
prompt=""
if [[ -n "$prompt_file" ]]; then
  prompt="$(cat "$prompt_file")"
elif [[ -n "${CODEX_PROMPT:-}" ]]; then
  prompt="$CODEX_PROMPT"
else
  echo "ERROR: prompt required (use --prompt-file or CODEX_PROMPT)" >&2
  exit 2
fi

# Build Codex exec command
# Build base codex exec command; detect CLI capabilities
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
  echo "[mod-codex] codex exec does not support --add-dir; proceeding without explicit repo mount" >> "$out_dir/codex.log" 2>/dev/null || true
fi
if [[ -n "$model" ]]; then
  cmd+=(--model "$model")
fi

# Always include /in as context when the directory exists
if [[ "$supports_add_dir" == true && -d "/in" ]]; then
  cmd+=(--add-dir "/in")
fi
cmd+=( - )

# Run Codex; pipe prompt via stdin; capture both stdout and stderr
logfile="$out_dir/codex.log"
manifest="$out_dir/codex-run.json"
echo "[mod-codex] starting codex exec with repo context" > "$logfile"
set +e
printf "%s" "$prompt" | "${cmd[@]}" 2>&1 | tee -a "$logfile"
status=${PIPESTATUS[1]}
set -e
if [[ ! -s "$logfile" ]]; then
  echo "[mod-codex] no output captured from codex" >> "$logfile"
fi

# Minimal manifest for downstream debugging
ts=$(date -u +%Y-%m-%dT%H:%M:%SZ)
printf '{"ts":"%s","exit_code":%s,"model":"%s","input":"%s"}\n' "$ts" "${status:-0}" "${model}" "$input_dir" > "$manifest"

exit "${status:-0}"

