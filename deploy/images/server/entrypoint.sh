#!/bin/sh
set -eu

if [ -n "${PLOY_RUNTIME_CA_CERT_PATH:-}" ] && [ -s "${PLOY_RUNTIME_CA_CERT_PATH}" ]; then
  export SSL_CERT_FILE="${PLOY_RUNTIME_CA_CERT_PATH}"
  export CURL_CA_BUNDLE="${PLOY_RUNTIME_CA_CERT_PATH}"
  export GIT_SSL_CAINFO="${PLOY_RUNTIME_CA_CERT_PATH}"
fi

exec /usr/local/bin/ployd "$@"
