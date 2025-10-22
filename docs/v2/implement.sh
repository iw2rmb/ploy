#!/bin/zsh
set -euo pipefail

# Directory layout helpers
SCRIPT_DIR="${0:A:h}"
REPO_ROOT="${SCRIPT_DIR:h:h}"
DESIGN_ROOT="${REPO_ROOT}/docs/design"
QUEUE_FILE="${REPO_ROOT}/docs/design/QUEUE.md"

if (( $# == 0 )); then
  if [[ ! -f "$QUEUE_FILE" ]]; then
    print -u2 "no design doc provided and queue file missing: $QUEUE_FILE"
    exit 1
  fi

  default_doc="$(
    python3 - "$QUEUE_FILE" <<'PY'
import pathlib
import sys

queue_path = pathlib.Path(sys.argv[1])
for raw_line in queue_path.read_text().splitlines():
    line = raw_line.strip()
    if not line.startswith("- [ ] "):
        continue
    first = line.find("`")
    if first == -1:
        continue
    second = line.find("`", first + 1)
    if second == -1:
        continue
    print(line[first + 1:second])
    break
PY
  )"
  if [[ -z "$default_doc" ]]; then
    print -u2 "no unclaimed design docs found in queue: $QUEUE_FILE"
    exit 1
  fi

  print "No design doc argument supplied; selecting from queue: ${default_doc}"
  set -- "$default_doc"
fi

CODEX_BIN="${CODEX_BIN:-codex}"
CODEX_ARGS=(
  exec
  "--dangerously-bypass-approvals-and-sandbox"
  "--cd" "$REPO_ROOT"
)

for doc in "$@"; do
  if [[ "$doc" == /* ]]; then
    candidate="$doc"
  elif [[ "$doc" == docs/design/* ]]; then
    candidate="${REPO_ROOT}/${doc}"
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
