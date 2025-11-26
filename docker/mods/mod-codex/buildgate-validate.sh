#!/usr/bin/env bash
# buildgate-validate.sh - Wrapper for calling ploy Build Gate HTTP API (repo+diff mode)
#
# Sends repo_url+ref(+diff_patch) payloads to the Build Gate API for build validation.
# The script no longer creates workspace tarballs; instead, it relies on Git refs and
# optional diff patches (gzipped unified diffs) for healing verification.
#
# Usage:
#   buildgate-validate --repo-url <url> --ref <ref> [options]
#
# Required flags or environment:
#   --repo-url <url>   | PLOY_REPO_URL        - Git repository URL
#   --ref <ref>        | PLOY_BUILDGATE_REF   - Git ref (branch/tag/commit)
#
# Optional flags or environment:
#   --profile <name>   | PLOY_BUILDGATE_PROFILE  - Build profile (default: auto)
#   --timeout <dur>    | PLOY_BUILDGATE_TIMEOUT  - Timeout duration (default: 10m)
#   --diff-patch <file>| PLOY_DIFF_PATCH_FILE    - Path to unified diff file to apply
#   --workspace <path> | WORKSPACE               - Reserved; accepted but unused in repo+diff mode
#
# Environment (connection):
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

# ─────────────────────────────────────────────────────────────────────────────
# Defaults from environment or fallback values
# ─────────────────────────────────────────────────────────────────────────────
repo_url="${PLOY_REPO_URL:-}"
ref="${PLOY_TARGET_REF:-}"
profile="${PLOY_BUILDGATE_PROFILE:-auto}"
timeout="${PLOY_BUILDGATE_TIMEOUT:-10m}"
diff_patch_file="${PLOY_DIFF_PATCH_FILE:-}"
workspace="${WORKSPACE:-/workspace}"

# ─────────────────────────────────────────────────────────────────────────────
# Parse command-line arguments (override env where provided)
# ─────────────────────────────────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo-url)
      repo_url="$2"; shift 2 ;;
    --ref)
      ref="$2"; shift 2 ;;
    --profile)
      profile="$2"; shift 2 ;;
    --timeout)
      timeout="$2"; shift 2 ;;
    --diff-patch)
      diff_patch_file="$2"; shift 2 ;;
    --workspace)
      workspace="$2"; shift 2 ;;
    -h|--help)
      cat <<EOF
Usage: buildgate-validate --repo-url <url> --ref <ref> [options]

Required:
  --repo-url <url>      Git repository URL (or PLOY_REPO_URL)
  --ref <ref>           Git ref to validate (or PLOY_BUILDGATE_REF)

Options:
  --profile <name>      Build profile (default: auto, or PLOY_BUILDGATE_PROFILE)
  --timeout <dur>       Timeout duration (default: 10m, or PLOY_BUILDGATE_TIMEOUT)
  --diff-patch <file>   Path to unified diff file to apply on top of repo+ref
  --workspace <path>    Reserved; accepted but unused in repo+diff mode
  -h, --help            Show this help message

Environment:
  PLOY_SERVER_URL       Required: ploy server base URL
  PLOY_API_TOKEN        Optional: bearer token for authentication
  PLOY_CA_CERT_PATH     Optional: CA cert for mTLS
  PLOY_CLIENT_CERT_PATH Optional: client cert for mTLS
  PLOY_CLIENT_KEY_PATH  Optional: client key for mTLS
EOF
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done

# ─────────────────────────────────────────────────────────────────────────────
# Validate required parameters
# ─────────────────────────────────────────────────────────────────────────────
if [[ -z "${PLOY_SERVER_URL:-}" ]]; then
  echo "error: PLOY_SERVER_URL is required" >&2; exit 2
fi
if [[ -z "$repo_url" ]]; then
  echo "error: --repo-url or PLOY_REPO_URL is required" >&2; exit 2
fi
if [[ -z "$ref" ]]; then
  echo "error: --ref or PLOY_BUILDGATE_REF is required" >&2; exit 2
fi

