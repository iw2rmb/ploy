# shellcheck shell=bash

configure_ployd_service() {
  if [[ ! -x "${BIN_DIR}/ployd" ]]; then
    warn "ployd binary not found at ${BIN_DIR}/ployd; skipping daemon installation"
    return
  fi

  local config_path="${PLOYD_CONFIG_PATH:-/etc/ploy/ployd.yaml}"
  mkdir -p "$(dirname "$config_path")"
  mkdir -p /etc/systemd/system

  if [[ "${BOOTSTRAP_PRIMARY}" == "true" ]]; then
    rm -f "$config_path"
  fi

  if [[ ! -f "$config_path" ]]; then
    local raw_host="${NODE_ADDRESS:-127.0.0.1}"
    if [[ -z "$raw_host" ]]; then
      raw_host="127.0.0.1"
    fi
    local endpoint_host
    endpoint_host="$(format_host_for_url "$raw_host")"
    local endpoint="https://${endpoint_host}:8443"
    local metrics_listen="${PLOYD_METRICS_LISTEN:-127.0.0.1:9101}"
    local tls_enabled="${PLOYD_HTTP_TLS_ENABLED:-true}"
    local require_client_cert="${PLOYD_HTTP_TLS_REQUIRE_CLIENT_CERT:-false}"
    local desired_node_id="${PLOYD_NODE_ID:-$NODE_ID}"
    desired_node_id="$(sanitize_node_id "$desired_node_id")"
    if [[ -z "$desired_node_id" ]]; then
      desired_node_id="control"
    fi
    cat >"$config_path" <<YAML
http:
  listen: "${PLOYD_HTTP_LISTEN:-0.0.0.0:8443}"
  tls:
    enabled: ${tls_enabled}
    cert: "${PLOYD_TLS_CERT_PATH:-/etc/ploy/pki/node.pem}"
    key: "${PLOYD_TLS_KEY_PATH:-/etc/ploy/pki/node-key.pem}"
    client_ca: "${PLOYD_TLS_CLIENT_CA_PATH:-/etc/ploy/pki/control-plane-ca.pem}"
    require_client_cert: ${require_client_cert}
metrics:
  listen: "${metrics_listen}"
control_plane:
  endpoint: "${endpoint}"
  node_id: "${desired_node_id}"
  ca: "/etc/ploy/pki/control-plane-ca.pem"
  certificate: "/etc/ploy/pki/node.pem"
  key: "/etc/ploy/pki/node-key.pem"
runtime:
  plugins:
    - name: local
      module: builtin
YAML
    log "wrote default ployd configuration at $config_path"
  fi

  ensure_transfer_guard "$config_path"

  configure_ployd_environment "$raw_host"

  cat >"${SYSTEMD_DIR}/ployd.service" <<UNIT
[Unit]
Description=ploy node daemon
After=network-online.target docker.service
Wants=network-online.target

[Service]
Type=simple
ExecStart=${BIN_DIR}/ployd --config $config_path
Restart=always
RestartSec=5
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
UNIT
  systemctl daemon-reload
  systemctl enable ployd >/dev/null 2>&1 || true
}

configure_ployd_environment() {
  local host="$1"
  local env_dir="/etc/systemd/system/ployd.service.d"
  mkdir -p "$env_dir"

  local cache_dir="${PLOYD_CACHE_HOME:-/var/cache/ploy}"
  local home_dir="${PLOYD_HOME_DIR:-/root}"
  local ipfs_host="$(format_host_for_url "$host")"
  local ipfs_api="${PLOY_IPFS_CLUSTER_API:-http://${ipfs_host}:9094}"

  mkdir -p "$cache_dir"

  cat >"${env_dir}/env.conf" <<ENV
[Service]
Environment=PLOY_IPFS_CLUSTER_API=${ipfs_api}
Environment=HOME=${home_dir}
Environment=XDG_CACHE_HOME=${cache_dir}
ENV
}

start_ployd_service() {
  if [[ ! -f "${SYSTEMD_DIR}/ployd.service" ]]; then
    warn "ployd systemd unit missing; skipping start"
    return
  fi
  systemctl daemon-reload
  systemctl enable ployd >/dev/null 2>&1 || true
  systemctl restart ployd
  if ! systemctl is-active --quiet ployd; then
    warn "ployd service failed to start"
  else
    log "ployd service running"
  fi
}

bootstrap_control_plane_ca() {
  if [[ "${BOOTSTRAP_PRIMARY}" != "true" ]]; then
    return
  fi
  if [[ ! -x "${BIN_DIR}/ployd" ]]; then
    fail "ployd binary missing; cannot bootstrap control-plane CA"
  fi
  if [[ -z "${CLUSTER_ID}" ]]; then
    fail "cluster id not set; cannot bootstrap control-plane CA"
  fi
  local node_id="${NODE_ID:-control}"
  local node_addr="${NODE_ADDRESS:-${CLUSTER_ID}}"
  log "bootstrapping control-plane CA for cluster ${CLUSTER_ID}"
  if ! "${BIN_DIR}/ployd" bootstrap-ca --cluster-id "${CLUSTER_ID}" --node-id "${node_id}" --address "${node_addr}"; then
    fail "control-plane CA bootstrap failed"
  fi
}

persist_cluster_metadata() {
  if [[ -z "${CLUSTER_ID}" ]]; then
    return
  fi
  mkdir -p /etc/ploy
  printf '%s\n' "${CLUSTER_ID}" >/etc/ploy/cluster-id
}

ensure_transfer_guard() {
  local config_path="$1"
  local base_dir="${PLOY_TRANSFERS_BASE_DIR:-/var/lib/ploy/ssh-artifacts}"
  mkdir -p "${base_dir}/slots" "${base_dir}/logs"
  chmod 0750 "${base_dir}" "${base_dir}/slots"
  if ! getent group ploy-artifacts >/dev/null 2>&1; then
    groupadd --system ploy-artifacts
  fi
  chgrp ploy-artifacts "${base_dir}" || true
  chmod g+rx "${base_dir}" || true

  local wrapper="/usr/local/libexec/ploy-slot-guard"
  mkdir -p "$(dirname "$wrapper")"
  cat >"$wrapper" <<WRAPPER
#!/usr/bin/env bash
set -euo pipefail
slot="\${1:-}"
if [[ -z "\$slot" ]]; then
  echo "slot guard: slot id required" >&2
  exit 1
fi
exec ${BIN_DIR}/ployd slot-guard --config "${config_path}" --slot "\$slot"
WRAPPER
  chmod 0755 "$wrapper"
  log "configured ploy slot guard via $wrapper"
}

sanitize_node_id() {
  local value="${1,,}"
  value="${value//[^a-z0-9-]/-}"
  value="${value##-}"
  value="${value%%-}"
  printf '%s' "$value"
}

format_host_for_url() {
  local value="$1"
  if [[ -z "$value" ]]; then
    printf '127.0.0.1'
    return
  fi
  if [[ "$value" == \[* ]]; then
    printf '%s' "$value"
    return
  fi
  if [[ "$value" == *:* ]]; then
    printf '[%s]' "$value"
    return
  fi
  printf '%s' "$value"
}
