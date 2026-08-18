#!/usr/bin/env bash
set -euo pipefail

e2e_runtime_image() {
  if [[ -z "${PLOY_E2E_IMAGE:-}" ]]; then
    echo "error: PLOY_E2E_IMAGE is required and must name a shell-capable image available to the Ploy nodes" >&2
    return 1
  fi
  printf '%s' "$PLOY_E2E_IMAGE"
}

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
  local -a run_args=(run)
  if [[ -n "${GITLAB_TOKEN:-}" ]]; then
    run_args+=(--gitlab-token-env GITLAB_TOKEN)
  fi
  run_args+=("$spec")
  if [[ -n "$selector" ]]; then
    run_args+=("$selector")
  fi
  output="$("$PLOY_BIN" "${run_args[@]}")"

  run_id="$(printf '%s\n' "$output" | awk -F': ' '/^run_id:/ {print $2; exit}')"
  mig_id="$(printf '%s\n' "$output" | awk -F': ' '/^mig_id:/ {print $2; exit}')"
  if [[ -z "$run_id" ]]; then
    echo "error: failed to parse run_id from ploy run output" >&2
    printf '%s\n' "$output" >&2
    return 1
  fi

  local follow_rc=0
  local status_json="{}"
  if [[ $follow -eq 1 ]]; then
    set +e
    status_json="$(e2e_wait_run_json "$run_id")"
    follow_rc=$?
    set -e
  fi
  if [[ -n "$pull_path" ]]; then
    "$PLOY_BIN" run pull "$run_id" "$pull_path" >&2
  fi

  jq -cn --argjson status "$status_json" --arg run_id "$run_id" --arg mig_id "$mig_id" \
    '$status + {run_id: $run_id, mig_id: ($status.mig_id // $mig_id)}'
  if [[ $follow_rc -ne 0 ]]; then
    echo "error: run ${run_id} completed with a non-success status" >&2
  fi
  return "$follow_rc"
}

e2e_wait_run_json() {
  local run_id="${1:?run id is required}"
  local timeout="${PLOY_E2E_TIMEOUT_SECONDS:-600}"
  local deadline=$((SECONDS + timeout))
  local status_json="{}"

  while ((SECONDS < deadline)); do
    status_json="$("$PLOY_BIN" run status "$run_id" --json 2>/dev/null || printf '{}')"
    if printf '%s' "$status_json" | jq -e '
      (.repos | length) > 0 and
      all(.repos[]; .status == "Success" or .status == "Fail" or .status == "Error" or .status == "Canceled" or .status == "Cancelled")
    ' >/dev/null; then
      printf '%s' "$status_json"
      if printf '%s' "$status_json" | jq -e 'all(.repos[]; .status == "Success")' >/dev/null; then
        return 0
      fi
      return 1
    fi
    sleep 1
  done

  echo "error: timed out after ${timeout}s waiting for run ${run_id}" >&2
  printf '%s' "$status_json"
  return 1
}

e2e_wait_job_log() {
  local job_id="${1:?job id is required}"
  local timeout="${PLOY_E2E_LOG_TIMEOUT_SECONDS:-60}"
  local deadline=$((SECONDS + timeout))
  local output=""

  while ((SECONDS < deadline)); do
    output="$("$PLOY_BIN" job log --format raw "$job_id" 2>/dev/null || true)"
    if [[ -n "$output" ]]; then
      printf '%s' "$output"
      return 0
    fi
    sleep 1
  done

  echo "error: timed out after ${timeout}s waiting for job log ${job_id}" >&2
  return 1
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
  selector="${selector%%\?*}"
  selector="${selector%%\#*}"
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
