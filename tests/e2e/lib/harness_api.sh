#!/usr/bin/env bash
set -euo pipefail

e2e_descriptor_path() {
  local marker="${PLOY_CONFIG_HOME}/clusters/default"
  local clusters_dir="${PLOY_CONFIG_HOME}/clusters"

  e2e_resolve_descriptor_path "$marker" "$clusters_dir"
}

e2e_descriptor_address() {
  local descriptor_path=""
  descriptor_path="$(e2e_descriptor_path)"
  e2e_descriptor_value "$descriptor_path" address
}

e2e_descriptor_token() {
  local descriptor_path=""
  descriptor_path="$(e2e_descriptor_path)"
  e2e_descriptor_value "$descriptor_path" token
}

e2e_api_get() {
  local path="${1:?path is required}"
  local server_url=""
  local token=""

  server_url="$(e2e_descriptor_address)"
  token="$(e2e_descriptor_token)"

  if [[ -z "$server_url" || -z "$token" ]]; then
    echo "error: failed to resolve server address/token from descriptor" >&2
    return 1
  fi

  curl -fsS \
    -H "Authorization: Bearer ${token}" \
    "${server_url}${path}"
}
