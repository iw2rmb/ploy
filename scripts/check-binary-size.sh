#!/usr/bin/env bash
# Binary size guardrail for CLI changes.
# Protects against excessive growth from new dependencies.
#
# Usage: ./scripts/check-binary-size.sh <binary-path> [threshold-mb]
#
# Exits 0 if binary size is within threshold, 1 otherwise.

set -euo pipefail

# Constants
readonly DEFAULT_THRESHOLD_MB=15  # Maximum allowed binary size in megabytes
readonly SCRIPT_NAME="$(basename "$0")"

# Helper: Print error message to stderr and exit
err() {
  echo "ERROR: $*" >&2
  exit 1
}

# Helper: Print usage message
usage() {
  cat <<EOF
Usage: $SCRIPT_NAME <binary-path> [threshold-mb]

Arguments:
  binary-path    Path to the binary to check (required)
  threshold-mb   Maximum allowed size in megabytes (default: $DEFAULT_THRESHOLD_MB)

Examples:
  $SCRIPT_NAME dist/ploy
  $SCRIPT_NAME dist/ploy 20

Exit codes:
  0 - Binary size is within threshold
  1 - Binary size exceeds threshold or error occurred
EOF
  exit 1
}

# Parse arguments
if [[ $# -lt 1 ]] || [[ $# -gt 2 ]]; then
  usage
fi

BINARY_PATH="$1"
THRESHOLD_MB="${2:-$DEFAULT_THRESHOLD_MB}"

# Validate binary exists
if [[ ! -f "$BINARY_PATH" ]]; then
  err "Binary not found at path: $BINARY_PATH"
fi

# Validate threshold is a positive number
if ! [[ "$THRESHOLD_MB" =~ ^[0-9]+$ ]] || [[ "$THRESHOLD_MB" -le 0 ]]; then
  err "Threshold must be a positive integer, got: $THRESHOLD_MB"
fi

# Get binary size in bytes
# Use 'stat' with platform-specific flags (macOS vs Linux)
if stat -f%z "$BINARY_PATH" >/dev/null 2>&1; then
  # macOS (BSD stat)
  SIZE_BYTES=$(stat -f%z "$BINARY_PATH")
elif stat -c%s "$BINARY_PATH" >/dev/null 2>&1; then
  # Linux (GNU stat)
  SIZE_BYTES=$(stat -c%s "$BINARY_PATH")
else
  err "Unable to determine binary size (stat command failed)"
fi

# Convert bytes to megabytes (integer division)
SIZE_MB=$((SIZE_BYTES / 1024 / 1024))

# Human-readable size for output
SIZE_HUMAN=$(numfmt --to=iec-i --suffix=B "$SIZE_BYTES" 2>/dev/null || echo "${SIZE_MB}MiB")

# Check threshold
if [[ "$SIZE_MB" -gt "$THRESHOLD_MB" ]]; then
  cat <<EOF
Binary size check FAILED
  Binary: $BINARY_PATH
  Size:   $SIZE_HUMAN ($SIZE_MB MB)
  Limit:  ${THRESHOLD_MB} MB
  Excess: $((SIZE_MB - THRESHOLD_MB)) MB over limit

To fix:
  1. Review recent dependency additions with 'go mod graph'
  2. Check for unused dependencies with 'go mod tidy'
  3. Consider build flags like '-ldflags=-s -w' to strip debug info
  4. Evaluate if new features justify the size increase
  5. If growth is justified, update threshold in Makefile or CI config
EOF
  exit 1
fi

# Success: Binary is within threshold
echo "Binary size check PASSED: $SIZE_HUMAN ($SIZE_MB MB) <= ${THRESHOLD_MB} MB limit"
exit 0
