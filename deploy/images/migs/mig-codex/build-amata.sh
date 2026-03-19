#!/usr/bin/env bash
set -Eeuo pipefail

# Builds the amata binary from ../amata (sibling repo) and stages it into the
# mig-codex Docker build context so the Dockerfile can COPY it without any
# in-image compilation.
#
# Output: deploy/images/migs/mig-codex/amata  (linux/amd64 ELF binary)
#
# Must be invoked from the ploy repository root, or run directly; the script
# resolves paths from its own location.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../../.." && pwd)"
AMATA_SRC_CANDIDATE="$(cd "$REPO_ROOT/../amata" 2>/dev/null && pwd)" || true

if [[ -z "$AMATA_SRC_CANDIDATE" || ! -d "$AMATA_SRC_CANDIDATE" ]]; then
  echo "error: amata source directory not found at ${REPO_ROOT}/../amata" >&2
  exit 1
fi

AMATA_SRC="$AMATA_SRC_CANDIDATE"
AMATA_MAIN="$AMATA_SRC/cmd/amata/main.go"

if [[ ! -f "$AMATA_MAIN" ]]; then
  echo "error: amata main package not found at ${AMATA_MAIN}" >&2
  exit 1
fi

STAGED="$SCRIPT_DIR/amata"

echo "==> Building amata binary (linux/amd64) from ${AMATA_SRC} ..."
(cd "$AMATA_SRC" && GOOS=linux GOARCH=amd64 go build -o "$STAGED" ./cmd/amata)

if [[ ! -f "$STAGED" ]]; then
  echo "error: go build succeeded but staged binary not found at ${STAGED}" >&2
  exit 1
fi

echo "==> Staged amata binary at ${STAGED}"
