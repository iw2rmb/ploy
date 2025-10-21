#!/usr/bin/env bash
# bootstraps the VPS lab IPFS Cluster over SSH for integration/E2E tests

set -euo pipefail

COMMAND="up"
PEER_COUNT="${PEER_COUNT:-3}"
STATE_DIR="${STATE_DIR:-$HOME/.ploy/ipfs-cluster-lab}"
REMOTE_STATE_DIR="${REMOTE_STATE_DIR:-/opt/ploy/ipfs-cluster-lab}"
STACK_NAME="${STACK_NAME:-ploy-ipfs-lab}"
KUBO_IMAGE="${KUBO_IMAGE:-ipfs/kubo:v0.38.1}"
CLUSTER_IMAGE="${CLUSTER_IMAGE:-ipfs/ipfs-cluster:v1.1.4}"
CLUSTER_SECRET_OVERRIDE="${CLUSTER_SECRET_OVERRIDE:-}"
PUBLIC_HOST="${PUBLIC_HOST:-}"
DESTROY_DATA="false"
SSH_HOST="${SSH_HOST:-root@45.9.42.212}"
SSH_PORT="${SSH_PORT:-22}"
SSH_IDENTITY="${SSH_IDENTITY:-}"

CLUSTER_API_PORT="${CLUSTER_API_PORT:-9094}"
CLUSTER_RPC_PORT="${CLUSTER_RPC_PORT:-9096}"
CLUSTER_METRICS_PORT="${CLUSTER_METRICS_PORT:-9100}"
IPFS_API_PORT="${IPFS_API_PORT:-5001}"
IPFS_GATEWAY_PORT="${IPFS_GATEWAY_PORT:-8080}"
IPFS_SWARM_TCP_PORT="${IPFS_SWARM_TCP_PORT:-4001}"
IPFS_SWARM_UDP_PORT="${IPFS_SWARM_UDP_PORT:-4001}"
REMOTE_COMPOSE_CMD=""

usage() {
  cat <<'EOS'
Usage:
  bootstrap_lab_cluster.sh [up|down] [options]

Commands:
  up (default)         Provision the IPFS lab cluster on the remote VPS via SSH.
  down                 Stop the cluster on the remote VPS (optionally purge data).

Options:
  -n, --peer-count N        Number of IPFS+cluster peers to launch (default: 3, max: 7).
  -d, --state-dir DIR       Local working directory for compose assets (default: $HOME/.ploy/ipfs-cluster-lab).
      --remote-state-dir DIR  Remote directory holding compose assets (default: /opt/ploy/ipfs-cluster-lab).
      --ssh-host HOST        SSH destination (default: root@45.9.42.212).
      --ssh-port PORT        SSH port (default: 22).
      --ssh-identity PATH    SSH identity file.
      --cluster-secret HEX   Override the shared CLUSTER_SECRET (hex string).
      --public-host HOST     Override the host/IP advertised back to tests.
      --destroy-data         When used with "down", removes the remote compose/state directory.
  -h, --help                Show this help and exit.
EOS
}

log() {
  printf '[bootstrap] %s\n' "$*" >&2
}

