#!/usr/bin/env bash
set -euo pipefail

# Deployment bootstrap script used by `ploy deploy bootstrap`.
# The script converges host dependencies to the required versions and
# performs preflight checks before installing software.

ETCD_VERSION="3.6.0"
IPFS_CLUSTER_VERSION="1.1.4"
DOCKER_VERSION="28.2.2"
DOCKER_CHANNEL="28"
GO_VERSION="1.25.2"

WORKDIR="${PLOY_WORKDIR:-/var/lib/ploy}"
TMP_ROOT="${WORKDIR}/tmp"
DOWNLOAD_DIR="${WORKDIR}/downloads"
BIN_DIR="/usr/local/bin"
SYSTEMD_DIR="/etc/systemd/system"

log() {
  printf '[bootstrap] %s\n' "$*" >&2
}

warn() {
  printf '[bootstrap][warn] %s\n' "$*" >&2
}

version_ge() {
  local lhs="$1"
  local rhs="$2"
  if [[ -z "$lhs" || -z "$rhs" ]]; then
    return 1
  fi
  local max
  max="$(printf '%s\n%s\n' "$lhs" "$rhs" | sort -V | tail -n1)"
  [[ "$max" == "$lhs" ]]
}

fail() {
  printf '[bootstrap][error] %s\n' "$*" >&2
  exit 1
}

require_root() {
  if [[ $(id -u) -ne 0 ]]; then
    fail "bootstrap requires root privileges"
  fi
}

detect_arch() {
  local machine shell_arch go_arch docker_arch ipfs_arch
  machine="$(uname -m)"
  case "$machine" in
    x86_64|amd64)
      shell_arch="amd64"
      go_arch="amd64"
      docker_arch="x86_64"
      ipfs_arch="linux-amd64"
      ;;
    aarch64|arm64)
      shell_arch="arm64"
      go_arch="arm64"
      docker_arch="aarch64"
      ipfs_arch="linux-arm64"
      ;;
    *)
      fail "unsupported architecture: $machine"
      ;;
  esac
  ARCH="$shell_arch"
  GO_ARCH="$go_arch"
  DOCKER_ARCH="$docker_arch"
  IPFS_ARCH="$ipfs_arch"
}

check_package_manager() {
  if command -v apt-get >/dev/null 2>&1; then
    PKG_MANAGER="apt"
    PKG_UPDATE_CMD=(apt-get update -y)
    PKG_INSTALL_CMD=(apt-get install -y)
    return
  fi
  if command -v dnf >/dev/null 2>&1; then
    PKG_MANAGER="dnf"
    PKG_UPDATE_CMD=(dnf makecache -y)
    PKG_INSTALL_CMD=(dnf install -y)
    return
  fi
  if command -v yum >/dev/null 2>&1; then
    PKG_MANAGER="yum"
    PKG_UPDATE_CMD=(yum makecache -y)
    PKG_INSTALL_CMD=(yum install -y)
    return
  fi
  fail "no supported package manager (apt, dnf, yum) detected"
}

install_package_set() {
  local packages=("$@")
  if [[ ${#packages[@]} -eq 0 ]]; then
    return
  fi
  case "$PKG_MANAGER" in
    apt)
      "${PKG_UPDATE_CMD[@]}"
      "${PKG_INSTALL_CMD[@]}" "${packages[@]}"
      ;;
    dnf|yum)
      "${PKG_UPDATE_CMD[@]}"
      "${PKG_INSTALL_CMD[@]}" "${packages[@]}"
      ;;
    *)
      fail "package manager not initialised"
      ;;
  esac
}

ensure_prerequisites() {
  local required_commands=(curl tar gzip sha256sum systemctl ss jq sed awk)
  local missing=()
  for cmd in "${required_commands[@]}"; do
    if ! command -v "$cmd" >/dev/null 2>&1; then
      missing+=("$cmd")
    fi
  done
  if [[ ${#missing[@]} -gt 0 ]]; then
    log "installing prerequisite packages: ${missing[*]}"
    case "$PKG_MANAGER" in
      apt)
        install_package_set curl tar gzip coreutils systemd iproute2 iptables jq sed
        ;;
      dnf|yum)
        install_package_set curl tar gzip coreutils systemd iproute iptables jq sed
        ;;
      *)
        fail "cannot install prerequisites without a supported package manager"
        ;;
    esac
  fi
}

