#!/usr/bin/env bash
# shellcheck shell=bash

# install_ploy_ca_bundle imports CA certs provided via Hydra mounts into
# container trust stores and exports fallback CA env vars.
#
# Inputs:
#   - PLOY_CA_CERT_PATH (default: /etc/ploy/certs/ca.crt)
#   - mounted cert files under /etc/ploy/ca/*
# Optional controls:
#   - PLOY_CA_IMPORT_JAVA=1 to import certs into JVM cacerts via keytool.
#   - PLOY_CA_LOG_FILE=/path/to/log to append warnings/info.
install_ploy_ca_bundle() {
  local ca_path="${PLOY_CA_CERT_PATH:-/etc/ploy/certs/ca.crt}"
  local ca_dir="/etc/ploy/ca"
  local import_java="${PLOY_CA_IMPORT_JAVA:-0}"
  local log_file="${PLOY_CA_LOG_FILE:-}"

  local has_input=0
  if [[ -r "$ca_path" ]] && [[ -s "$ca_path" ]]; then
    has_input=1
  fi
  if compgen -G "${ca_dir}/*" >/dev/null; then
    has_input=1
  fi
  if [[ "$has_input" -eq 0 ]]; then
    return 0
  fi

  local tmp_dir
  tmp_dir="$(mktemp -d)"

  _ploy_ca_log() {
    local msg="$1"
    echo "$msg" >&2
    if [[ -n "$log_file" ]]; then
      echo "$msg" >>"$log_file"
    fi
  }

  _ploy_ca_sha256_from_cert_file() {
    local cert_file="$1"
    keytool -printcert -file "$cert_file" 2>/dev/null \
      | awk -F': ' '/^[[:space:]]*SHA256:/{gsub(":", "", $2); print tolower($2); exit}'
  }

  _ploy_ca_sha256_from_alias() {
    local alias_name="$1"
    keytool -list -v -cacerts -storepass changeit -alias "$alias_name" 2>/dev/null \
      | awk -F': ' '/^[[:space:]]*SHA256:/{gsub(":", "", $2); print tolower($2); exit}'
  }

  if [[ -r "$ca_path" ]] && [[ -s "$ca_path" ]]; then
    awk '/-----BEGIN CERTIFICATE-----/{n++} {print > (d"/cert" n ".crt")}' d="$tmp_dir" "$ca_path"
  fi
  if compgen -G "${ca_dir}/*" >/dev/null; then
    local mounted_ca
    for mounted_ca in "${ca_dir}"/*; do
      [[ -f "$mounted_ca" ]] || continue
      awk '/-----BEGIN CERTIFICATE-----/{n++} {print > (d"/cert" n ".crt")}' d="$tmp_dir" "$mounted_ca"
    done
  fi

  shopt -s nullglob
  local certs=("$tmp_dir"/*.crt)
  shopt -u nullglob
  if [[ ${#certs[@]} -eq 0 ]]; then
    rm -rf "$tmp_dir"
    return 0
  fi

  local sys_ca_dir=""
  if command -v update-ca-certificates >/dev/null 2>&1; then
    sys_ca_dir="/usr/local/share/ca-certificates/ploy"
    mkdir -p "$sys_ca_dir"
  fi

  local cert_path
  for cert_path in "${certs[@]}"; do
    if [[ "$import_java" == "1" ]] && command -v keytool >/dev/null 2>&1; then
      local alias_name cert_sha existing_sha should_import
      cert_sha="$(_ploy_ca_sha256_from_cert_file "$cert_path")"
      if [[ -n "$cert_sha" ]]; then
        alias_name="ploy_ca_sha256_${cert_sha}"
      else
        alias_name="ploy_ca_$(basename "$cert_path" .crt)"
      fi

      should_import=1
      if keytool -list -cacerts -storepass changeit -alias "$alias_name" >/dev/null 2>&1; then
        if [[ -n "$cert_sha" ]]; then
          existing_sha="$(_ploy_ca_sha256_from_alias "$alias_name")"
          if [[ -n "$existing_sha" && "$existing_sha" != "$cert_sha" ]]; then
            if ! keytool -delete -cacerts -storepass changeit -alias "$alias_name" >/dev/null 2>&1; then
              _ploy_ca_log "warning: keytool delete failed for alias ${alias_name}"
              should_import=0
            fi
          else
            should_import=0
          fi
        else
          # Fall back to positional alias semantics when fingerprinting is unavailable.
          should_import=0
        fi
      fi

      if [[ "$should_import" -eq 1 ]]; then
        keytool -importcert -noprompt -trustcacerts -cacerts -storepass changeit -alias "$alias_name" -file "$cert_path" >/dev/null 2>&1 || {
          _ploy_ca_log "warning: keytool import failed for ${cert_path}"
        }
      fi
    fi
    if [[ -n "$sys_ca_dir" ]]; then
      cp "$cert_path" "$sys_ca_dir/" || true
    fi
  done

  if [[ -n "$sys_ca_dir" ]]; then
    update-ca-certificates >/dev/null 2>&1 || true
  fi

  # Export fallback CA env vars to a merged bundle: system roots + extra certs.
  local fallback_dir="/tmp/ploy-extra-ca"
  local fallback_bundle="${fallback_dir}/ca-bundle.pem"
  mkdir -p "$fallback_dir"
  : >"$fallback_bundle"
  if [[ -r /etc/ssl/certs/ca-certificates.crt ]] && [[ -s /etc/ssl/certs/ca-certificates.crt ]]; then
    cat /etc/ssl/certs/ca-certificates.crt >>"$fallback_bundle"
    printf '\n' >>"$fallback_bundle"
  fi
  for cert_path in "${certs[@]}"; do
    cat "$cert_path" >>"$fallback_bundle"
    printf '\n' >>"$fallback_bundle"
  done
  if [[ -s "$fallback_bundle" ]]; then
    export SSL_CERT_FILE="$fallback_bundle"
    export CURL_CA_BUNDLE="$fallback_bundle"
    export GIT_SSL_CAINFO="$fallback_bundle"
    export CODEX_CA_CERTIFICATE="$fallback_bundle"
    export NODE_EXTRA_CA_CERTS="$fallback_bundle"
    export NPM_CONFIG_CAFILE="$fallback_bundle"
  fi

  rm -rf "$tmp_dir"
}
