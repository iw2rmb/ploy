#!/usr/bin/env bash
set -euo pipefail

# Dev-only helper to append/remove a Traefik buffering middleware for api.dev.ployman.app
#
# Requirements:
#   - SSH access to the VPS as root (TARGET_HOST env var)
#   - Remote Traefik dynamic config at /opt/ploy/traefik-data/dynamic-config.yml
#   - Traefik is configured to watch this file (no restart strictly required)
#
# Usage:
#   TARGET_HOST=your.vps.host scripts/dev/add-traefik-buffering-mw.sh add    # default
#   TARGET_HOST=your.vps.host scripts/dev/add-traefik-buffering-mw.sh remove
#
# The script creates a timestamped backup before changes and tags the block with markers
# so it can be removed cleanly.

REMOTE_FILE="/opt/ploy/traefik-data/dynamic-config.yml"
MARK_START="# BEGIN dev-buffering-middleware (api.dev.ployman.app)"
MARK_END="# END dev-buffering-middleware"
MODE="${1:-add}"

if [[ -z "${TARGET_HOST:-}" ]]; then
  echo "TARGET_HOST is required (e.g., export TARGET_HOST=your.vps.host)" >&2
  exit 1
fi

backup_remote_file() {
  local ts
  ts="$(date -u +%Y%m%dT%H%M%SZ)"
  ssh -o StrictHostKeyChecking=no root@"$TARGET_HOST" \
    "test -f '${REMOTE_FILE}' && cp -a '${REMOTE_FILE}' '${REMOTE_FILE}.bak.${ts}' || true"
  echo "Backup created at ${REMOTE_FILE}.bak.${ts}"
}

append_block() {
  # Only append if not already present
  if ssh -o StrictHostKeyChecking=no root@"$TARGET_HOST" "grep -qF \"${MARK_START}\" '${REMOTE_FILE}'"; then
    echo "Block already present; skipping append."
    return 0
  fi

  backup_remote_file

  # Append YAML block with a high-priority router that targets ploy-api@consulcatalog
  # and adds a buffering middleware for request bodies.
  ssh -o StrictHostKeyChecking=no root@"$TARGET_HOST" bash -lc "cat >> '${REMOTE_FILE}' <<'YAML'
${MARK_START}
http:
  serversTransports:
    dev-slow:
      # Relaxed timeouts for long-running upstream builds (dev only)
      forwardingTimeouts:
        dialTimeout: 30s
        responseHeaderTimeout: 900s
        idleConnTimeout: 900s
        readIdleTimeout: 900s

  middlewares:
    dev-api-buffering:
      buffering:
        maxRequestBodyBytes: 33554432   # 32 MB
        memRequestBodyBytes: 1048576    # 1 MB

  services:
    dev-ploy-api-slow:
      loadBalancer:
        servers:
          - url: "http://127.0.0.1:8081"
      serversTransport: dev-slow

  routers:
    dev-ploy-api-buffered:
      rule: \"Host(\"api.dev.ployman.app\") && PathPrefix(`/v1`)\"
      entryPoints:
        - websecure
      service: dev-ploy-api-slow
      middlewares:
        - dev-api-buffering
      tls:
        certResolver: dev-wildcard
      priority: 1000
${MARK_END}
YAML"

  echo "Appended buffering middleware and router override to ${REMOTE_FILE}."
}

remove_block() {
  if ! ssh -o StrictHostKeyChecking=no root@"$TARGET_HOST" "grep -qF \"${MARK_START}\" '${REMOTE_FILE}'"; then
    echo "Block not found; nothing to remove."
    return 0
  fi
  backup_remote_file
  ssh -o StrictHostKeyChecking=no root@"$TARGET_HOST" \
    "awk 'BEGIN{skip=0} {if(\$0 ~ /${MARK_START//\//\/}/){skip=1} else if(\$0 ~ /${MARK_END//\//\/}/){skip=0; next} if(!skip) print}' '${REMOTE_FILE}' > '${REMOTE_FILE}.tmp' && mv '${REMOTE_FILE}.tmp' '${REMOTE_FILE}'"
  echo "Removed dev buffering block from ${REMOTE_FILE}."
}

reload_traefik() {
  # Prefer Nomad-managed detection first
  if ssh -o StrictHostKeyChecking=no root@"$TARGET_HOST" "su - ploy -c '/opt/hashicorp/bin/nomad-job-manager.sh status --job traefik-system' >/dev/null 2>&1"; then
    echo "Traefik is Nomad-managed (job: traefik-system); providers.file watch should reload automatically."
    return 0
  fi
  # Fallback: systemd-managed traefik
  if ssh -o StrictHostKeyChecking=no root@"$TARGET_HOST" "systemctl is-active --quiet traefik"; then
    echo "Restarting Traefik via systemd..."
    ssh -o StrictHostKeyChecking=no root@"$TARGET_HOST" "systemctl restart traefik && sleep 1 && systemctl status --no-pager traefik | sed -n '1,10p'" || true
    return 0
  fi
  echo "Traefik management not detected; relying on file provider watch."
}

case "$MODE" in
  add)
    append_block
    ;;
  remove)
    remove_block
    ;;
  *)
    echo "Unknown mode: $MODE (expected: add|remove)" >&2
    exit 2
    ;;
esac

reload_traefik
echo "Done."