check_disk_space() {
  local target path total_kb required_gb available_gb
  target="${1:-$WORKDIR}"
  required_gb="${PLOY_MIN_DISK_GB:-40}"
  if [[ ! -d "$target" ]]; then
    mkdir -p "$target"
  fi
  total_kb="$(df -Pk "$target" | tail -1 | awk '{print $4}')"
  available_gb=$(( (total_kb + 1023) / 1024 / 1024 ))
  if (( available_gb < required_gb )); then
    fail "insufficient disk space at $target: need ${required_gb}GiB, have ${available_gb}GiB"
  fi
  log "disk space check passed: ${available_gb}GiB available at $target"
}

check_required_ports() {
  local ports raw port
  raw="${PLOY_REQUIRED_PORTS:-}"
  if [[ -z "$raw" ]]; then
    warn "no required ports defined; skipping port availability checks"
    return
  fi
  IFS=' ' read -r -a ports <<<"$raw"
  for port in "${ports[@]}"; do
    [[ -z "$port" ]] && continue
    if ss -tulpn 2>/dev/null | grep -q ":${port} " ; then
      fail "port ${port} currently in use; cannot continue bootstrap"
    fi
    log "port ${port} available"
  done
}

prepare_workspace() {
  mkdir -p "$TMP_ROOT" "$DOWNLOAD_DIR"
  chmod 0755 "$WORKDIR" "$TMP_ROOT" "$DOWNLOAD_DIR"
}

with_tmpdir() {
  local fn="$1"
  shift
  local tmpdir
  tmpdir="$(mktemp -d "${TMP_ROOT}/bootstrap.XXXXXX")"
  trap 'rm -rf "$tmpdir"' RETURN
  "$fn" "$tmpdir" "$@"
  trap - RETURN
  rm -rf "$tmpdir"
}

ensure_group() {
  local name="$1"
  if ! getent group "$name" >/dev/null 2>&1; then
    groupadd --system "$name"
    log "created system group $name"
  fi
}

