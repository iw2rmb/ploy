#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<USAGE
mig-codex [--input <dir>] [--out <dir>] [--auth <auth.json>] [--config <config.toml>] [--model <name>] [--prompt-file <file>]
mig-codex amata run /in/amata.yaml [--set <param>=<value> ...]

Environment:
  CODEX_PROMPT      Inline prompt text (required in direct codex mode; optional in amata mode).
  CODEX_MODEL       Optional model override (e.g., o4-mini, gpt-4.1-mini, etc.).
  CODEX_AUTH_JSON   Inline JSON for auth; if set, written to ~/.codex/auth.json.
  CODEX_CONFIG_TOML Inline TOML for config; if set, written to ~/.codex/config.toml.
  CODEX_RESUME      If set to "1" and /in/codex-session.txt exists, resume the prior
                    Codex session instead of starting fresh (for healing retries).
  PLOY_API_TOKEN    Optional bearer token for Build Gate API (unused; gate runs externally).

Behavior:
  - Amata mode (first arg "amata"): delegates to amata binary; CODEX_PROMPT not required.
  - Direct codex mode (default): uses codex exec; CODEX_PROMPT or --prompt-file required.
  - Places auth at /root/.codex/auth.json when --auth is given or CODEX_AUTH_JSON is set.
  - Places config at /root/.codex/config.toml when --config is given or CODEX_CONFIG_TOML is set.
  - Always adds repository directory with: codex exec --add-dir <input> ...
  - Writes logs to <out>/codex.log and a small run manifest to <out>/codex-run.json.
  - When CODEX_RESUME=1 and a prior session exists, uses "codex exec resume <session>"
    to continue in the same thread, preserving context across healing attempts.
USAGE
}

# ─── Amata mode: "mig-codex amata run /in/amata.yaml [--set param=val ...]" ──
# Invoked when the manifest command is amata-routed (amata.spec is present).
# Auth credentials are still materialized from env vars. CODEX_PROMPT is not required.
# Artifact contract is kept: codex.log, codex-last.txt, codex-run.json written to /out.
if [[ "${1:-}" == "amata" ]]; then
    shift  # drop "amata"; forward remaining args verbatim to amata binary

    out_dir="${OUTDIR:-/out}"
    model="${CODEX_MODEL:-}"
    mkdir -p "$out_dir" /root/.codex

    if [[ -n "${CODEX_AUTH_JSON:-}" ]]; then
        umask 077
        printf "%s" "$CODEX_AUTH_JSON" > /root/.codex/auth.json
    fi
    if [[ -n "${CODEX_CONFIG_TOML:-}" ]]; then
        umask 077
        printf "%s" "$CODEX_CONFIG_TOML" > /root/.codex/config.toml
    fi

    logfile="$out_dir/codex.log"
    manifest_file="$out_dir/codex-run.json"
    echo "[mig-codex] starting amata run" | tee "$logfile" >&2
    set +e
    amata "$@" 2>&1 | tee -a "$logfile" >&2
    status=${PIPESTATUS[0]}
    set -e
    if [[ ! -s "$logfile" ]]; then
        echo "[mig-codex] no output captured from amata" | tee -a "$logfile" >&2
    fi
    # Ensure codex-last.txt always exists (nodeagent parseCodexLastField relies on it).
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
fi

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

mkdir -p "$out_dir" /root/.codex

# Auth via file path
if [[ -n "$auth_file" ]]; then
  install -m 600 "$auth_file" /root/.codex/auth.json
fi

# Auth via env content.
# Note: CODEX_AUTH_JSON may be injected via `ploy config env set --key CODEX_AUTH_JSON ...`
# and propagated through the global config mechanism. See docs/envs/README.md#Global Env Configuration.
if [[ -n "${CODEX_AUTH_JSON:-}" ]]; then
  umask 077
  printf "%s" "$CODEX_AUTH_JSON" > /root/.codex/auth.json
fi

# Config via file path
if [[ -n "$config_file" ]]; then
  install -m 600 "$config_file" /root/.codex/config.toml
fi

# Config via env content
if [[ -n "${CODEX_CONFIG_TOML:-}" ]]; then
  umask 077
  printf "%s" "$CODEX_CONFIG_TOML" > /root/.codex/config.toml
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

# Session resume mode: If CODEX_RESUME=1 and a prior session ID is available in
# /in/codex-session.txt, we will use "codex exec resume <session>" to continue
# in the same thread. This preserves conversation context across healing attempts.
resume_session=""
if [[ "${CODEX_RESUME:-}" == "1" && -f "/in/codex-session.txt" ]]; then
  # Read session ID, stripping any trailing newlines/carriage returns.
  resume_session="$(tr -d '\r\n' < /in/codex-session.txt)"
fi

# For resume runs, prepend a short instruction to the prompt so Codex knows
# that the previous Build Gate still failed and it should continue healing
# from the existing context. Codex never runs the gate itself; the node agent
# handles validation externally based on workspace diffs.
if [[ -n "$resume_session" ]]; then
  resume_prefix="The previous Build Gate still failed (see /in/build-gate.log for the latest failure). Continue healing from the existing context by editing files under /workspace as needed to fix the error, then stop when you are done. Do not run build tools or tests inside this container; validation runs externally."
  prompt="$resume_prefix"$'\n\n'"$prompt"
fi

# Build Codex exec command
# Build base codex exec command; detect CLI capabilities via --help output.
cmd=(codex exec)
help_out="$(codex exec --help 2>&1 || true)"

