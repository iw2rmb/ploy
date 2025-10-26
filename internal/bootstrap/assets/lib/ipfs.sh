# shellcheck shell=bash

install_ipfs_cluster() {
  local current archive url
  if command -v ipfs-cluster-service >/dev/null 2>&1; then
    current="$(ipfs-cluster-service --version 2>/dev/null | awk '{print $3}' | tr -d 'v')"
    if [[ "$current" == "$IPFS_CLUSTER_VERSION" ]]; then
      log "IPFS Cluster already at $current; skipping"
      return
    fi
    warn "IPFS Cluster version $current detected, upgrading to $IPFS_CLUSTER_VERSION"
  else
    log "IPFS Cluster not present; installing $IPFS_CLUSTER_VERSION"
  fi
  archive="ipfs-cluster-service_v${IPFS_CLUSTER_VERSION}_${IPFS_ARCH}.tar.gz"
  url="https://dist.ipfs.tech/ipfs-cluster-service/v${IPFS_CLUSTER_VERSION}/${archive}"
  with_tmpdir install_ipfs_cluster_service_from_archive "$archive" "$url"
  install_ipfs_cluster_ctl
}

install_ipfs_cluster_service_from_archive() {
  local tmpdir="$1" archive="$2" url="$3"
  local extracted
  curl -fsSL "$url" -o "${tmpdir}/${archive}"
  tar -xzf "${tmpdir}/${archive}" -C "$tmpdir"
  extracted="${tmpdir}/ipfs-cluster-service"
  install -m0755 "${extracted}/ipfs-cluster-service" "${BIN_DIR}/ipfs-cluster-service"
  log "installed ipfs-cluster-service ${IPFS_CLUSTER_VERSION}"
}

install_ipfs_cluster_ctl() {
  local current archive url
  if command -v ipfs-cluster-ctl >/dev/null 2>&1; then
    current="$(ipfs-cluster-ctl --version 2>/dev/null | awk '{print $4}' | tr -d 'v')"
    if [[ "$current" == "$IPFS_CLUSTER_VERSION" ]]; then
      log "ipfs-cluster-ctl already at $current; skipping"
      return
    fi
    warn "ipfs-cluster-ctl version $current detected, upgrading to $IPFS_CLUSTER_VERSION"
  else
    log "ipfs-cluster-ctl not present; installing $IPFS_CLUSTER_VERSION"
  fi
  archive="ipfs-cluster-ctl_v${IPFS_CLUSTER_VERSION}_${IPFS_ARCH}.tar.gz"
  url="https://dist.ipfs.tech/ipfs-cluster-ctl/v${IPFS_CLUSTER_VERSION}/${archive}"
  with_tmpdir install_ipfs_cluster_ctl_from_archive "$archive" "$url"
}

install_ipfs_cluster_ctl_from_archive() {
  local tmpdir="$1" archive="$2" url="$3"
  local extracted
  curl -fsSL "$url" -o "${tmpdir}/${archive}"
  tar -xzf "${tmpdir}/${archive}" -C "$tmpdir"
  extracted="${tmpdir}/ipfs-cluster-ctl"
  install -m0755 "${extracted}/ipfs-cluster-ctl" "${BIN_DIR}/ipfs-cluster-ctl"
  log "installed ipfs-cluster-ctl ${IPFS_CLUSTER_VERSION}"
}
