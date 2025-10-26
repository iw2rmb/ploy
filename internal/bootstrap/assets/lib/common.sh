# shellcheck shell=bash
# Common logging, error handling, and safety utilities for bootstrap.sh

log() {
  printf '[bootstrap] %s\n' "$*" >&2
}

warn() {
  printf '[bootstrap][warn] %s\n' "$*" >&2
}

fail() {
  printf '[bootstrap][error] %s\n' "$*" >&2
  exit 1
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

require_root() {
  if [[ $(id -u) -ne 0 ]]; then
    fail "bootstrap requires root privileges"
  fi
}

stop_service_if_active() {
  local service="$1"
  if [[ -z "$service" ]]; then
    return
  fi
  if ! systemctl list-unit-files | grep -q "^${service}\\.service"; then
    return
  fi
  if systemctl is-active --quiet "$service"; then
    systemctl stop "$service"
    STOPPED_SERVICES+=("$service")
    log "stopped ${service} service to release bootstrap resources"
  fi
}

restart_stopped_services() {
  if [[ ${#STOPPED_SERVICES[@]} -eq 0 ]]; then
    return
  fi
  for (( idx=${#STOPPED_SERVICES[@]}-1; idx>=0; idx-- )); do
    local service="${STOPPED_SERVICES[idx]}"
    if [[ -z "$service" ]]; then
      continue
    fi
    if ! systemctl list-unit-files | grep -q "^${service}\\.service"; then
      continue
    fi
    if systemctl is-active --quiet "$service"; then
      continue
    fi
    if ! systemctl restart "$service"; then
      warn "failed to restart ${service} service; check logs"
    else
      log "restarted ${service} service"
    fi
  done
  STOPPED_SERVICES=()
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

prepare_workspace() {
  mkdir -p "$TMP_ROOT" "$DOWNLOAD_DIR"
  chmod 0755 "$WORKDIR" "$TMP_ROOT" "$DOWNLOAD_DIR"
}

check_disk_space() {
  local target total_kb required_gb available_gb
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
      local handled=0
      case "$port" in
        2379|2380)
          stop_service_if_active "etcd"
          handled=1
          ;;
        9094|9095)
          stop_service_if_active "ployd"
          handled=1
          ;;
      esac
      if [[ $handled -eq 1 ]]; then
        sleep 1
        if ss -tulpn 2>/dev/null | grep -q ":${port} " ; then
          fail "port ${port} currently in use; cannot continue bootstrap"
        fi
        log "port ${port} freed after restarting dependent services"
        continue
      fi
      fail "port ${port} currently in use; cannot continue bootstrap"
    fi
    log "port ${port} available"
  done
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
  if systemctl list-unit-files | grep -q '^ployd.service'; then
    if systemctl is-active --quiet ployd; then
      log "  ployd service: active"
    else
      warn "ployd service not active"
    fi
  fi
}
