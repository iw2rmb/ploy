#!/bin/sh
set -eu

cfg="${GARAGE_CONFIG_FILE:-/etc/garage.toml}"
key_id="${GARAGE_DEFAULT_ACCESS_KEY:-}"
secret_key="${GARAGE_DEFAULT_SECRET_KEY:-}"
bucket="${GARAGE_DEFAULT_BUCKET:-}"

if [ -z "$key_id" ] || [ -z "$secret_key" ] || [ -z "$bucket" ]; then
  echo "error: GARAGE_DEFAULT_ACCESS_KEY, GARAGE_DEFAULT_SECRET_KEY, and GARAGE_DEFAULT_BUCKET must be set" >&2
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

if ! /garage -c "$cfg" key info "$key_id" >/dev/null 2>&1; then
  /garage -c "$cfg" key import "$key_id" "$secret_key" -n "ploy-local-key" --yes >/dev/null
fi

if ! /garage -c "$cfg" bucket info "$bucket" >/dev/null 2>&1; then
  /garage -c "$cfg" bucket create "$bucket" >/dev/null
fi

/garage -c "$cfg" bucket allow --read --write --owner "$bucket" --key "$key_id" >/dev/null
echo "garage bootstrap complete: bucket=$bucket key=$key_id"
