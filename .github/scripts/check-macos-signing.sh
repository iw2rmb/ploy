#!/bin/sh
set -eu

if [ "${1:-false}" = true ]; then
  exit 0
fi

# GoReleaser otherwise silently skips notarization when API credentials are empty.
: "${MACOS_SIGN_P12:?MACOS_SIGN_P12 is required for releases}"
: "${MACOS_SIGN_PASSWORD:?MACOS_SIGN_PASSWORD is required for releases}"
: "${MACOS_NOTARY_KEY:?MACOS_NOTARY_KEY is required for releases}"
: "${MACOS_NOTARY_KEY_ID:?MACOS_NOTARY_KEY_ID is required for releases}"
: "${MACOS_NOTARY_ISSUER_ID:?MACOS_NOTARY_ISSUER_ID is required for releases}"
