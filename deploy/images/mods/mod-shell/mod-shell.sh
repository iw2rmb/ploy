#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<USAGE
mod-shell --script <path> [--dir <workspace>] [--out <dir>]

Environment:
  MOD_SHELL_SCRIPT   Relative or absolute script path to execute.
  WORKSPACE          Workspace directory (default: /workspace).
  OUTDIR             Output directory for reports/logs (default: /out).

Behavior:
  - Changes directory to the workspace.
  - Resolves the script path (absolute or relative to workspace).
  - Executes the script with bash, inheriting the current environment.
  - Writes a small run report to <out>/shell-run.json.
USAGE
}

workspace="${WORKSPACE:-/workspace}"
outdir="${OUTDIR:-/out}"
script_path=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --script)
      script_path="$2"; shift 2 ;;
    --dir)
      workspace="$2"; shift 2 ;;
    --out)
      outdir="$2"; shift 2 ;;
    -h|--help)
      usage; exit 0 ;;
    *)
      echo "unknown arg: $1" >&2
      usage >&2
      exit 1 ;;
  esac
done

mkdir -p "$outdir"

if [[ -z "$script_path" ]]; then
  script_path="${MOD_SHELL_SCRIPT:-}"
fi

if [[ -z "$script_path" ]]; then
  echo "error: --script <path> or MOD_SHELL_SCRIPT is required" >&2
  usage >&2
  exit 2
fi

cd "$workspace"

if [[ "$script_path" != /* ]]; then
  script_path="$workspace/$script_path"
fi

if [[ ! -f "$script_path" ]]; then
  echo "error: script not found: $script_path" >&2
  exit 3
fi

echo "[mod-shell] executing script: $script_path"

status=0
bash "$script_path" || status=$?

ts=$(date -u +%Y-%m-%dT%H:%M:%SZ)
report="$outdir/shell-run.json"

if [[ $status -ne 0 ]]; then
  echo "[mod-shell] script failed with exit $status" >&2
  cat > "$report" <<JSON
{"success":false,"script":"$script_path","workspace":"$workspace","exit_code":$status,"ts":"$ts"}
JSON
  exit "$status"
fi

cat > "$report" <<JSON
{"success":true,"script":"$script_path","workspace":"$workspace","exit_code":0,"ts":"$ts"}
JSON
echo "[mod-shell] completed successfully"
