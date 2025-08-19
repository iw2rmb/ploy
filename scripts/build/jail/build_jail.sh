#!/usr/bin/env bash
set -euo pipefail
usage(){ echo "usage: $0 --app <name> --src <dir> --sha <sha> --out-dir <dir>"; exit 1; }
APP=""; SRC=""; SHA="dev"; OUTDIR="."
while [[ $# -gt 0 ]]; do
  case "$1" in
    --app) APP="$2"; shift 2;;
    --src) SRC="$2"; shift 2;;
    --sha) SHA="$2"; shift 2;;
    --out-dir) OUTDIR="$2"; shift 2;;
    *) usage;;
  esac
done
[[ -z "$APP" || -z "$SRC" ]] && usage
OUT="$OUTDIR/${APP}-${SHA}-jail.tar"
tar -cf "$OUT" -C "$SRC" .
echo "$OUT"

# SBOM/signature (optional)

if command -v syft >/dev/null 2>&1; then syft scan "$OUT" -o json > "$OUT.sbom.json" || true; fi

# Enhanced keyless OIDC artifact signing
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -f "$SCRIPT_DIR/../common/signing.sh" ]]; then
  source "$SCRIPT_DIR/../common/signing.sh"
  sign_ploy_artifact "$OUT" "artifact" || true
else
  # Fallback to basic signing
  if command -v cosign >/dev/null 2>&1; then 
    cosign sign-blob --yes --output-signature "$OUT.sig" "$OUT" || true
  fi
fi