# ─────────────────────────────────────────────────────────────────────────────
# encode_diff_patch: Reads a unified diff file, gzips, and base64-encodes it.
# Args: <file_path>
# Output: base64-encoded gzipped diff (no newlines)
# ─────────────────────────────────────────────────────────────────────────────
encode_diff_patch() {
  local file="$1"
  if [[ ! -f "$file" ]]; then
    echo "error: diff patch file not found: $file" >&2
    return 1
  fi
  # Gzip the diff and base64-encode (strip newlines for JSON embedding)
  gzip -c "$file" | base64 | tr -d '\n'
}

# ─────────────────────────────────────────────────────────────────────────────
# Build the JSON request payload with repo_url, ref, profile, timeout,
# and optional diff_patch.
# ─────────────────────────────────────────────────────────────────────────────
diff_patch_b64=""
if [[ -n "$diff_patch_file" ]]; then
  # User explicitly provided a diff patch file
  diff_patch_b64=$(encode_diff_patch "$diff_patch_file")
fi

# Construct the request JSON. Use jq if available for proper escaping;
# otherwise fall back to a simple heredoc approach.
if command -v jq >/dev/null 2>&1; then
  # Build JSON safely using jq (handles special characters)
  if [[ -n "$diff_patch_b64" ]]; then
    request_json=$(jq -n \
      --arg repo_url "$repo_url" \
      --arg ref "$ref" \
      --arg profile "$profile" \
      --arg timeout "$timeout" \
      --arg diff_patch "$diff_patch_b64" \
      '{repo_url: $repo_url, ref: $ref, profile: $profile, timeout: $timeout, diff_patch: $diff_patch}')
  else
    request_json=$(jq -n \
      --arg repo_url "$repo_url" \
      --arg ref "$ref" \
      --arg profile "$profile" \
      --arg timeout "$timeout" \
      '{repo_url: $repo_url, ref: $ref, profile: $profile, timeout: $timeout}')
  fi
else
  # Fallback: simple heredoc (assumes no special chars in values)
  if [[ -n "$diff_patch_b64" ]]; then
    request_json=$(cat <<EOF
{
  "repo_url": "$repo_url",
  "ref": "$ref",
  "profile": "$profile",
  "timeout": "$timeout",
  "diff_patch": "$diff_patch_b64"
}
EOF
)
  else
    request_json=$(cat <<EOF
{
  "repo_url": "$repo_url",
  "ref": "$ref",
  "profile": "$profile",
  "timeout": "$timeout"
}
EOF
)
  fi
fi

# ─────────────────────────────────────────────────────────────────────────────
# Prepare curl arguments for the HTTP request
# ─────────────────────────────────────────────────────────────────────────────
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

# ─────────────────────────────────────────────────────────────────────────────
# Call the Build Gate API
# ─────────────────────────────────────────────────────────────────────────────
echo "[buildgate] Validating build via $PLOY_SERVER_URL/v1/buildgate/validate" >&2
echo "[buildgate] repo=$repo_url ref=$ref profile=$profile timeout=$timeout" >&2
if [[ -n "$diff_patch_b64" ]]; then
  echo "[buildgate] diff_patch provided ($(echo -n "$diff_patch_b64" | wc -c | tr -d ' ') bytes encoded)" >&2
fi

response=$(curl "${curl_args[@]}" "${PLOY_SERVER_URL}/v1/buildgate/validate")

# ─────────────────────────────────────────────────────────────────────────────
# Parse and display the response
# ─────────────────────────────────────────────────────────────────────────────
if command -v jq >/dev/null 2>&1; then
  job_id=$(echo "$response" | jq -r '.job_id // empty')
  status=$(echo "$response" | jq -r '.status // empty')
else
  job_id=$(echo "$response" | grep -o '"job_id":"[^"]*"' | cut -d'"' -f4)
  status=$(echo "$response" | grep -o '"status":"[^"]*"' | cut -d'"' -f4)
fi

echo "[buildgate] Job submitted: $job_id (status: $status)" >&2

# ─────────────────────────────────────────────────────────────────────────────
# Check for sync completion or poll for async result
# ─────────────────────────────────────────────────────────────────────────────
if echo "$response" | grep -q '"result"'; then
  # Result available immediately (sync completion)
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
  # Async mode: poll until completed or failed
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
