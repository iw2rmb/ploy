#!/usr/bin/env bash
set -euo pipefail

e2e_is_truthy() {
  case "${1:-}" in
    1|true|TRUE|yes|YES|on|ON)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

e2e_artifacts_init() {
  local default_base="${1:?default_base is required}"
  local skip_flag_var="${2:-}"
  local skip_flag_val=""
  local ts=""

  export PLOY_E2E_ARTIFACT_BASE="${PLOY_E2E_ARTIFACT_BASE:-$default_base}"

  if [[ -n "$skip_flag_var" ]]; then
    skip_flag_val="${!skip_flag_var:-0}"
    if e2e_is_truthy "$skip_flag_val"; then
      E2E_ARTIFACT_DIR=""
      return 0
    fi
  fi

  ts="$(date +%y%m%d%H%M%S)"
  export PLOY_E2E_ARTIFACT_DIR="${PLOY_E2E_ARTIFACT_DIR:-${PLOY_E2E_ARTIFACT_BASE}/${ts}}"
  E2E_ARTIFACT_DIR="$PLOY_E2E_ARTIFACT_DIR"
  mkdir -p "$E2E_ARTIFACT_DIR"
}
