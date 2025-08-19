#!/usr/bin/env bash
set -euo pipefail
usage(){ echo "usage: $0 --app <name> --sha <sha> --out-dir <dir>"; exit 1; }
APP=""; SHA="dev"; OUTDIR="."
while [[ $# -gt 0 ]]; do
  case "$1" in
    --app) APP="$2"; shift 2;;
    --sha) SHA="$2"; shift 2;;
    --out-dir) OUTDIR="$2"; shift 2;;
    *) usage;;
  esac
done
[[ -z "$APP" ]] && usage
OUT="$OUTDIR/${APP}-${SHA}.img"
dd if=/dev/zero of="$OUT" bs=1M count=64
echo "$OUT"

# SBOM/signature (optional)

if command -v syft >/dev/null 2>&1; then syft scan "$OUT" -o json > "$OUT.sbom.json" || true; fi
if command -v cosign >/dev/null 2>&1; then cosign sign-blob --yes --output-signature "$OUT.sig" "$OUT" || true; fi

