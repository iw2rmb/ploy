#!/usr/bin/env bash
set -euo pipefail
usage() { echo "usage: $0 --tar <jib_tar> --main <MainClass> [--out <image.qcow2>] [--app <appname>] [--sha <gitsha>]"; exit 1; }

TAR=""; MAIN=""; OUT=""; APP=""; SHA=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --tar) TAR="$2"; shift 2;;
    --main) MAIN="$2"; shift 2;;
    --out) OUT="$2"; shift 2;;
    --app) APP="$2"; shift 2;;
    --sha) SHA="$2"; shift 2;;
    *) usage;;
  esac
done

[[ -z "$TAR" || -z "$MAIN" ]] && usage
APP="${APP:-java-app}"
SHA="${SHA:-dev}"
OUT="${OUT:-${APP}-${SHA}.qcow2}"

which capstan >/dev/null 2>&1 || { echo "capstan not found in PATH"; exit 2; }

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
WORK="$(mktemp -d)"
cleanup(){ rm -rf "$WORK"; }
trap cleanup EXIT

STAGING="$WORK/staging"; mkdir -p "$STAGING"
case "$TAR" in
  *.tar) tar -xf "$TAR" -C "$STAGING" ;;
  *.tar.gz|*.tgz) tar -xzf "$TAR" -C "$STAGING" ;;
  *) echo "Unsupported tar format: $TAR"; exit 3;;
esac

APPDIR="$WORK/capstan_project/files/app"; mkdir -p "$APPDIR"
for d in classes resources libs dependencies ; do
  [[ -d "$STAGING/$d" ]] && cp -a "$STAGING/$d" "$APPDIR/$d"
done

CF="$WORK/capstan_project/Capstanfile"; mkdir -p "$(dirname "$CF")"
sed "s|{{MAIN_CLASS}}|$MAIN|g; s|{{APP}}|$APP|g" "$SCRIPT_DIR/Capstanfile.tmpl" > "$CF"

pushd "$WORK/capstan_project" >/dev/null
capstan build -p qemu -f qcow2 -v
popd >/dev/null

IMG_DIR="$HOME/.capstan/repository/ploy/${APP}/qemu"
IMG="${IMG_DIR}/${APP}.qcow2"
[[ -f "$IMG" ]] || { echo "Expected Capstan image not found at $IMG"; exit 4; }
cp "$IMG" "$OUT"
echo "OSv image written to $OUT"

# SBOM/signature (optional)

# SBOM/signature (optional)
if command -v syft >/dev/null 2>&1; then syft scan "$OUT" -o json > "$OUT.sbom.json" || true; fi
if command -v cosign >/dev/null 2>&1; then cosign sign-blob --yes --output-signature "$OUT.sig" "$OUT" || true; fi