fail() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      up|start)
        COMMAND="up"
        shift
        ;;
      down|destroy|stop)
        COMMAND="down"
        shift
        ;;
      -n|--peer-count)
        [[ $# -lt 2 ]] && fail "missing value for $1"
        PEER_COUNT="$2"
        shift 2
        ;;
      -d|--state-dir)
        [[ $# -lt 2 ]] && fail "missing value for $1"
        STATE_DIR="$2"
        shift 2
        ;;
      --remote-state-dir)
        [[ $# -lt 2 ]] && fail "missing value for $1"
        REMOTE_STATE_DIR="$2"
        shift 2
        ;;
      --ssh-host)
        [[ $# -lt 2 ]] && fail "missing value for $1"
        SSH_HOST="$2"
        shift 2
        ;;
      --ssh-port)
        [[ $# -lt 2 ]] && fail "missing value for $1"
        SSH_PORT="$2"
        shift 2
        ;;
      --ssh-identity)
        [[ $# -lt 2 ]] && fail "missing value for $1"
        SSH_IDENTITY="$2"
        shift 2
        ;;
      --cluster-secret)
        [[ $# -lt 2 ]] && fail "missing value for $1"
        CLUSTER_SECRET_OVERRIDE="$2"
        shift 2
        ;;
      --public-host)
        [[ $# -lt 2 ]] && fail "missing value for $1"
        PUBLIC_HOST="$2"
        shift 2
        ;;
      --destroy-data)
        DESTROY_DATA="true"
        shift
        ;;
      -h|--help)
        usage
        exit 0
        ;;
      *)
        usage
        fail "unknown argument: $1"
        ;;
    esac
  done
}

build_ssh_args() {
  SSH_ARGS=(-p "$SSH_PORT" -o BatchMode=yes)
  if [[ -n "$SSH_IDENTITY" ]]; then
    SSH_ARGS+=(-i "$SSH_IDENTITY")
  fi
}

remote_exec() {
  # shellcheck disable=SC2029
  ssh "${SSH_ARGS[@]}" "$SSH_HOST" "$@"
}

remote_exec_bash() {
  local cmd="$1"
  local escaped=${cmd//\'/\'"\'"\'}
  remote_exec "bash -lc '$escaped'"
}

ensure_local_dependencies() {
  command -v ssh >/dev/null 2>&1 || fail "ssh is required to reach the VPS lab nodes."
  command -v scp >/dev/null 2>&1 || fail "scp is required to copy compose assets."
  command -v openssl >/dev/null 2>&1 || command -v dd >/dev/null 2>&1 || \
    fail "either openssl or dd must be available to generate secrets."
}

validate_inputs() {
  [[ "$PEER_COUNT" =~ ^[0-9]+$ ]] || fail "peer count must be numeric."
  (( PEER_COUNT >= 1 )) || fail "peer count must be at least 1."
  (( PEER_COUNT <= 7 )) || fail "peer count must not exceed 7."
  [[ -z "$CLUSTER_SECRET_OVERRIDE" || "$CLUSTER_SECRET_OVERRIDE" =~ ^[0-9a-fA-F]+$ ]] || \
    fail "cluster secret override must be a hex string."
}

ensure_local_state_dirs() {
  mkdir -p "$STATE_DIR/compose"
  for idx in $(seq 0 $((PEER_COUNT - 1))); do
    mkdir -p "$STATE_DIR/compose/ipfs${idx}" \
             "$STATE_DIR/compose/cluster${idx}" \
             "$STATE_DIR/compose/export${idx}"
  done
  STATE_DIR="$(cd "$STATE_DIR" && pwd)"
}

ensure_remote_state_dirs() {
  remote_exec mkdir -p "${REMOTE_STATE_DIR}/compose"
  for idx in $(seq 0 $((PEER_COUNT - 1))); do
    remote_exec mkdir -p \
      "${REMOTE_STATE_DIR}/compose/ipfs${idx}" \
      "${REMOTE_STATE_DIR}/compose/cluster${idx}" \
      "${REMOTE_STATE_DIR}/compose/export${idx}"
  done
}

copy_assets_to_remote() {
  local scp_args=()
  if [[ -n "$SSH_IDENTITY" ]]; then
    scp_args+=(-i "$SSH_IDENTITY")
  fi
  scp_args+=(-P "$SSH_PORT")
  scp "${scp_args[@]}" "$STATE_DIR/.env" "$SSH_HOST:${REMOTE_STATE_DIR}/.env"
  scp "${scp_args[@]}" "$STATE_DIR/docker-compose.yaml" "$SSH_HOST:${REMOTE_STATE_DIR}/docker-compose.yaml"
}

generate_secret() {
  local env_file="$STATE_DIR/.env"
  local secret=""
  if [[ -n "$CLUSTER_SECRET_OVERRIDE" ]]; then
    secret="$CLUSTER_SECRET_OVERRIDE"
  elif [[ -f "$env_file" ]]; then
    secret="$(grep -E '^CLUSTER_SECRET=' "$env_file" | head -n1 | cut -d= -f2-)"
  fi
  if [[ -z "$secret" ]]; then
    if command -v openssl >/dev/null 2>&1; then
      secret="$(openssl rand -hex 32)"
    else
      secret="$(dd if=/dev/urandom bs=32 count=1 2>/dev/null | hexdump -v -e '1/1 "%02x"')"
    fi
  fi
  printf 'CLUSTER_SECRET=%s\n' "$secret" >"$env_file"
}

render_compose_file() {
  local compose_file="$STATE_DIR/docker-compose.yaml"
  local cluster_secret_placeholder="\${CLUSTER_SECRET}"
  cat >"$compose_file" <<EOF_COMPOSE
services:
EOF_COMPOSE

  for idx in $(seq 0 $((PEER_COUNT - 1))); do
    cat >>"$compose_file" <<EOF_SERVICE
  ipfs${idx}:
    image: ${KUBO_IMAGE}
    container_name: ${STACK_NAME}_ipfs${idx}
    restart: unless-stopped
    command:
      - daemon
      - --migrate=true
    environment:
      IPFS_PROFILE: server
    volumes:
      - ./compose/ipfs${idx}:/data/ipfs
      - ./compose/export${idx}:/export
    networks:
      - ipfs-cluster-lab
EOF_SERVICE
    if [[ "$idx" -eq 0 ]]; then
      cat >>"$compose_file" <<EOF_IPFS_PORTS
    ports:
      - "${IPFS_SWARM_TCP_PORT}:4001"
      - "${IPFS_SWARM_UDP_PORT}:4001/udp"
      - "${IPFS_API_PORT}:5001"
      - "${IPFS_GATEWAY_PORT}:8080"
EOF_IPFS_PORTS
    fi

    cat >>"$compose_file" <<EOF_CLUSTER

  cluster${idx}:
    image: ${CLUSTER_IMAGE}
    container_name: ${STACK_NAME}_cluster${idx}
    depends_on:
      ipfs${idx}:
        condition: service_started
    restart: unless-stopped
    environment:
      CLUSTER_PEERNAME: cluster${idx}
      CLUSTER_SECRET: ${cluster_secret_placeholder}
      CLUSTER_IPFSHTTP_NODEMULTIADDRESS: /dns4/ipfs${idx}/tcp/5001
      CLUSTER_CRDT_TRUSTEDPEERS: '*'
      CLUSTER_MONITORPINGINTERVAL: 2s
      CLUSTER_RESTAPI_HTTPLISTENMULTIADDRESS: /ip4/0.0.0.0/tcp/9094
      CLUSTER_RPC_LISTENMULTIADDRESS: /ip4/0.0.0.0/tcp/9096
      CLUSTER_METRICS_LISTENMULTIADDRESS: /ip4/0.0.0.0/tcp/9100
    volumes:
      - ./compose/cluster${idx}:/data/ipfs-cluster
    networks:
      - ipfs-cluster-lab
EOF_CLUSTER
    if [[ "$idx" -eq 0 ]]; then
      cat >>"$compose_file" <<EOF_CLUSTER_PORTS
    ports:
      - "${CLUSTER_API_PORT}:9094"
      - "${CLUSTER_RPC_PORT}:9096"
      - "${CLUSTER_METRICS_PORT}:9100"
EOF_CLUSTER_PORTS
    fi

    cat >>"$compose_file" <<'EOF_SEPARATOR'

EOF_SEPARATOR
  done

  cat >>"$compose_file" <<EOF_NETWORKS
networks:
  ipfs-cluster-lab:
    name: ${STACK_NAME}_net
EOF_NETWORKS
}

ensure_remote_docker() {
  if remote_exec_bash 'command -v docker >/dev/null 2>&1'; then
    if ! remote_exec_bash 'docker info >/dev/null 2>&1'; then
      remote_exec_bash 'systemctl start docker >/dev/null 2>&1 || service docker start >/dev/null 2>&1' || \
        fail "docker is installed on ${SSH_HOST} but could not be started."
    fi
    return
  fi

  log "docker not detected on ${SSH_HOST}; attempting installation"
  remote_exec_bash 'set -euo pipefail; export DEBIAN_FRONTEND=noninteractive; if command -v apt-get >/dev/null 2>&1; then apt-get update && apt-get install -y docker.io docker-compose; elif command -v yum >/dev/null 2>&1; then yum install -y docker docker-compose; else exit 90; fi; if command -v systemctl >/dev/null 2>&1; then systemctl enable docker >/dev/null 2>&1 || true; systemctl start docker >/dev/null 2>&1 || true; else service docker start >/dev/null 2>&1 || true; fi'
  if ! remote_exec_bash 'docker info >/dev/null 2>&1'; then
    fail "docker installation on ${SSH_HOST} appears to have failed; please install manually and retry."
  fi
}

ensure_remote_compose() {
  if remote_exec_bash 'command -v docker-compose >/dev/null 2>&1'; then
    remote_exec_bash 'docker-compose version >/dev/null 2>&1' || \
      fail "docker-compose binary found on ${SSH_HOST} but failed to run."
    REMOTE_COMPOSE_CMD="docker-compose"
    return
  fi
  log "docker compose not detected on ${SSH_HOST}; attempting installation"
  remote_exec_bash 'set -euo pipefail; export DEBIAN_FRONTEND=noninteractive; if command -v apt-get >/dev/null 2>&1; then apt-get update && apt-get install -y docker-compose; elif command -v yum >/dev/null 2>&1; then yum install -y docker-compose; else exit 90; fi'
  if ! remote_exec_bash 'command -v docker-compose >/dev/null 2>&1'; then
    fail "docker-compose must be installed on ${SSH_HOST}; install it manually and retry."
  fi
  REMOTE_COMPOSE_CMD="docker-compose"
}

ensure_remote_prereqs() {
  ensure_remote_docker
  ensure_remote_compose
}

up_cluster() {
  ensure_local_dependencies
  build_ssh_args
  validate_inputs
  ensure_local_state_dirs
  generate_secret
  render_compose_file
  ensure_remote_state_dirs
  copy_assets_to_remote
  ensure_remote_prereqs

  log "ensuring clean slate on ${SSH_HOST}:${REMOTE_STATE_DIR}"
  remote_exec_bash "cd \"$REMOTE_STATE_DIR\" && ${REMOTE_COMPOSE_CMD} -f docker-compose.yaml --env-file \"$REMOTE_STATE_DIR/.env\" down --remove-orphans || true"

  log "starting ${PEER_COUNT}-peer cluster on ${SSH_HOST}:${REMOTE_STATE_DIR}"
  log "remote compose command: cd \"$REMOTE_STATE_DIR\" && ${REMOTE_COMPOSE_CMD} -f docker-compose.yaml --env-file \"$REMOTE_STATE_DIR/.env\" up -d --remove-orphans"
  remote_exec_bash "cd \"$REMOTE_STATE_DIR\" && ${REMOTE_COMPOSE_CMD} -f docker-compose.yaml --env-file \"$REMOTE_STATE_DIR/.env\" up -d --remove-orphans"
  remote_exec_bash "cd \"$REMOTE_STATE_DIR\" && ${REMOTE_COMPOSE_CMD} -f docker-compose.yaml ps"

  if [[ -z "$PUBLIC_HOST" ]]; then
    PUBLIC_HOST="${SSH_HOST##*@}"
  fi

  cat <<EOF_OUT

Cluster bootstrap complete on ${SSH_HOST}.

  Export for tests:
    export PLOY_IPFS_CLUSTER_API="http://${PUBLIC_HOST}:${CLUSTER_API_PORT}"

  Remote health checks:
    ssh ${SSH_HOST} ${REMOTE_COMPOSE_CMD} -f ${REMOTE_STATE_DIR}/docker-compose.yaml logs -f cluster0
    ssh ${SSH_HOST} ${REMOTE_COMPOSE_CMD} -f ${REMOTE_STATE_DIR}/docker-compose.yaml exec cluster0 ipfs-cluster-ctl peers ls
EOF_OUT
}

down_cluster() {
  ensure_local_dependencies
  build_ssh_args
  ensure_remote_compose
  log "stopping cluster on ${SSH_HOST}:${REMOTE_STATE_DIR}"
  remote_exec_bash "cd \"$REMOTE_STATE_DIR\" && ${REMOTE_COMPOSE_CMD} -f docker-compose.yaml --env-file \"$REMOTE_STATE_DIR/.env\" down"
  if [[ "$DESTROY_DATA" == "true" ]]; then
    remote_exec rm -rf "$REMOTE_STATE_DIR"
  fi
}

main() {
  if [[ $# -gt 0 ]]; then
    parse_args "$@"
  fi
  case "$COMMAND" in
    up) up_cluster "$@" ;;
    down) down_cluster "$@" ;;
    *) fail "unsupported command: $COMMAND" ;;
  esac
}

main "$@"
