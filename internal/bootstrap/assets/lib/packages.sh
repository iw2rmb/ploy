# shellcheck shell=bash
# shellcheck disable=SC2034

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
    apt|dnf|yum)
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

ensure_group() {
  local name="$1"
  if ! getent group "$name" >/dev/null 2>&1; then
    groupadd --system "$name"
    log "created system group $name"
  fi
}

ensure_system_user() {
  local name="$1"
  if id "$name" >/dev/null 2>&1; then
    return
  fi
  useradd --system --create-home --shell /bin/bash "$name"
  log "created system user $name"
}

ensure_sshd() {
  if ! command -v sshd >/dev/null 2>&1; then
    log "sshd not present; installing openssh-server"
    case "$PKG_MANAGER" in
      apt)
        install_package_set openssh-server
        ;;
      dnf|yum)
        install_package_set openssh-server
        ;;
      *)
        warn "unknown package manager; skipping sshd installation"
        ;;
    esac
  fi
  if ! command -v ssh >/dev/null 2>&1; then
    case "$PKG_MANAGER" in
      apt)
        install_package_set openssh-client
        ;;
      dnf|yum)
        install_package_set openssh-clients
        ;;
      *)
        warn "unknown package manager; skipping ssh client installation"
        ;;
    esac
  fi
}

configure_sshd() {
  local sshd_dir="/etc/ssh"
  local dropin_dir="${sshd_dir}/sshd_config.d"

  mkdir -p "$dropin_dir"
  cat >"${dropin_dir}/ploy.conf" <<'CONF'
PasswordAuthentication no
ChallengeResponseAuthentication no
KbdInteractiveAuthentication no
UsePAM yes
PermitRootLogin prohibit-password
AllowUsers root
LogLevel VERBOSE
AllowTcpForwarding yes
AllowAgentForwarding yes
GatewayPorts no
X11Forwarding no
ClientAliveInterval 300
ClientAliveCountMax 2
CONF

  if systemctl list-unit-files | grep -q '^ssh\.service'; then
    systemctl enable ssh >/dev/null 2>&1 || true
    systemctl restart ssh >/dev/null 2>&1 || true
  fi
  if systemctl list-unit-files | grep -q '^sshd\.service'; then
    systemctl enable sshd >/dev/null 2>&1 || true
    systemctl restart sshd >/dev/null 2>&1 || true
  fi
}
