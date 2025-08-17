#!/usr/bin/env bash
set -euo pipefail
usage(){ echo "usage: $0 --app <name> --app-dir <path> --lane A|B --sha <sha> --out-dir <dir>"; exit 1; }
APP=""; APPDIR=""; LANE="A"; SHA="dev"; OUTDIR="."
while [[ $# -gt 0 ]]; do
  case "$1" in
    --app) APP="$2"; shift 2;;
    --app-dir) APPDIR="$2"; shift 2;;
    --lane) LANE="$2"; shift 2;;
    --sha) SHA="$2"; shift 2;;
    --out-dir) OUTDIR="$2"; shift 2;;
    *) usage;;
  esac
done
[[ -z "$APP" || -z "$APPDIR" ]] && usage
OUT="$OUTDIR/${APP}-${SHA}.img"
./build/kraft/gen_kraft_yaml.sh --lane "$LANE" --app-dir "$APPDIR" >/dev/null 2>&1 || true
pushd "$APPDIR" >/dev/null
if command -v kraft >/dev/null 2>&1; then
  kraft build -j 4
  if [[ -f build/image_qemu ]]; then cp build/image_qemu "$OUT"; else dd if=/dev/zero of="$OUT" bs=1M count=4; fi
else
  dd if=/dev/zero of="$OUT" bs=1M count=4
fi
popd >/dev/null
echo "$OUT"

# SBOM/signature (optional)

if command -v syft >/dev/null 2>&1; then syft packages "$OUT" -o json > "$OUT.sbom.json" || true; fi
if command -v cosign >/dev/null 2>&1; then cosign sign-blob --yes --output-signature "$OUT.sig" "$OUT" || true; fi