install_etcd() {
  local current tmp archive url
  if command -v etcd >/dev/null 2>&1; then
    current="$(etcd --version | awk '/etcd Version/ {print $3}' | tr -d 'v')"
    if [[ "$current" == "$ETCD_VERSION" ]] || [[ "$current" == 3.6.* ]]; then
      log "etcd already at $current; skipping"
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

install_go() {
  local current archive url
  if command -v go >/dev/null 2>&1; then
    current="$(go version | awk '{print $3}' | tr -d 'go')"
    if [[ "$current" == "$GO_VERSION" ]] || [[ "$current" == 1.25.* ]]; then
      log "Go already at $current; skipping"
      return
    fi
    warn "Go version $current detected, upgrading to $GO_VERSION"
  else
    log "Go not present; installing $GO_VERSION"
  fi
  archive="go${GO_VERSION}.linux-${GO_ARCH}.tar.gz"
  url="https://go.dev/dl/${archive}"
  with_tmpdir install_go_from_archive "$archive" "$url"
}

install_go_from_archive() {
  local tmpdir="$1" archive="$2" url="$3"
  curl -fsSL "$url" -o "${tmpdir}/${archive}"
  rm -rf /usr/local/go
  tar -xzf "${tmpdir}/${archive}" -C /usr/local
  ln -sf /usr/local/go/bin/go "${BIN_DIR}/go"
  log "installed Go ${GO_VERSION}"
}

install_docker() {
  local current archive url extracted
  if command -v docker >/dev/null 2>&1; then
    current="$(docker version --format '{{.Server.Version}}' 2>/dev/null || docker --version | awk '{print $3}' | tr -d ',')"
    if version_ge "$current" "${DOCKER_VERSION}"; then
      log "Docker already at $current; ensuring service configuration"
      configure_docker_service
      return
    fi
    warn "Docker version $current detected, upgrading to ${DOCKER_VERSION}"
  else
    log "Docker not present; installing ${DOCKER_VERSION}"
  fi
  archive="docker-${DOCKER_VERSION}.tgz"
  url="https://download.docker.com/linux/static/stable/${DOCKER_ARCH}/${archive}"
  with_tmpdir install_docker_from_archive "$archive" "$url"
  configure_docker_service
}

install_docker_from_archive() {
  local tmpdir="$1" archive="$2" url="$3"
  local extracted
  curl -fsSL "$url" -o "${tmpdir}/${archive}"
  tar -xzf "${tmpdir}/${archive}" -C "$tmpdir"
  extracted="${tmpdir}/docker"
  install -m0755 "${extracted}/docker" "${BIN_DIR}/docker"
  install -m0755 "${extracted}/dockerd" "${BIN_DIR}/dockerd"
  install -m0755 "${extracted}/containerd"* "${BIN_DIR}/"
  install -m0755 "${extracted}/runc" "${BIN_DIR}/runc"
  install -m0755 "${extracted}/ctr" "${BIN_DIR}/ctr"
  install -m0755 "${extracted}/docker-proxy" "${BIN_DIR}/docker-proxy"
  install -m0755 "${extracted}/docker-init" "${BIN_DIR}/docker-init"
  log "installed Docker ${DOCKER_VERSION} binaries"
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

configure_docker_service() {
  ensure_group docker
  mkdir -p /etc/docker
  if [[ ! -f /etc/docker/daemon.json ]]; then
    cat >/etc/docker/daemon.json <<'JSON'
{
  "log-driver": "json-file",
  "log-opts": {
    "max-size": "10m",
    "max-file": "3"
  },
  "exec-opts": ["native.cgroupdriver=systemd"],
  "storage-driver": "overlay2"
}
JSON
    log "wrote /etc/docker/daemon.json"
  fi
  cat >"${SYSTEMD_DIR}/docker.service" <<'UNIT'
[Unit]
Description=Docker Application Container Engine
Documentation=https://docs.docker.com
After=network-online.target firewalld.service
Wants=network-online.target

[Service]
Type=notify
ExecStart=/usr/local/bin/dockerd --config-file=/etc/docker/daemon.json
ExecReload=/bin/kill -s HUP $MAINPID
LimitNOFILE=1048576
LimitNPROC=1048576
LimitCORE=infinity
TasksMax=infinity
TimeoutStartSec=0
Delegate=yes
KillMode=process

[Install]
WantedBy=multi-user.target
UNIT
  systemctl daemon-reload
  systemctl reset-failed docker.service || true
  systemctl enable docker
  systemctl restart docker
  log "docker service enabled and restarted"
}

summarise_versions() {
  log "Bootstrap versions:"
  if command -v etcd >/dev/null 2>&1; then
    log "  etcd: $(etcd --version | awk '/etcd Version/ {print $3}')"
  else
    warn "etcd binary not found in PATH"
  fi
  if command -v ipfs-cluster-service >/dev/null 2>&1; then
    log "  ipfs-cluster-service: $(ipfs-cluster-service --version 2>/dev/null | awk '{print $3}')"
  else
    warn "ipfs-cluster-service binary not found in PATH"
  fi
  if command -v docker >/dev/null 2>&1; then
    log "  docker: $(docker version --format '{{.Server.Version}}' 2>/dev/null || docker --version | awk '{print $3}')"
  else
    warn "docker binary not found in PATH"
  fi
  if command -v go >/dev/null 2>&1; then
    log "  go: $(go version | awk '{print $3}')"
  else
    warn "go binary not found in PATH"
  fi
  if systemctl is-active --quiet etcd; then
    log "  etcd service: active"
  else
    warn "etcd service not active"
  fi
}

main() {
  log "starting deployment bootstrap (script ${PLOY_BOOTSTRAP_VERSION:-unknown})"
  require_root
  detect_arch
  check_package_manager
  ensure_prerequisites
  check_disk_space "$WORKDIR"
  check_required_ports
  prepare_workspace
  install_etcd
  install_ipfs_cluster
  install_docker
  install_go
  summarise_versions
  log "bootstrap complete"
}

main "$@"
