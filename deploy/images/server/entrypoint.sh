#!/bin/sh
set -eu

if [ -n "${PLOY_CA_CERTS:-}" ] && [ -s "${PLOY_CA_CERTS}" ]; then
  export SSL_CERT_FILE="${PLOY_CA_CERTS}"
  export CURL_CA_BUNDLE="${PLOY_CA_CERTS}"
  export GIT_SSL_CAINFO="${PLOY_CA_CERTS}"
fi

exec /usr/local/bin/ployd "$@"
