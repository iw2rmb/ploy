#!/usr/bin/env bash
set -euo pipefail

e2e_repo_root_from_script() {
  local script_path="${1:?script_path is required}"
  local script_dir=""
  local candidate=""

  script_dir="$(cd "$(dirname "$script_path")" && pwd)"
  candidate="$script_dir"

  while [[ "$candidate" != "/" ]]; do
    if [[ -d "$candidate/.git" ]]; then
      printf '%s\n' "$candidate"
      return 0
    fi
    candidate="$(dirname "$candidate")"
  done

  case "$script_dir" in
    */tests/e2e/*)
      printf '%s\n' "${script_dir%%/tests/e2e/*}"
      return 0
      ;;
  esac

  echo "error: failed to resolve repository root from ${script_path}" >&2
  return 1
}

e2e_init() {
  local script_path="${1:?script_path is required}"

  REPO_ROOT="$(e2e_repo_root_from_script "$script_path")"
  export REPO_ROOT

  export PLOY_CONFIG_HOME="${PLOY_CONFIG_HOME:-$HOME/.config/ploy/local}"
  ensure_local_descriptor "$REPO_ROOT" "$PLOY_CONFIG_HOME"

  PLOY_BIN="$REPO_ROOT/dist/ploy"
  if [[ ! -x "$PLOY_BIN" ]]; then
    echo "error: ploy binary not found at ${PLOY_BIN}; run 'make build' first" >&2
    return 1
  fi
  export PLOY_BIN
}
