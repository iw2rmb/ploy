#!/usr/bin/env bash
set -euo pipefail

# ploy-lab.sh — convenience wrapper for targeting the VPS lab control plane.
#
# Usage:
#   scripts/ploy-lab.sh <ploy args...>
# Examples:
#   scripts/ploy-lab.sh cluster rollout server --address 45.9.42.212 --binary dist/ployd-linux --user root
#   scripts/ploy-lab.sh cluster rollout nodes --all --binary dist/ployd-node-linux --user root
#   scripts/ploy-lab.sh mod run --repo-url https://github.com/example/repo.git --repo-base-ref main --repo-target-ref feature/x --follow
#
# The script sets PLOY_CONTROL_PLANE_URL to the lab server URL and delegates to
# the compiled CLI at dist/ploy. TLS (mTLS) credentials are read from your
# default cluster descriptor (~/.config/ploy/clusters/default → JSON file).

SCRIPT_DIR="$(cd "${BASH_SOURCE[0]%/*}" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Determine the lab URL:
# 1) Respect PLOY_LAB_URL if set
# 2) Else use address from the default descriptor JSON if jq is available
# 3) Else fall back to the known lab endpoint
LAB_URL_DEFAULT="https://45.9.42.212:8443"
LAB_URL="${PLOY_LAB_URL:-}"
if [[ -z "$LAB_URL" ]]; then
  if command -v jq >/dev/null 2>&1; then
    DEFAULT_ID_FILE="$HOME/.config/ploy/clusters/default"
    if [[ -f "$DEFAULT_ID_FILE" ]]; then
      DEFAULT_ID="$(cat "$DEFAULT_ID_FILE" 2>/dev/null || true)"
      DESC_FILE="$HOME/.config/ploy/clusters/${DEFAULT_ID}.json"
      if [[ -f "$DESC_FILE" ]]; then
        ADDRESS="$(jq -r '.address // empty' "$DESC_FILE" 2>/dev/null || true)"
        if [[ -n "$ADDRESS" ]]; then
          LAB_URL="$ADDRESS"
        fi
      fi
    fi
  fi
fi
if [[ -z "$LAB_URL" ]]; then
  LAB_URL="$LAB_URL_DEFAULT"
fi

# Resolve CLI path (built artifact)
CLI_BIN="${CLI_BIN:-$REPO_ROOT/dist/ploy}"
if [[ ! -x "$CLI_BIN" ]]; then
  echo "error: CLI not found at $CLI_BIN; run 'make build' first" >&2
  exit 1
fi

exec env PLOY_CONTROL_PLANE_URL="$LAB_URL" "$CLI_BIN" "$@"
