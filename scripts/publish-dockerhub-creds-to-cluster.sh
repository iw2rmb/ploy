#!/usr/bin/env bash
set -euo pipefail

# Write Docker Hub credentials into ployd's systemd environment and perform docker login on remote hosts.
# Usage:
#   scripts/publish-dockerhub-creds-to-cluster.sh <host> [<host>...]
# Env:
#   DOCKERHUB_USERNAME (required)
#   DOCKERHUB_PAT (required)
#   SSH_USER (default: ploy)
#   SSH_IDENTITY (default: from ~/.config/ploy/clusters/<default>.json)

if [[ $# -lt 1 ]]; then
  echo "usage: $0 <host> [<host>...]" >&2
  exit 2
fi

if [[ -z "${DOCKERHUB_USERNAME:-}" || -z "${DOCKERHUB_PAT:-}" ]]; then
  echo "error: DOCKERHUB_USERNAME and DOCKERHUB_PAT must be set" >&2
  exit 2
fi

SSH_USER=${SSH_USER:-ploy}
SSH_IDENTITY=${SSH_IDENTITY:-}
if [[ -z "$SSH_IDENTITY" ]]; then
  # Try to read from default cluster descriptor
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
  echo "==> Publishing Docker Hub creds to $host"
  ssh -o BatchMode=yes -o StrictHostKeyChecking=accept-new "${SSH_OPTS[@]}" "$SSH_USER@$host" bash -lc "'
set -euo pipefail
sudo mkdir -p /etc/systemd/system/ployd.service.d
cat | sudo tee /etc/systemd/system/ployd.service.d/env.conf >/dev/null <<ENV
[Service]
Environment=DOCKERHUB_USERNAME=${DOCKERHUB_USERNAME}
Environment=DOCKERHUB_PAT=${DOCKERHUB_PAT}
ENV
echo "logging in to Docker Hub..."
echo "${DOCKERHUB_PAT}" | sudo docker login -u "${DOCKERHUB_USERNAME}" --password-stdin >/dev/null 2>&1 || true
sudo systemctl daemon-reload
sudo systemctl restart ployd || true
echo "OK: creds applied on $(hostname)"
'" || {
    echo "warning: failed to update $host" >&2
  }
done

echo "All done"

