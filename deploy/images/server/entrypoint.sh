#!/bin/sh
set -eu

# PLOY_CA_CERTS materializer: accepts a readable file path or inline PEM content.
# When the value is a file path, it is used directly. When the value is inline PEM,
# it is written to a temp file so SSL_CERT_FILE and friends can reference it.
if [ -n "${PLOY_CA_CERTS:-}" ]; then
  if [ -f "${PLOY_CA_CERTS}" ] && [ -s "${PLOY_CA_CERTS}" ]; then
    ploy_ca_file="${PLOY_CA_CERTS}"
  else
    ploy_ca_file="$(mktemp /tmp/ploy-ca-certs-XXXXXX.pem)"
    printf '%s\n' "${PLOY_CA_CERTS}" > "${ploy_ca_file}"
  fi
  if [ -s "${ploy_ca_file}" ]; then
    export SSL_CERT_FILE="${ploy_ca_file}"
    export CURL_CA_BUNDLE="${ploy_ca_file}"
    export GIT_SSL_CAINFO="${ploy_ca_file}"
  fi
fi

exec /usr/local/bin/ployd "$@"
