#!/usr/bin/env bash
set -euo pipefail
usage(){ echo "usage: $0 --lane A|B --app-dir <dir> --port 8080"; exit 1; }
LANE="A"; APPDIR=""; PORT="8080"
while [[ $# -gt 0 ]]; do
  case "$1" in
    --lane) LANE="$2"; shift 2;;
    --app-dir) APPDIR="$2"; shift 2;;
    --port) PORT="$2"; shift 2;;
    *) usage;;
  esac
done
[[ -z "$APPDIR" ]] && usage
TPL="lanes/${LANE}-unikraft-minimal/kraft.yaml"
[[ "$LANE" == "B" ]] && TPL="lanes/${LANE}-unikraft-posix/kraft.yaml"
OUT="$APPDIR/kraft.yaml"
mkdir -p "$(dirname "$OUT")"
cp "$TPL" "$OUT" || true
sed -i.bak "s/http_port:.*/http_port: ${PORT}/" "$OUT" || true
echo "$OUT"
