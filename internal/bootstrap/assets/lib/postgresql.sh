# shellcheck shell=bash

install_postgresql() {
  if [[ "${PLOY_INSTALL_POSTGRESQL:-false}" != "true" ]]; then
    log "skipping PostgreSQL installation (not requested)"
    return
  fi

  log "installing PostgreSQL"

  case "$PKG_MANAGER" in
    apt)
      "${PKG_INSTALL_CMD[@]}" postgresql postgresql-contrib
      ;;
    yum|dnf)
      "${PKG_INSTALL_CMD[@]}" postgresql-server postgresql-contrib
      postgresql-setup --initdb || postgresql-setup initdb || true
      ;;
    *)
      fail "unsupported package manager for PostgreSQL: ${PKG_MANAGER}"
      ;;
  esac

  systemctl enable postgresql >/dev/null 2>&1 || true
  systemctl start postgresql

  if ! systemctl is-active --quiet postgresql; then
    fail "PostgreSQL service failed to start"
  fi

  log "PostgreSQL installed and running"

  # Create ploy user and database
  log "creating PostgreSQL user 'ploy' and database 'ploy'"

  sudo -u postgres psql -c "CREATE USER ploy WITH PASSWORD 'ploy';" 2>/dev/null || true
  sudo -u postgres psql -c "CREATE DATABASE ploy OWNER ploy;" 2>/dev/null || true
  sudo -u postgres psql -c "GRANT ALL PRIVILEGES ON DATABASE ploy TO ploy;" 2>/dev/null || true

  log "PostgreSQL user and database configured"
}

write_pki_certificates() {
  if [[ -z "${PLOY_CA_CERT_PEM:-}" ]]; then
    log "skipping PKI certificate setup (no CA provided)"
    return
  fi

  log "writing PKI certificates to /etc/ploy/pki/"

  local pki_dir="/etc/ploy/pki"
  mkdir -p "$pki_dir"
  chmod 0750 "$pki_dir"

  # Write CA certificate
  if [[ -n "${PLOY_CA_CERT_PEM:-}" ]]; then
    printf '%s' "$PLOY_CA_CERT_PEM" >"${pki_dir}/control-plane-ca.pem"
    chmod 0644 "${pki_dir}/control-plane-ca.pem"
    log "wrote CA certificate"
  fi

  # Write CA key (server only)
  if [[ -n "${PLOY_CA_KEY_PEM:-}" ]]; then
    printf '%s' "$PLOY_CA_KEY_PEM" >"${pki_dir}/ca-key.pem"
    chmod 0600 "${pki_dir}/ca-key.pem"
    log "wrote CA private key"
  fi

  # Write server certificate
  if [[ -n "${PLOY_SERVER_CERT_PEM:-}" ]]; then
    printf '%s' "$PLOY_SERVER_CERT_PEM" >"${pki_dir}/node.pem"
    chmod 0644 "${pki_dir}/node.pem"
    log "wrote server certificate"
  fi

  # Write server key
  if [[ -n "${PLOY_SERVER_KEY_PEM:-}" ]]; then
    printf '%s' "$PLOY_SERVER_KEY_PEM" >"${pki_dir}/node-key.pem"
    chmod 0600 "${pki_dir}/node-key.pem"
    log "wrote server private key"
  fi

  log "PKI certificates configured"
}
