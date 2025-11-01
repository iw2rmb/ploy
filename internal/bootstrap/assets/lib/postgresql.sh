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

  # Derive the DSN after installation
  derive_postgresql_dsn
}

derive_postgresql_dsn() {
  log "deriving PostgreSQL DSN"

  # Try to determine the socket directory
  local socket_dir="/var/run/postgresql"
  if [[ ! -d "$socket_dir" ]]; then
    # Common alternative on RHEL/CentOS
    socket_dir="/tmp"
  fi

  # Determine the port
  local pg_port="5432"
  if command -v pg_config >/dev/null 2>&1; then
    local configured_port
    configured_port="$(sudo -u postgres psql -t -c 'SHOW port;' 2>/dev/null | tr -d '[:space:]')" || true
    if [[ -n "$configured_port" ]]; then
      pg_port="$configured_port"
    fi
  fi

  # Build DSN - prefer socket connection for local installation
  # Format: host=/var/run/postgresql user=ploy dbname=ploy sslmode=disable
  local derived_dsn="host=${socket_dir} user=ploy dbname=ploy sslmode=disable"

  # Test the connection
  if ! sudo -u ploy psql "${derived_dsn}" -c '\q' >/dev/null 2>&1; then
    log "socket connection failed, trying TCP with password"
    # Fall back to TCP connection with password
    derived_dsn="host=localhost port=${pg_port} user=ploy password=ploy dbname=ploy sslmode=disable"

    if ! PGPASSWORD=ploy psql "${derived_dsn}" -c '\q' >/dev/null 2>&1; then
      fail "failed to establish PostgreSQL connection with derived DSN"
    fi
  fi

  # Export the derived DSN so it's available to the ployd service configuration
  export PLOY_SERVER_PG_DSN="$derived_dsn"
  log "derived PostgreSQL DSN: ${derived_dsn}"
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
