#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -lt 1 ]; then
  echo "usage: roadmap-search.sh <regex> [targets_newline_separated]" >&2
  exit 2
fi

regex="$1"
targets_raw="${2-}"
target_args=()

if [ -n "$targets_raw" ]; then
  while IFS= read -r target; do
    [ -z "$target" ] && continue
    target_args+=("-g" "$target")
  done <<< "$targets_raw"
fi

if rg -n --column --no-heading --color never -S -e "$regex" \
  -g '!.git/**' -g '!build/**' -g '!target/**' -g '!out/**' \
  "${target_args[@]}" .; then
  exit 0
fi

code=$?
if [ "$code" -eq 1 ]; then
  exit 0
fi
exit "$code"