# Detect auto-approval flag (required for non-interactive execution).
if grep -q -- "--yolo" <<<"$help_out"; then
  cmd+=(--yolo)
elif grep -q -- "--dangerously-bypass-approvals-and-sandbox" <<<"$help_out"; then
  cmd+=(--dangerously-bypass-approvals-and-sandbox)
fi

# Detect --add-dir support for attaching repository context.
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

# Always include /in as context when the directory exists.
if [[ "$supports_add_dir" == true && -d "/in" ]]; then
  cmd+=(--add-dir "/in")
fi

# Detect and enable structured output options for session/thread capture.
# --json: Emit JSON events to stdout (enables JSONL capture for downstream parsing).
if grep -q -- "--json" <<<"$help_out"; then
  cmd+=(--json)
fi

# --output-last-message: Write the final assistant message to a file (for debugging).
if grep -q -- "--output-last-message" <<<"$help_out"; then
  cmd+=(--output-last-message "$out_dir/codex-last.txt")
fi

# --output-dir: Write full transcript/session data to a directory (for audit/resume).
if grep -q -- "--output-dir" <<<"$help_out"; then
  cmd+=(--output-dir "$out_dir/codex-transcript")
fi

# Append resume sub-command if we have a prior session ID. This instructs the Codex
# CLI to continue the existing conversation thread rather than starting fresh.
# The "resume <session_id>" subcommand must come after all flags but before the
# trailing "-" that signals stdin prompt input.
if [[ -n "$resume_session" ]]; then
  cmd+=(resume "$resume_session")
fi

cmd+=( - )

# Run Codex; pipe prompt via stdin; capture stdout/stderr to log and JSONL files.
logfile="$out_dir/codex.log"
manifest="$out_dir/codex-run.json"
jsonl="$out_dir/codex-events.jsonl"

# Initialize log file with start message; log resume mode if active.
echo "[mig-codex] starting codex exec with repo context" | tee "$logfile" >&2
if [[ -n "$resume_session" ]]; then
  echo "[mig-codex] resume mode enabled; session=$resume_session" | tee -a "$logfile" >&2
fi
set +e
# Pipe Codex output to:
#   1. codex.log (human-readable log, appended)
#   2. codex-events.jsonl (JSON events for session ID extraction)
#   3. stderr (captured by Docker → Ploy log streamer → SSE logs endpoint)
# The tee chain ensures all destinations receive the same stream.
printf "%s" "$prompt" | "${cmd[@]}" 2>&1 | tee -a "$logfile" | tee "$jsonl" >&2
status=${PIPESTATUS[1]}
set -e
if [[ ! -s "$logfile" ]]; then
  echo "[mig-codex] no output captured from codex" | tee -a "$logfile" >&2
fi

# Extract session/thread ID from JSON events (if jq is available and events were captured).
# The Codex CLI emits thread.started events with a thread_id field we can use for resume.
session_id=""
if command -v jq >/dev/null 2>&1 && [[ -s "$jsonl" ]]; then
  # Select the first thread.started event and extract thread_id; ignore parse errors.
  session_id="$(jq -r 'select(.type=="thread.started") | .thread_id // empty' "$jsonl" 2>/dev/null | head -1 || true)"
fi

# Write session ID to a separate file for downstream consumption (nodeagent resume mode).
if [[ -n "$session_id" ]]; then
  printf "%s\n" "$session_id" > "$out_dir/codex-session.txt"
fi

# Ensure codex-last.txt always exists (reliable capture for nodeagent parseCodexLastField).
# If --output-last-message was unavailable or didn't produce a file, fall back to
# extracting from JSONL events or the raw log.
if [[ ! -s "$out_dir/codex-last.txt" ]]; then
  last_msg=""
  # Try extracting the last assistant message from JSONL events.
  if command -v jq >/dev/null 2>&1 && [[ -s "$jsonl" ]]; then
    last_msg="$(jq -r 'select(.type=="message" and .role=="assistant") | .content // empty' "$jsonl" 2>/dev/null | tail -1 || true)"
  fi
  # Fall back to the last non-empty line from codex.log.
  if [[ -z "$last_msg" && -s "$logfile" ]]; then
    last_msg="$(grep -v '^\s*$' "$logfile" | tail -1 || true)"
  fi
  if [[ -n "$last_msg" ]]; then
    printf "%s\n" "$last_msg" > "$out_dir/codex-last.txt"
  else
    # Write an empty marker so the file always exists.
    touch "$out_dir/codex-last.txt"
  fi
fi

# Write run manifest with all captured metadata for nodeagent consumption.
# Fields:
#   ts                       - ISO timestamp of completion
#   exit_code                - Codex CLI exit status
#   model                    - Model used (may be empty if default)
#   input                    - Input directory path
#   session_id               - Thread/session ID for resume (may be empty)
#   resumed                  - Boolean indicating if this was a resume run
ts=$(date -u +%Y-%m-%dT%H:%M:%SZ)

# Determine if this was a resumed session (for manifest metadata).
was_resumed=false
if [[ -n "$resume_session" ]]; then
  was_resumed=true
fi

printf '{"ts":"%s","exit_code":%s,"model":"%s","input":"%s","session_id":"%s","resumed":%s}\n' \
  "$ts" "${status:-0}" "${model}" "$input_dir" \
  "${session_id}" "${was_resumed}" > "$manifest"

exit "${status:-0}"
