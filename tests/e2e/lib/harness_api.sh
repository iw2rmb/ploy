#!/usr/bin/env bash
set -euo pipefail

e2e_api_get() {
  local path="${1:?path is required}"

  if [[ -z "${PLOY_SERVER_URL:-}" || -z "${PLOY_AUTH_TOKEN:-}" ]]; then
    echo "error: PLOY_SERVER_URL and PLOY_AUTH_TOKEN are required" >&2
    return 1
  fi

  curl -fsS \
    -H "Authorization: Bearer ${PLOY_AUTH_TOKEN}" \
    "${PLOY_SERVER_URL}${path}"
}
