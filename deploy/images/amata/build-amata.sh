#!/usr/bin/env bash
set -Eeuo pipefail

# Builds the amata binary from ../amata (sibling repo) and stages it into the
# amata Docker build context so the Dockerfile can COPY it without any
# in-image compilation.
#
# Output: deploy/images/amata/amata  (ELF binary for PLATFORM, default linux/amd64)
#
# Must be invoked from the ploy repository root, or run directly; the script
# resolves paths from its own location.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../../.." && pwd)"

# Derive GOOS/GOARCH from PLATFORM env var (e.g. linux/amd64, linux/arm64).
# Callers (build-and-push.sh, garage.sh) expose PLATFORM; default to linux/amd64.
_PLATFORM="${PLATFORM:-linux/amd64}"
_GOOS="${_PLATFORM%%/*}"
_GOARCH="${_PLATFORM##*/}"
# Validate: only linux targets make sense for a container image.
if [[ "$_GOOS" != "linux" ]]; then
  echo "error: unsupported GOOS '${_GOOS}' derived from PLATFORM='${_PLATFORM}'; only linux is supported" >&2
  exit 1
fi
case "$_GOARCH" in
  amd64|arm64|arm) ;;
  *) echo "error: unsupported GOARCH '${_GOARCH}' derived from PLATFORM='${_PLATFORM}'" >&2; exit 1 ;;
esac

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

echo "==> Building amata binary (${_PLATFORM}) from ${AMATA_SRC} ..."
(cd "$AMATA_SRC" && GOOS="$_GOOS" GOARCH="$_GOARCH" go build -o "$STAGED" ./cmd/amata)

if [[ ! -f "$STAGED" ]]; then
  echo "error: go build succeeded but staged binary not found at ${STAGED}" >&2
  exit 1
fi

echo "==> Staged amata binary at ${STAGED}"
