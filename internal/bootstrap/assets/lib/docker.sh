# shellcheck shell=bash

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

  # Non-interactive Docker Hub login when creds present
  if [[ -n "${DOCKERHUB_USERNAME:-}" && -n "${DOCKERHUB_PAT:-}" ]]; then
    if printf '%s' "${DOCKERHUB_PAT}" | ${BIN_DIR}/docker login -u "${DOCKERHUB_USERNAME}" --password-stdin >/dev/null 2>&1; then
      log "docker hub login configured for ${DOCKERHUB_USERNAME}"
    else
      warn "docker hub login failed; continuing without saved auth"
    fi
  fi
}
