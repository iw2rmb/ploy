#!/bin/sh
set -eu

# Hydra CA delivery: CA cert is written to /etc/ploy/pki/ca.crt during server
# bootstrap. Set TLS env vars so that curl, git, and Go net/http honour it.
CA_CERT="/etc/ploy/pki/ca.crt"
if [ -r "$CA_CERT" ] && [ -s "$CA_CERT" ]; then
  export SSL_CERT_FILE="$CA_CERT"
  export CURL_CA_BUNDLE="$CA_CERT"
  export GIT_SSL_CAINFO="$CA_CERT"
fi

exec /usr/local/bin/ployd "$@"
