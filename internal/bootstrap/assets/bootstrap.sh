#!/usr/bin/env bash
set -euo pipefail

# @@BOOTSTRAP_INLINE_LIBS@@

# Deployment bootstrap script used by `ploy cluster add --address <host>`.
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
declare -a STOPPED_SERVICES=()

CLUSTER_ID=""
NODE_ID=""
NODE_ADDRESS=""
BOOTSTRAP_PRIMARY="false"

PKG_MANAGER=""
PKG_UPDATE_CMD=()
PKG_INSTALL_CMD=()
ARCH=""
GO_ARCH=""
DOCKER_ARCH=""
IPFS_ARCH=""

# Reference globals so ShellCheck recognises their use across sourced modules.
: "${ETCD_VERSION}" "${IPFS_CLUSTER_VERSION}" "${DOCKER_VERSION}" "${DOCKER_CHANNEL}" "${GO_VERSION}"
: "${WORKDIR}" "${TMP_ROOT}" "${DOWNLOAD_DIR}" "${BIN_DIR}" "${SYSTEMD_DIR}"
: "${PKG_MANAGER}" "${ARCH}" "${GO_ARCH}" "${DOCKER_ARCH}" "${IPFS_ARCH}"
: "${PKG_UPDATE_CMD[*]:-}" "${PKG_INSTALL_CMD[*]:-}" "${STOPPED_SERVICES[*]:-}"

SCRIPT_PATH="${BASH_SOURCE[0]:-${0}}"
SCRIPT_DIR="$(cd -- "$(dirname -- "${SCRIPT_PATH}")" >/dev/null 2>&1 && pwd -P)"
LIB_DIR="${SCRIPT_DIR}/lib"
if [[ ! -d "${LIB_DIR}" ]]; then
  LIB_DIR="$(bootstrap_inline_libdir)"
fi

# shellcheck disable=SC1091
source "${LIB_DIR}/common.sh"
# shellcheck disable=SC1091
source "${LIB_DIR}/args.sh"
# shellcheck disable=SC1091
source "${LIB_DIR}/packages.sh"
# shellcheck disable=SC1091
source "${LIB_DIR}/etcd.sh"
# shellcheck disable=SC1091
source "${LIB_DIR}/ipfs.sh"
# shellcheck disable=SC1091
source "${LIB_DIR}/docker.sh"
# shellcheck disable=SC1091
source "${LIB_DIR}/ployd.sh"
# shellcheck disable=SC1091
source "${LIB_DIR}/tools.sh"
# shellcheck disable=SC1091
source "${LIB_DIR}/postgresql.sh"

trap restart_stopped_services EXIT

main() {
  parse_args "$@"

  if [[ -z "${CLUSTER_ID}" ]]; then
    if [[ "${BOOTSTRAP_PRIMARY}" == "true" ]]; then
      CLUSTER_ID="$(generate_cluster_id)"
      log "generated cluster id ${CLUSTER_ID}"
    else
      fail "cluster id required when joining an existing cluster"
    fi
  fi

  if [[ -z "${NODE_ID}" ]]; then
    NODE_ID="control"
  fi

  if [[ -z "${NODE_ADDRESS}" ]]; then
    NODE_ADDRESS="${CLUSTER_ID}"
  fi

  log "starting deployment bootstrap (script ${PLOY_BOOTSTRAP_VERSION:-unknown})"
  require_root
  detect_arch
  check_package_manager
  ensure_prerequisites
  ensure_sshd
  configure_sshd
  check_disk_space "$WORKDIR"
  check_required_ports
  prepare_workspace
  install_postgresql
  write_pki_certificates
  install_etcd
  install_ipfs_cluster
  install_docker
  configure_ployd_service
  bootstrap_control_plane_ca
  persist_cluster_metadata
  start_ployd_service
  install_go
  summarise_versions
  log "bootstrap complete"
}

if [[ "${BASH_SOURCE[0]:-}" == "${0}" || -z "${BASH_SOURCE[0]:-}" ]]; then
  main "$@"
fi
