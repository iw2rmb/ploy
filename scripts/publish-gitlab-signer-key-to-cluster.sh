#!/usr/bin/env bash
set -euo pipefail

# Publish PLOY_GITLAB_SIGNER_AES_KEY (base64) to ployd units on target hosts and restart.
# Usage:
#   scripts/publish-gitlab-signer-key-to-cluster.sh <host> [<host>...]
# Env:
#   PLOY_GITLAB_SIGNER_AES_KEY (required, base64-encoded 16/24/32-byte key)
#   SSH_USER (default: ploy)
#   SSH_IDENTITY (default: from ~/.config/ploy/clusters/<default>.json)

if [[ $# -lt 1 ]]; then
  echo "usage: $0 <host> [<host>...]" >&2
  exit 2
fi

if [[ -z "${PLOY_GITLAB_SIGNER_AES_KEY:-}" ]]; then
  echo "error: PLOY_GITLAB_SIGNER_AES_KEY must be set (base64)" >&2
  exit 2
fi

SSH_USER=${SSH_USER:-ploy}
SSH_IDENTITY=${SSH_IDENTITY:-}
if [[ -z "$SSH_IDENTITY" ]]; then
  if [[ -r "$HOME/.config/ploy/clusters/default" ]]; then
    cluster=$(cat "$HOME/.config/ploy/clusters/default")
    ident=$(jq -r '.ssh_identity_path // empty' "$HOME/.config/ploy/clusters/${cluster}.json" 2>/dev/null || true)
    if [[ -n "$ident" ]]; then
      SSH_IDENTITY=$(eval echo "$ident")
    fi
  fi
fi

if [[ -n "$SSH_IDENTITY" ]]; then
  SSH_OPTS=(-i "$SSH_IDENTITY")
else
  SSH_OPTS=()
fi

for host in "$@"; do
  echo "==> Publishing signer AES key to $host"
  ssh -o BatchMode=yes -o StrictHostKeyChecking=accept-new "${SSH_OPTS[@]}" "$SSH_USER@$host" bash -lc "'
set -euo pipefail
sudo mkdir -p /etc/systemd/system/ployd.service.d
cat | sudo tee /etc/systemd/system/ployd.service.d/env-gitlab-signer.conf >/dev/null <<ENV
[Service]
Environment=PLOY_GITLAB_SIGNER_AES_KEY=${PLOY_GITLAB_SIGNER_AES_KEY}
ENV
sudo systemctl daemon-reload
sudo systemctl restart ployd || true
echo "OK: signer key applied on $(hostname)"
'" || {
    echo "warning: failed to update $host" >&2
  }
done

echo "All done"

