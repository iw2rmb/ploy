#!/usr/bin/env bash
set -euo pipefail

# Update ployd on a set of nodes.
# - Builds Linux binary if missing (dist/ployd-linux)
# - Copies to each node at /usr/local/bin/ployd (atomic via temp file)
# - Restarts systemd unit and verifies it is active
#
# Usage:
#   NODES="45.9.42.212 46.173.16.177 81.200.119.187" scripts/update_ployd_cluster.sh
#   # or pass nodes as args
#   scripts/update_ployd_cluster.sh 45.9.42.212 46.173.16.177 81.200.119.187
#
# Env:
#   SSH_USER        SSH username (default: root)
#   BINARY          Path to ployd-linux (default: dist/ployd-linux)
#   SYSTEMCTL       Override systemctl command (default: systemctl)
#

SSH_USER=${SSH_USER:-root}
BINARY=${BINARY:-dist/ployd-linux}
SYSTEMCTL=${SYSTEMCTL:-systemctl}

nodes=()
if [[ ${#@} -gt 0 ]]; then
  nodes=("$@")
elif [[ -n "${NODES:-}" ]]; then
  # shellcheck disable=SC2206
  nodes=(${NODES})
else
  echo "Usage: NODES=\"<ip> <ip>...\" $0 or pass nodes as args" >&2
  exit 64
fi

if [[ ! -x "$BINARY" ]]; then
  echo "Building ployd for linux (missing $BINARY)" >&2
  GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
    go build -trimpath -ldflags='-s -w' -o "$BINARY" ./cmd/ployd
fi

for ip in "${nodes[@]}"; do
  echo "==> Updating node $ip"
  scp -q "$BINARY" "${SSH_USER}@${ip}:/usr/local/bin/ployd.new"
  ssh -q "${SSH_USER}@${ip}" "install -m 0755 /usr/local/bin/ployd.new /usr/local/bin/ployd && rm -f /usr/local/bin/ployd.new && ${SYSTEMCTL} restart ployd && ${SYSTEMCTL} is-active --quiet ployd"
  echo "OK: $ip running updated ployd"
done

echo "All nodes updated"

