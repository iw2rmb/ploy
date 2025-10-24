#!/usr/bin/env bash
set -euo pipefail

if [[ -z "${CODEX_BIN:-}" ]]; then
  echo "CODEX_BIN must be set" >&2
  exit 1
fi

exec "${CODEX_BIN}" "$@"
