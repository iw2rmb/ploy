#!/bin/sh
set -eu

# Hydra CA delivery: PLOY_CA_CERTS must be a readable file path pointing to a
# PEM bundle. Inline PEM content is no longer supported; use Hydra CA mount
# entries or provide a file path instead.
if [ -n "${PLOY_CA_CERTS:-}" ]; then
  if [ -r "${PLOY_CA_CERTS}" ] && [ -s "${PLOY_CA_CERTS}" ]; then
    export SSL_CERT_FILE="${PLOY_CA_CERTS}"
    export CURL_CA_BUNDLE="${PLOY_CA_CERTS}"
    export GIT_SSL_CAINFO="${PLOY_CA_CERTS}"
  else
    echo "warning: PLOY_CA_CERTS is set but is not a readable file; migrate to Hydra CA mount or provide a file path" >&2
  fi
fi

exec /usr/local/bin/ployd "$@"
