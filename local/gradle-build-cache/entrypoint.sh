#!/bin/sh
set -eu

DATA_DIR="${DATA_DIR:-/data}"
CONF_DIR="${CONF_DIR:-$DATA_DIR/conf}"
CONFIG_FILE="$CONF_DIR/config.yaml"
SEED_CONFIG="${SEED_CONFIG:-/seed/config.yaml}"

mkdir -p "$CONF_DIR"

if [ ! -f "$CONFIG_FILE" ]; then
  cp "$SEED_CONFIG" "$CONFIG_FILE"
  chmod u+rw "$CONFIG_FILE" 2>/dev/null || true
fi

# Always force anonymous read/write cache access for local development.
# This avoids ending up with an unusable cache when no users are defined.
sed -i -E 's/^([[:space:]]*anonymousLevel:).*/\1 readwrite/' "$CONFIG_FILE" 2>/dev/null || true

exec build-cache-node start -d "$DATA_DIR"
