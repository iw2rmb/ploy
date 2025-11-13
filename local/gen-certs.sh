#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<USAGE
Generate local mTLS bundle for Ploy (CA, server, admin, node).

Usage:
  $(basename "$0") [-n NODE_ID] [--force]

Options:
  -n, --node-id  Node ID for worker cert (default: node-1)
  -f, --force    Overwrite existing files
  -h, --help     Show this help
USAGE
}

NODE_ID="node-1"
FORCE=0
while [[ $# -gt 0 ]]; do
  case "$1" in
    -n|--node-id) NODE_ID="$2"; shift 2 ;;
    -f|--force)   FORCE=1; shift ;;
    -h|--help)    usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; usage; exit 2 ;;
  esac
done

DIR="$(cd "$(dirname "$0")" && pwd)"
PKI="$DIR/pki"
mkdir -p "$PKI"

need() { command -v "$1" >/dev/null 2>&1 || { echo "error: missing $1" >&2; exit 2; }; }
need openssl

maybe_write() {
  local path="$1"; shift
  if [[ -f "$path" && $FORCE -eq 0 ]]; then
    echo "skip (exists): $path"
    return 1
  fi
  return 0
}

echo "==> Generating CA"
if maybe_write "$PKI/ca.key"; then
  openssl ecparam -name prime256v1 -genkey -noout -out "$PKI/ca.key"
fi
if maybe_write "$PKI/ca.crt"; then
  openssl req -x509 -new -key "$PKI/ca.key" -days 3650 -sha256 \
    -subj "/CN=ploy-local-CA" -out "$PKI/ca.crt"
fi

echo "==> Generating server cert (localhost, server, 127.0.0.1)"
if maybe_write "$PKI/server.key"; then
  openssl ecparam -name prime256v1 -genkey -noout -out "$PKI/server.key"
fi
if maybe_write "$PKI/server.csr"; then
  openssl req -new -key "$PKI/server.key" -subj "/CN=ployd-local" -out "$PKI/server.csr"
fi
printf "subjectAltName=DNS:localhost,DNS:server,IP:127.0.0.1\nextendedKeyUsage=serverAuth\n" > "$PKI/server.ext"
if maybe_write "$PKI/server.crt"; then
  openssl x509 -req -in "$PKI/server.csr" -CA "$PKI/ca.crt" -CAkey "$PKI/ca.key" -CAcreateserial \
    -days 365 -sha256 -extfile "$PKI/server.ext" -out "$PKI/server.crt"
fi

echo "==> Generating admin client cert (cli-admin)"
if maybe_write "$PKI/admin.key"; then
  openssl ecparam -name prime256v1 -genkey -noout -out "$PKI/admin.key"
fi
if maybe_write "$PKI/admin.csr"; then
  openssl req -new -key "$PKI/admin.key" -subj "/CN=cli-admin-local/OU=Ploy role=cli-admin" -out "$PKI/admin.csr"
fi
printf "extendedKeyUsage=clientAuth\n" > "$PKI/client.ext"
if maybe_write "$PKI/admin.crt"; then
  openssl x509 -req -in "$PKI/admin.csr" -CA "$PKI/ca.crt" -CAkey "$PKI/ca.key" -CAcreateserial \
    -days 365 -sha256 -extfile "$PKI/client.ext" -out "$PKI/admin.crt"
fi

echo "==> Generating node client cert (worker, CN=node:${NODE_ID})"
if maybe_write "$PKI/node.key"; then
  openssl ecparam -name prime256v1 -genkey -noout -out "$PKI/node.key"
fi
if maybe_write "$PKI/node.csr"; then
  openssl req -new -key "$PKI/node.key" -subj "/CN=node:${NODE_ID}/OU=Ploy role=worker" -out "$PKI/node.csr"
fi
if maybe_write "$PKI/node.crt"; then
  openssl x509 -req -in "$PKI/node.csr" -CA "$PKI/ca.crt" -CAkey "$PKI/ca.key" -CAcreateserial \
    -days 365 -sha256 -extfile "$PKI/client.ext" -out "$PKI/node.crt"
fi

chmod 600 "$PKI"/*.key
chmod 644 "$PKI"/*.crt

cat <<SUMMARY

Certificates written to: $PKI
  CA:      ca.crt (key: ca.key)
  Server:  server.crt (key: server.key)
  Admin:   admin.crt (key: admin.key)
  Node:    node.crt (key: node.key, CN=node:${NODE_ID})

Next:
  docker compose -f "$DIR/docker-compose.yml" up -d
  curl --cacert "$PKI/ca.crt" --cert "$PKI/admin.crt" --key "$PKI/admin.key" https://localhost:8443/health
SUMMARY

