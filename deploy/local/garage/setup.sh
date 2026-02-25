#!/bin/sh
set -eu

cfg="${GARAGE_CONFIG_FILE:-/etc/garage.toml}"
key_id="${GARAGE_DEFAULT_ACCESS_KEY:-}"
secret_key="${GARAGE_DEFAULT_SECRET_KEY:-}"
bucket="${GARAGE_DEFAULT_BUCKET:-}"
registry_bucket="${GARAGE_REGISTRY_BUCKET:-}"
zone="${GARAGE_DEFAULT_ZONE:-local}"
capacity="${GARAGE_DEFAULT_CAPACITY:-1G}"

if [ -z "$key_id" ] || [ -z "$secret_key" ] || [ -z "$bucket" ]; then
  echo "error: GARAGE_DEFAULT_ACCESS_KEY, GARAGE_DEFAULT_SECRET_KEY, and GARAGE_DEFAULT_BUCKET must be set" >&2
  exit 1
fi

if ! printf '%s' "$key_id" | grep -Eq '^GK[0-9a-fA-F]{24}$'; then
  echo "error: GARAGE_DEFAULT_ACCESS_KEY must match ^GK[0-9a-fA-F]{24}$" >&2
  exit 1
fi

if ! printf '%s' "$secret_key" | grep -Eq '^[0-9a-fA-F]{64}$'; then
  echo "error: GARAGE_DEFAULT_SECRET_KEY must be 64 hex characters" >&2
  exit 1
fi

echo "waiting for garage status..."
i=0
while ! /garage -c "$cfg" status >/dev/null 2>&1; do
  i=$((i + 1))
  if [ "$i" -ge 60 ]; then
    echo "error: garage did not become ready in time" >&2
    exit 1
  fi
  sleep 1
done

layout_out="$(/garage -c "$cfg" layout show 2>/dev/null || true)"
if printf '%s\n' "$layout_out" | grep -q "No nodes currently have a role in the cluster."; then
  node_full="$(/garage -c "$cfg" node id -q | tr -d '\r')"
  node_id="${node_full%%@*}"
  current_version="$(printf '%s\n' "$layout_out" | sed -n 's/.*Current cluster layout version:[[:space:]]*//p' | head -n 1 | tr -d '\r[:space:]')"
  if [ -z "$current_version" ]; then
    current_version=0
  fi
  next_version=$((current_version + 1))

  /garage -c "$cfg" layout assign -z "$zone" -c "$capacity" "$node_id" >/dev/null
  /garage -c "$cfg" layout apply --version "$next_version" >/dev/null
fi

if ! /garage -c "$cfg" key info "$key_id" >/dev/null 2>&1; then
  /garage -c "$cfg" key import "$key_id" "$secret_key" -n "ploy-local-key" --yes >/dev/null
fi

if ! /garage -c "$cfg" bucket info "$bucket" >/dev/null 2>&1; then
  /garage -c "$cfg" bucket create "$bucket" >/dev/null
fi

/garage -c "$cfg" bucket allow --read --write --owner "$bucket" --key "$key_id" >/dev/null

if [ -n "$registry_bucket" ]; then
  if ! /garage -c "$cfg" bucket info "$registry_bucket" >/dev/null 2>&1; then
    /garage -c "$cfg" bucket create "$registry_bucket" >/dev/null
  fi
  /garage -c "$cfg" bucket allow --read --write --owner "$registry_bucket" --key "$key_id" >/dev/null
fi

echo "garage bootstrap complete: bucket=$bucket key=$key_id"
