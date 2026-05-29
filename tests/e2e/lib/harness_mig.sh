#!/usr/bin/env bash
set -euo pipefail

e2e_mig_run_json() {
  local spec="${1:?spec path is required}"
  local selector="${2:-}"
  shift
  if [[ $# -gt 0 ]]; then
    shift
  fi

  local follow=0
  local pull_path=""
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --follow)
        follow=1
        shift
        ;;
      --pull)
        pull_path="${2:?--pull requires a path}"
        shift 2
        ;;
      *)
        echo "error: unsupported e2e run option: $1" >&2
        return 1
        ;;
    esac
  done

  local output run_id mig_id
  if [[ -n "$selector" ]]; then
    output="$("$PLOY_BIN" run "$spec" "$selector")"
  else
    output="$("$PLOY_BIN" run "$spec")"
  fi

  run_id="$(printf '%s\n' "$output" | awk -F': ' '/^run_id:/ {print $2; exit}')"
  mig_id="$(printf '%s\n' "$output" | awk -F': ' '/^mig_id:/ {print $2; exit}')"
  if [[ -z "$run_id" ]]; then
    echo "error: failed to parse run_id from ploy run output" >&2
    printf '%s\n' "$output" >&2
    return 1
  fi

  local follow_rc=0
  if [[ $follow -eq 1 ]]; then
    "$PLOY_BIN" run status "$run_id" --follow >&2 || follow_rc=$?
  fi
  if [[ -n "$pull_path" ]]; then
    "$PLOY_BIN" run pull "$run_id" "$pull_path" >&2
  fi

  local status_json="{}"
  if [[ $follow -eq 1 ]]; then
    status_json="$("$PLOY_BIN" run status "$run_id" --json 2>/dev/null || printf '{}')"
  fi
  jq -cn --argjson status "$status_json" --arg run_id "$run_id" --arg mig_id "$mig_id" \
    '$status + {run_id: $run_id, mig_id: ($status.mig_id // $mig_id)}'
  return "$follow_rc"
}

e2e_repo_selector() {
  local repo="${1:?repo is required}"
  local ref="${2:-}"

  local selector="$repo"
  if [[ "$selector" == *"://"* ]]; then
    selector="${selector#*://}"
    selector="${selector#*@}"
    selector="${selector#*/}"
  fi
  selector="${selector%.git}"
  selector="${selector%/}"

  if [[ -n "$ref" ]]; then
    selector="${selector}:${ref}"
  fi
  printf '%s' "$selector"
}

e2e_mig_run_id() {
  if [[ $# -gt 0 ]]; then
    printf '%s' "$1" | jq -r '.run_id // empty'
    return
  fi

  jq -r '.run_id // empty'
}

e2e_run_status_safe() {
  local run_id="${1:-}"

  if [[ -z "$run_id" ]]; then
    return 0
  fi

  "$PLOY_BIN" run status "$run_id" || true
}
