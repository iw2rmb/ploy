#!/usr/bin/env bash
# buildgate-validate.sh - Wrapper for calling ploy buildgate HTTP API
#
# Usage: buildgate-validate [--workspace <path>] [--profile <profile>]
#
# Environment:
#   PLOY_SERVER_URL         - Required: ploy server URL (e.g., https://server:8443)
#   PLOY_CA_CERT_PATH       - Optional: path to CA certificate for mTLS
#   PLOY_CLIENT_CERT_PATH   - Optional: path to client certificate for mTLS
#   PLOY_CLIENT_KEY_PATH    - Optional: path to client key for mTLS
#   PLOY_API_TOKEN          - Optional: bearer token for Build Gate HTTP API
#
# Exit codes:
#   0: Build gate passed
#   1: Build gate failed or execution error
#   2: Invalid arguments

set -euo pipefail

# NOTE: Content truncated in repo snippet. Use full script content from original.
# For portability in this change, we embed the script from mods/mod-codex/buildgate-validate.sh.

workspace="${WORKSPACE:-/workspace}"
profile="${PLOY_BUILDGATE_PROFILE:-java}"
timeout="${PLOY_BUILDGATE_TIMEOUT:-10m}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --workspace) workspace="$2"; shift 2 ;;
    --profile) profile="$2"; shift 2 ;;
    -h|--help)
      echo "Usage: buildgate-validate [--workspace <path>] [--profile <profile>]"; exit 0 ;;
    *) echo "unknown arg: $1" >&2; exit 2 ;;
  esac
done

if [[ -z "${PLOY_SERVER_URL:-}" ]]; then
  echo "PLOY_SERVER_URL is required" >&2; exit 2
fi
if [[ ! -d "$workspace" ]]; then
  echo "workspace not found: $workspace" >&2; exit 2
fi

# Create a tarball of the workspace
workspace_tar=$(mktemp)
trap "rm -f '$workspace_tar'" EXIT

cd "$workspace"
tar czf "$workspace_tar" .
cd - >/dev/null

# Encode tarball as base64 for JSON
content_archive=$(base64 < "$workspace_tar" | tr -d '\n')

# Build request payload
request_json=$(cat <<EOF
{
  "content_archive": "$content_archive",
  "profile": "$profile",
  "timeout": "$timeout"
}
EOF
)

# Prepare curl arguments
curl_args=(
  -X POST
  -H "Content-Type: application/json"
  --data "$request_json"
  --silent
  --show-error
  --fail-with-body
)

# Optional bearer token authentication
if [[ -n "${PLOY_API_TOKEN:-}" ]]; then
  curl_args+=(-H "Authorization: Bearer $PLOY_API_TOKEN")
fi

# Add mTLS certificates if provided
if [[ -n "${PLOY_CA_CERT_PATH:-}" && -f "$PLOY_CA_CERT_PATH" ]]; then
  curl_args+=(--cacert "$PLOY_CA_CERT_PATH")
fi
if [[ -n "${PLOY_CLIENT_CERT_PATH:-}" && -f "$PLOY_CLIENT_CERT_PATH" ]]; then
  curl_args+=(--cert "$PLOY_CLIENT_CERT_PATH")
fi
if [[ -n "${PLOY_CLIENT_KEY_PATH:-}" && -f "$PLOY_CLIENT_KEY_PATH" ]]; then
  curl_args+=(--key "$PLOY_CLIENT_KEY_PATH")
fi

# Call the API
echo "[buildgate] Validating build via $PLOY_SERVER_URL/v1/buildgate/validate" >&2
response=$(curl "${curl_args[@]}" "${PLOY_SERVER_URL}/v1/buildgate/validate")

# Parse response
if command -v jq >/dev/null 2>&1; then
  job_id=$(echo "$response" | jq -r '.job_id // empty')
  status=$(echo "$response" | jq -r '.status // empty')
else
  job_id=$(echo "$response" | grep -o '"job_id":"[^"]*"' | cut -d'"' -f4)
  status=$(echo "$response" | grep -o '"status":"[^"]*"' | cut -d'"' -f4)
fi

echo "[buildgate] Job submitted: $job_id (status: $status)" >&2

# Check if result is available (sync completion)
if echo "$response" | grep -q '"result"'; then
  # Result available - extract and format
  if command -v jq >/dev/null 2>&1; then
    echo "$response" | jq .
  else
    echo "$response"
  fi

  if echo "$response" | grep -q '"passed":\s*true'; then
    echo "[buildgate] ✓ Build gate PASSED" >&2
    exit 0
  else
    echo "[buildgate] ✗ Build gate FAILED" >&2
    exit 1
  fi
else
  # Async - need to poll
  echo "[buildgate] Job processing asynchronously, polling for result..." >&2

  while true; do
    sleep 2

    poll_response=$(curl "${curl_args[@]}" "${PLOY_SERVER_URL}/v1/buildgate/jobs/${job_id}")
    if command -v jq >/dev/null 2>&1; then
      poll_status=$(echo "$poll_response" | jq -r '.status // empty')
    else
      poll_status=$(echo "$poll_response" | grep -o '"status":"[^"]*"' | cut -d'"' -f4)
    fi

    if [[ "$poll_status" == "completed" || "$poll_status" == "failed" ]]; then
      # Output final result
      if command -v jq >/dev/null 2>&1; then
        echo "$poll_response" | jq .
      else
        echo "$poll_response"
      fi

      if echo "$poll_response" | grep -q '"passed":\s*true'; then
        echo "[buildgate] ✓ Build gate PASSED" >&2
        exit 0
      else
        echo "[buildgate] ✗ Build gate FAILED" >&2
        exit 1
      fi
    fi

    echo "[buildgate] Status: $poll_status, waiting..." >&2
  done
fi
