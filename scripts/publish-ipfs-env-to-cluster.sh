#!/usr/bin/env bash
set -euo pipefail

# Publish IPFS Cluster env to ployd units on target hosts and restart.
# Usage:
#   scripts/publish-ipfs-env-to-cluster.sh <host> [<host>...]
# Env:
#   PLOY_IPFS_CLUSTER_API (required)
#   PLOY_IPFS_CLUSTER_TOKEN (optional)
#   PLOY_IPFS_CLUSTER_USERNAME (optional)
#   PLOY_IPFS_CLUSTER_PASSWORD (optional)
#   PLOY_IPFS_CLUSTER_REPL_MIN (optional, default: 1)
#   PLOY_IPFS_CLUSTER_REPL_MAX (optional, default: 1)
#   SSH_USER (default: ploy)
#   SSH_IDENTITY (optional)

if [[ $# -lt 1 ]]; then
  echo "usage: $0 <host> [<host>...]" >&2
  exit 2
fi

if [[ -z "${PLOY_IPFS_CLUSTER_API:-}" ]]; then
  echo "error: PLOY_IPFS_CLUSTER_API must be set" >&2
  exit 2
fi

SSH_USER=${SSH_USER:-ploy}
SSH_IDENTITY=${SSH_IDENTITY:-}

REPL_MIN=${PLOY_IPFS_CLUSTER_REPL_MIN:-1}
REPL_MAX=${PLOY_IPFS_CLUSTER_REPL_MAX:-1}

for host in "$@"; do
  echo "==> Publishing IPFS Cluster env to $host"
  ssh -o BatchMode=yes -o StrictHostKeyChecking=accept-new ${SSH_IDENTITY:+-i "$SSH_IDENTITY"} "$SSH_USER@$host" bash -lc "'
set -euo pipefail
sudo mkdir -p /etc/systemd/system/ployd.service.d
cat | sudo tee /etc/systemd/system/ployd.service.d/90-ipfs.conf >/dev/null <<ENV
[Service]
Environment=PLOY_IPFS_CLUSTER_API=${PLOY_IPFS_CLUSTER_API}
Environment=PLOY_IPFS_CLUSTER_TOKEN=${PLOY_IPFS_CLUSTER_TOKEN-}
Environment=PLOY_IPFS_CLUSTER_USERNAME=${PLOY_IPFS_CLUSTER_USERNAME-}
Environment=PLOY_IPFS_CLUSTER_PASSWORD=${PLOY_IPFS_CLUSTER_PASSWORD-}
Environment=PLOY_IPFS_CLUSTER_REPL_MIN=${REPL_MIN}
Environment=PLOY_IPFS_CLUSTER_REPL_MAX=${REPL_MAX}
ENV
sudo systemctl daemon-reload
sudo systemctl restart ployd || true
echo "OK: IPFS env applied on $(hostname)"
'" || {
    echo "warning: failed to update $host" >&2
  }
done

echo "All done"
