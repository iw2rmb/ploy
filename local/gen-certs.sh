#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<USAGE
Generate local CA certificate for Ploy (used for signing node certificates during bootstrap).

Usage:
  $(basename "$0") [--force]

Options:
  -f, --force    Overwrite existing files
  -h, --help     Show this help
USAGE
}

FORCE=0
while [[ $# -gt 0 ]]; do
  case "$1" in
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

chmod 600 "$PKI"/*.key
chmod 644 "$PKI"/*.crt

cat <<SUMMARY

CA certificate written to: $PKI
  CA:      ca.crt (key: ca.key)

Note: Server and nodes use bearer token authentication.
      The CA is used to sign node certificates during bootstrap.

Next:
  export PLOY_AUTH_SECRET=\$(openssl rand -base64 32)
  docker compose -f "$DIR/docker-compose.yml" up -d
  # Generate admin token using: ploy token create --role cli-admin
SUMMARY

