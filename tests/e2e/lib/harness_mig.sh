#!/usr/bin/env bash
set -euo pipefail

e2e_gitlab_flags() {
  E2E_GITLAB_FLAGS=()

  if [[ -n "${PLOY_GITLAB_PAT:-}" ]]; then
    E2E_GITLAB_FLAGS+=(--gitlab-pat "${PLOY_GITLAB_PAT}")
  fi
  if [[ -n "${PLOY_GITLAB_DOMAIN:-}" ]]; then
    E2E_GITLAB_FLAGS+=(--gitlab-domain "${PLOY_GITLAB_DOMAIN}")
  fi
}

e2e_args_have_flag() {
  local wanted="${1:?wanted flag is required}"
  shift

  for arg in "$@"; do
    if [[ "$arg" == "$wanted" ]]; then
      return 0
    fi
  done
  return 1
}

e2e_mig_run_json() {
  local -a raw_args=("$@")
  local -a args=()
  local generated_spec=""
  local has_spec=0

  e2e_gitlab_flags

  while [[ ${#raw_args[@]} -gt 0 ]]; do
    case "${raw_args[0]}" in
      --repo-url)
        if [[ ${#raw_args[@]} -lt 2 ]]; then
          echo "error: --repo-url requires a value" >&2
          return 1
        fi
        args+=(--repo "${raw_args[1]}")
        raw_args=("${raw_args[@]:2}")
        ;;
      --repo-base-ref)
        if [[ ${#raw_args[@]} -lt 2 ]]; then
          echo "error: --repo-base-ref requires a value" >&2
          return 1
        fi
        args+=(--base-ref "${raw_args[1]}")
        raw_args=("${raw_args[@]:2}")
        ;;
      --repo-target-ref)
        if [[ ${#raw_args[@]} -lt 2 ]]; then
          echo "error: --repo-target-ref requires a value" >&2
          return 1
        fi
        args+=(--target-ref "${raw_args[1]}")
        raw_args=("${raw_args[@]:2}")
        ;;
      --spec)
        if [[ ${#raw_args[@]} -lt 2 ]]; then
          echo "error: --spec requires a value" >&2
          return 1
        fi
        has_spec=1
        args+=("${raw_args[0]}" "${raw_args[1]}")
        raw_args=("${raw_args[@]:2}")
        ;;
      *)
        args+=("${raw_args[0]}")
        raw_args=("${raw_args[@]:1}")
        ;;
    esac
  done

  if [[ $has_spec -eq 0 ]]; then
    generated_spec="$(mktemp "${TMPDIR:-/tmp}/ploy-e2e-spec.XXXXXX.yaml")"
    printf '{}\n' >"$generated_spec"
    args+=(--spec "$generated_spec")
  fi

  if [[ -n "${E2E_ARTIFACT_DIR:-}" ]] && ! e2e_args_have_flag --artifact-dir "${args[@]}"; then
    args+=(--artifact-dir "$E2E_ARTIFACT_DIR")
  fi

  "$PLOY_BIN" run --json "${args[@]}" "${E2E_GITLAB_FLAGS[@]}"
  local rc=$?
  if [[ -n "$generated_spec" && -f "$generated_spec" ]]; then
    rm -f "$generated_spec"
  fi
  return $rc
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
