#!/usr/bin/env bash
# Fetch Traefik logs from VPS (systemd + file logs)
# Usage:
#   TARGET_HOST=your.vps.ip ./tests/lanes/fetch-traefik-logs.sh
# Optional:
#   LINES=500 FOLLOW=true

set -euo pipefail

TARGET_HOST=${TARGET_HOST:-}
LINES=${LINES:-200}
FOLLOW=${FOLLOW:-false}

if [[ -z "$TARGET_HOST" ]]; then
  echo "TARGET_HOST is required" >&2
  exit 1
fi

ssh -o ConnectTimeout=20 "root@${TARGET_HOST}" bash -s <<EOF
set -euo pipefail
echo "===> Traefik journal (last ${LINES} lines)" >&2
journalctl -u traefik -n ${LINES} --no-pager || true
echo
echo "===> /var/log/traefik/traefik.log (last ${LINES} lines)" >&2
tail -n ${LINES} /var/log/traefik/traefik.log 2>/dev/null || echo "No /var/log/traefik/traefik.log"
echo
echo "===> /var/log/traefik/access.log (last ${LINES} lines)" >&2
tail -n ${LINES} /var/log/traefik/access.log 2>/dev/null || echo "No /var/log/traefik/access.log"
EOF

