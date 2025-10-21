#!/bin/zsh
set -euo pipefail

# Directory layout helpers
SCRIPT_DIR="${0:A:h}"
REPO_ROOT="${SCRIPT_DIR:h:h}"
TASK_DIR="${REPO_ROOT}/docs/tasks/roadmap"

if (( $# == 0 )); then
  print -u2 "usage: ${0##*/} task-file [task-file ...]"
  print -u2 "Provide task files located under docs/tasks/roadmap (relative paths)."
  exit 1
fi

CODEX_BIN="${CODEX_BIN:-codex}"
CODEX_ARGS=(
  exec
  "--dangerously-bypass-approvals-and-sandbox"
  "--cd" "$REPO_ROOT"
)

for task in "$@"; do
  if [[ "$task" == /* ]]; then
    candidate="$task"
  else
    candidate="${TASK_DIR}/${task}"
  fi

  if [[ ! -f "$candidate" ]]; then
    print -u2 "task file not found: $candidate"
    exit 2
  fi

  print "Implementing task: ${candidate}"
  prompt=$'Implement the following task as described:\n\n'"$(<"$candidate")"$'\n'
  "$CODEX_BIN" "${CODEX_ARGS[@]}" - <<< "$prompt"
done
