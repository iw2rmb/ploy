# shellcheck shell=bash

install_etcd() {
  local current archive url
  if command -v etcd >/dev/null 2>&1; then
    current="$(etcd --version | awk '/etcd Version/ {print $3}' | tr -d 'v')"
    if [[ "$current" == "$ETCD_VERSION" ]] || [[ "$current" == 3.6.* ]]; then
      log "etcd already at $current; skipping"
      start_etcd_service
      return
    fi
    warn "etcd version $current detected, upgrading to $ETCD_VERSION"
  else
    log "etcd not present; installing $ETCD_VERSION"
  fi
  archive="etcd-v${ETCD_VERSION}-linux-${ARCH}.tar.gz"
  url="https://github.com/etcd-io/etcd/releases/download/v${ETCD_VERSION}/${archive}"
  with_tmpdir install_etcd_from_archive "$archive" "$url"
  configure_etcd_service
  start_etcd_service
}

install_etcd_from_archive() {
  local tmpdir="$1" archive="$2" url="$3"
  local extracted
  curl -fsSL "$url" -o "${tmpdir}/${archive}"
  tar -xzf "${tmpdir}/${archive}" -C "$tmpdir"
  extracted="${tmpdir}/etcd-v${ETCD_VERSION}-linux-${ARCH}"
  install -m0755 "${extracted}/etcd" "${BIN_DIR}/etcd"
  install -m0755 "${extracted}/etcdctl" "${BIN_DIR}/etcdctl"
  log "installed etcd ${ETCD_VERSION} to ${BIN_DIR}"
}

configure_etcd_service() {
  local data_dir="/var/lib/etcd"
  mkdir -p "$data_dir"
  cat >/etc/systemd/system/etcd.service <<'UNIT'
[Unit]
Description=etcd key-value store
Documentation=https://etcd.io/docs/
After=network-online.target
Wants=network-online.target

[Service]
Type=notify
ExecStart=/usr/local/bin/etcd \
  --name=ploy-bootstrap \
  --data-dir=/var/lib/etcd \
  --listen-client-urls=http://127.0.0.1:2379 \
  --advertise-client-urls=http://127.0.0.1:2379 \
  --listen-peer-urls=http://127.0.0.1:2380 \
  --initial-advertise-peer-urls=http://127.0.0.1:2380 \
  --initial-cluster=ploy-bootstrap=http://127.0.0.1:2380 \
  --initial-cluster-state=new \
  --initial-cluster-token=ploy-bootstrap \
  --max-wals=5
Restart=on-failure
RestartSec=3
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
UNIT
  systemctl daemon-reload
  systemctl enable etcd >/dev/null 2>&1 || true
}

start_etcd_service() {
  systemctl restart etcd
  if ! systemctl is-active --quiet etcd; then
    fail "failed to start etcd service"
  fi
  log "etcd service running"
}
