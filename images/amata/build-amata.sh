#!/usr/bin/env bash
set -Eeuo pipefail

# Builds the amata binary from ../amata (sibling repo) and stages it into the
# amata Docker build context so the Dockerfile can COPY it without any
# in-image compilation.
#
# Output:
#   images/amata/amata-linux-<arch> for each arch in PLATFORM
#   images/amata/amata alias copied from the first PLATFORM entry
#
# Must be invoked from the ploy repository root, or run directly; the script
# resolves paths from its own location.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT=$HOME/@iw2rmb/amata

# Derive GOOS/GOARCH values from PLATFORM env var (e.g. linux/amd64,linux/arm64).
_PLATFORM_RAW="${PLATFORM:-linux/amd64,linux/arm64}"

AMATA_SRC_CANDIDATE="$(cd "$REPO_ROOT" 2>/dev/null && pwd)" || true

if [[ -z "$AMATA_SRC_CANDIDATE" || ! -d "$AMATA_SRC_CANDIDATE" ]]; then
  echo "error: amata source directory not found at ${REPO_ROOT}" >&2
  exit 1
fi

AMATA_SRC="$AMATA_SRC_CANDIDATE"
AMATA_MAIN="$AMATA_SRC/cmd/amata/main.go"

if [[ ! -f "$AMATA_MAIN" ]]; then
  echo "error: amata main package not found at ${AMATA_MAIN}" >&2
  exit 1
fi

first_arch=""
for platform in $(printf '%s' "$_PLATFORM_RAW" | tr ',' ' '); do
  goos="${platform%%/*}"
  goarch="${platform##*/}"
  if [[ "$goos" != "linux" ]]; then
    echo "error: unsupported GOOS '${goos}' in PLATFORM='${_PLATFORM_RAW}'; only linux is supported" >&2
    exit 1
  fi
  case "$goarch" in
    amd64|arm64|arm) ;;
    *) echo "error: unsupported GOARCH '${goarch}' in PLATFORM='${_PLATFORM_RAW}'" >&2; exit 1 ;;
  esac
  if [[ -z "$first_arch" ]]; then
    first_arch="$goarch"
  fi

  staged="$SCRIPT_DIR/amata-linux-${goarch}"
  echo "==> Building amata binary (${goos}/${goarch}) from ${AMATA_SRC} ..."
  (cd "$AMATA_SRC" && GOOS="$goos" GOARCH="$goarch" go build -o "$staged" ./cmd/amata)

  if [[ ! -f "$staged" ]]; then
    echo "error: go build succeeded but staged binary not found at ${staged}" >&2
    exit 1
  fi
  echo "==> Staged amata binary at ${staged}"
done

if [[ -z "$first_arch" ]]; then
  echo "error: PLATFORM='${_PLATFORM_RAW}' did not include any target platform" >&2
  exit 1
fi

cp "$SCRIPT_DIR/amata-linux-${first_arch}" "$SCRIPT_DIR/amata"
echo "==> Updated compatibility alias at ${SCRIPT_DIR}/amata (from linux/${first_arch})"
