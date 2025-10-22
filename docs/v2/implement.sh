#!/bin/zsh
set -euo pipefail

# Directory layout helpers
SCRIPT_DIR="${0:A:h}"
REPO_ROOT="${SCRIPT_DIR:h:h}"
DESIGN_ROOT="${REPO_ROOT}/docs/design"

if (( $# == 0 )); then
  print -u2 "usage: ${0##*/} design-doc [design-doc ...]"
  print -u2 "Provide design docs (relative paths or directories) located under docs/design."
  exit 1
fi

CODEX_BIN="${CODEX_BIN:-codex}"
CODEX_ARGS=(
  exec
  "--dangerously-bypass-approvals-and-sandbox"
  "--search"
  "--cd" "$REPO_ROOT"
)

for doc in "$@"; do
  if [[ "$doc" == /* ]]; then
    candidate="$doc"
  else
    candidate="${DESIGN_ROOT}/${doc}"
  fi

  if [[ -d "$candidate" ]]; then
    candidate="${candidate%/}/README.md"
  fi

  if [[ ! -f "$candidate" ]]; then
    print -u2 "design doc not found: $candidate"
    exit 2
  fi

  print "Implementing design doc: ${candidate}"
  prompt=$'Implement the following design doc:\n\n'"$(<"$candidate")"$'\n'
  "$CODEX_BIN" "${CODEX_ARGS[@]}" - <<< "$prompt"
done
