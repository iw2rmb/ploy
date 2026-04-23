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
  local java_marker_file="/etc/ploy/certs/.java_cacerts_marker"

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

  _ploy_ca_hash_stdin() {
    if command -v sha256sum >/dev/null 2>&1; then
      sha256sum | awk '{print tolower($1)}'
      return
    fi
    if command -v shasum >/dev/null 2>&1; then
      shasum -a 256 | awk '{print tolower($1)}'
      return
    fi
    cksum | awk '{print tolower($1) "-" tolower($2)}'
  }

  _ploy_ca_hash_file() {
    local file_path="$1"
    if command -v sha256sum >/dev/null 2>&1; then
      sha256sum "$file_path" | awk '{print tolower($1)}'
      return
    fi
    if command -v shasum >/dev/null 2>&1; then
      shasum -a 256 "$file_path" | awk '{print tolower($1)}'
      return
    fi
    cksum "$file_path" | awk '{print tolower($1) "-" tolower($2)}'
  }

  _ploy_ca_cert_fingerprint() {
    local cert_file="$1"
    local cert_sha
    cert_sha="$(_ploy_ca_hash_file "$cert_file" 2>/dev/null || true)"
    printf '%s' "$cert_sha"
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

  local -a unique_certs=()
  local -a unique_cert_fps=()
  local -A seen_cert_fps=()
  local cert_path cert_sha
  for cert_path in "${certs[@]}"; do
    cert_sha="$(_ploy_ca_cert_fingerprint "$cert_path")"
    if [[ -z "$cert_sha" ]]; then
      _ploy_ca_log "warning: unable to fingerprint certificate ${cert_path}; skipping"
      continue
    fi
    if [[ -n "${seen_cert_fps[$cert_sha]:-}" ]]; then
      continue
    fi
    seen_cert_fps["$cert_sha"]=1
    unique_certs+=("$cert_path")
    unique_cert_fps+=("$cert_sha")
  done
  if [[ ${#unique_certs[@]} -eq 0 ]]; then
    rm -rf "$tmp_dir"
    return 0
  fi

  if [[ "$import_java" == "1" ]] && command -v keytool >/dev/null 2>&1; then
    local java_marker_hash java_marker_current cert_fp alias_name import_failed i
    java_marker_hash="$(printf '%s\n' "${unique_cert_fps[@]}" | sort | _ploy_ca_hash_stdin 2>/dev/null || true)"
    java_marker_current=""
    if [[ -r "$java_marker_file" ]]; then
      java_marker_current="$(tr -d '[:space:]' < "$java_marker_file")"
    fi

    if [[ -n "$java_marker_hash" && "$java_marker_current" != "$java_marker_hash" ]]; then
      import_failed=0
      for i in "${!unique_certs[@]}"; do
        cert_path="${unique_certs[$i]}"
        cert_fp="${unique_cert_fps[$i]}"
        alias_name="ploy_ca_sha256_${cert_fp}"
        if ! keytool -importcert -noprompt -trustcacerts -cacerts -storepass changeit -alias "$alias_name" -file "$cert_path" >/dev/null 2>&1; then
          if ! keytool -list -cacerts -storepass changeit -alias "$alias_name" >/dev/null 2>&1; then
            _ploy_ca_log "warning: keytool import failed for ${cert_path}"
            import_failed=1
          fi
        fi
      done

      if [[ "$import_failed" -eq 0 ]]; then
        mkdir -p "$(dirname "$java_marker_file")"
        printf '%s\n' "$java_marker_hash" >"$java_marker_file" 2>/dev/null || true
      fi
    fi
  fi

  local sys_ca_dir=""
  if command -v update-ca-certificates >/dev/null 2>&1; then
    sys_ca_dir="/usr/local/share/ca-certificates/ploy"
    mkdir -p "$sys_ca_dir"
  fi

  if [[ -n "$sys_ca_dir" ]]; then
    shopt -s nullglob
    local existing_sys_certs=("$sys_ca_dir"/*.crt)
    shopt -u nullglob
    if [[ ${#existing_sys_certs[@]} -gt 0 ]]; then
      rm -f "${existing_sys_certs[@]}" || true
    fi
  fi

  if [[ -n "$sys_ca_dir" ]]; then
    cp -- "${unique_certs[@]}" "$sys_ca_dir/" || true
  fi

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
  for cert_path in "${unique_certs[@]}"; do
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
