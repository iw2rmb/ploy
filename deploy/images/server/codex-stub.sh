#!/bin/sh
set -eu

if [ "${1:-}" != "exec" ]; then
  echo "codex stub: expected 'exec' subcommand" >&2
  exit 2
fi
shift

while [ "$#" -gt 0 ]; do
  case "$1" in
    --json-output|--non-interactive)
      shift
      ;;
    --*)
      # Accept unsupported flags for forward-compat with local e2e fixtures.
      shift
      ;;
    *)
      break
      ;;
  esac
done

if [ "$#" -lt 1 ]; then
  echo "codex stub: prompt argument is required" >&2
  exit 2
fi

repo_id="${PLOY_PREP_REPO_ID:-}"
target_ref="${PLOY_PREP_TARGET_REF:-}"

if [ -z "$repo_id" ]; then
  echo "codex stub: PLOY_PREP_REPO_ID is required" >&2
  exit 2
fi

case "$target_ref" in
  *fail*)
    echo "{\"error\":\"forced prep failure\",\"target_ref\":\"$target_ref\"}"
    exit 1
    ;;
esac

cat <<EOF
{"schema_version":1,"repo_id":"$repo_id","runner_mode":"simple","targets":{"build":{"status":"passed","command":"go test ./...","env":{},"failure_code":null},"unit":{"status":"not_attempted","env":{}},"all_tests":{"status":"not_attempted","env":{}}},"orchestration":{"pre":[],"post":[]},"tactics_used":["go_default"],"attempts":[],"evidence":{"log_refs":["inline://prep/stub"],"diagnostics":[]},"repro_check":{"status":"passed","details":"stub prep profile"},"prompt_delta_suggestion":{"status":"none","summary":"","candidate_lines":[]}}
EOF
