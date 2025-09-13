#!/usr/bin/env bash
set -euo pipefail

# Usage: ./watch-events.sh <EXEC_ID>

if [[ $# -lt 1 ]]; then
  echo "Usage: $0 <EXEC_ID>" >&2
  exit 1
fi

EXEC_ID="$1"
API_BASE="${PLOY_CONTROLLER:-}"
if [[ -z "$API_BASE" ]]; then
  echo "PLOY_CONTROLLER must be set (e.g., https://api.dev.ployman.app/v1)" >&2
  exit 1
fi

curl -sN "$API_BASE/transflow/logs/$EXEC_ID?follow=1"

