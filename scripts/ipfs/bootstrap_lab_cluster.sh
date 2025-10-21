#!/usr/bin/env bash
# bootstraps a disposable IPFS Cluster lab on a VPS for integration/E2E tests

set -euo pipefail

COMMAND="up"
PEER_COUNT="${PEER_COUNT:-3}"
STATE_DIR="${STATE_DIR:-$HOME/.ploy/ipfs-cluster-lab}"
STACK_NAME="${STACK_NAME:-ploy-ipfs-lab}"
KUBO_IMAGE="${KUBO_IMAGE:-ipfs/kubo:v0.38.1}"
CLUSTER_IMAGE="${CLUSTER_IMAGE:-ipfs/ipfs-cluster:v1.1.4}"
CLUSTER_SECRET_OVERRIDE="${CLUSTER_SECRET_OVERRIDE:-}"
PUBLIC_HOST="${PUBLIC_HOST:-}"
DESTROY_DATA="false"

CLUSTER_API_PORT="${CLUSTER_API_PORT:-9094}"
CLUSTER_RPC_PORT="${CLUSTER_RPC_PORT:-9096}"
CLUSTER_METRICS_PORT="${CLUSTER_METRICS_PORT:-9100}"
IPFS_API_PORT="${IPFS_API_PORT:-5001}"
IPFS_GATEWAY_PORT="${IPFS_GATEWAY_PORT:-8080}"
IPFS_SWARM_TCP_PORT="${IPFS_SWARM_TCP_PORT:-4001}"
IPFS_SWARM_UDP_PORT="${IPFS_SWARM_UDP_PORT:-4001}"
DOCKER_COMPOSE_PRINT=""

usage() {
  cat <<'EOF'
Usage:
  bootstrap_lab_cluster.sh [up|down] [options]

Commands:
  up (default)         Render docker-compose assets and start the lab cluster.
  down                 Stop the lab cluster (optionally prune persisted state).

Options:
  -n, --peer-count N   Number of IPFS+cluster peers to launch (default: 3, max: 7).
  -d, --state-dir DIR  Directory to store compose files and persistent state.
                       Defaults to $HOME/.ploy/ipfs-cluster-lab or STATE_DIR env.
      --cluster-secret HEX
                       Override the shared CLUSTER_SECRET (hex string).
      --public-host HOST
                       Hostname or IP advertised back to test runners; defaults
                       to the machine FQDN when unset.
      --destroy-data   When used with "down", wipes the compose/ state directory.
  -h, --help           Show this help and exit.

Environment overrides:
  PEER_COUNT, STATE_DIR, STACK_NAME, KUBO_IMAGE, CLUSTER_IMAGE,
  CLUSTER_API_PORT, CLUSTER_RPC_PORT, CLUSTER_METRICS_PORT,
  IPFS_API_PORT, IPFS_GATEWAY_PORT, IPFS_SWARM_TCP_PORT, IPFS_SWARM_UDP_PORT.
EOF
}

log() {
  printf '[bootstrap] %s\n' "$*" >&2
}

fail() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

ensure_dependencies() {
  command -v docker >/dev/null 2>&1 || fail "docker is required on the VPS lab host."
  if docker compose version >/dev/null 2>&1; then
    DOCKER_COMPOSE_BIN=(docker compose)
    DOCKER_COMPOSE_PRINT="docker compose"
  elif command -v docker-compose >/dev/null 2>&1; then
    DOCKER_COMPOSE_BIN=(docker-compose)
    DOCKER_COMPOSE_PRINT="docker-compose"
  else
    fail "docker compose plugin or docker-compose binary must be installed."
  fi
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

validate_inputs() {
  [[ "$PEER_COUNT" =~ ^[0-9]+$ ]] || fail "peer count must be numeric."
  (( PEER_COUNT >= 1 )) || fail "peer count must be at least 1."
  (( PEER_COUNT <= 7 )) || fail "peer count must not exceed 7."
  [[ -z "$CLUSTER_SECRET_OVERRIDE" || "$CLUSTER_SECRET_OVERRIDE" =~ ^[0-9a-fA-F]+$ ]] || \
    fail "cluster secret override must be a hex string."
}

ensure_state_dirs() {
  mkdir -p "$STATE_DIR/compose"
  for idx in $(seq 0 $((PEER_COUNT - 1))); do
    mkdir -p "$STATE_DIR/compose/ipfs${idx}" \
             "$STATE_DIR/compose/cluster${idx}" \
             "$STATE_DIR/compose/export${idx}"
  done
  STATE_DIR="$(cd "$STATE_DIR" && pwd)"
}

generate_secret() {
  local env_file secret
  env_file="$STATE_DIR/.env"
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
  cat >"$compose_file" <<EOF
version: "3.8"
name: ${STACK_NAME}
services:
EOF

  for idx in $(seq 0 $((PEER_COUNT - 1))); do
    cat >>"$compose_file" <<EOF
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
EOF
    if [[ "$idx" -eq 0 ]]; then
      cat >>"$compose_file" <<EOF
    ports:
      - "${IPFS_SWARM_TCP_PORT}:4001"
      - "${IPFS_SWARM_UDP_PORT}:4001/udp"
      - "${IPFS_API_PORT}:5001"
      - "${IPFS_GATEWAY_PORT}:8080"
EOF
    fi

    cat >>"$compose_file" <<EOF

  cluster${idx}:
    image: ${CLUSTER_IMAGE}
    container_name: ${STACK_NAME}_cluster${idx}
    depends_on:
      ipfs${idx}:
        condition: service_started
    restart: unless-stopped
    environment:
      CLUSTER_PEERNAME: cluster${idx}
      CLUSTER_SECRET: \${CLUSTER_SECRET}
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
EOF
    if [[ "$idx" -eq 0 ]]; then
      cat >>"$compose_file" <<EOF
    ports:
      - "${CLUSTER_API_PORT}:9094"
      - "${CLUSTER_RPC_PORT}:9096"
      - "${CLUSTER_METRICS_PORT}:9100"
EOF
    fi

    cat >>"$compose_file" <<EOF

EOF
  done

  cat >>"$compose_file" <<EOF
networks:
  ipfs-cluster-lab:
    name: ${STACK_NAME}_net
EOF
}

up_cluster() {
  ensure_dependencies
  validate_inputs
  ensure_state_dirs
  generate_secret
  render_compose_file

  log "starting ${PEER_COUNT}-peer cluster from ${STATE_DIR}"
  (
    cd "$STATE_DIR"
    "${DOCKER_COMPOSE_BIN[@]}" -f docker-compose.yaml --env-file .env up -d --remove-orphans
    "${DOCKER_COMPOSE_BIN[@]}" -f docker-compose.yaml ps
  )

  if [[ -z "$PUBLIC_HOST" ]]; then
    PUBLIC_HOST="$(hostname -f 2>/dev/null || hostname || echo "localhost")"
  fi

  cat <<EOF

Cluster bootstrap complete.

  Export for tests:
    export PLOY_IPFS_CLUSTER_API="http://${PUBLIC_HOST}:${CLUSTER_API_PORT}"

  Health checks:
    ${DOCKER_COMPOSE_PRINT} -f ${STATE_DIR}/docker-compose.yaml logs -f cluster0
    ${DOCKER_COMPOSE_PRINT} -f ${STATE_DIR}/docker-compose.yaml exec cluster0 ipfs-cluster-ctl peers ls
EOF
}

down_cluster() {
  ensure_dependencies
  validate_inputs
  if [[ -d "$STATE_DIR" ]]; then
    STATE_DIR="$(cd "$STATE_DIR" && pwd)"
  fi
  local compose_file="$STATE_DIR/docker-compose.yaml"
  if [[ ! -f "$compose_file" ]]; then
    log "nothing to stop; compose file not found at ${compose_file}"
    return 0
  fi
  log "stopping cluster defined in ${compose_file}"
  (
    cd "$STATE_DIR"
    "${DOCKER_COMPOSE_BIN[@]}" -f docker-compose.yaml --env-file .env down
  )
  if [[ "$DESTROY_DATA" == "true" ]]; then
    log "destroying compose state under ${STATE_DIR}/compose"
    rm -rf "$STATE_DIR/compose"
  fi
}

main() {
  parse_args "$@"
  case "$COMMAND" in
    up) up_cluster ;;
    down) down_cluster ;;
    *) fail "unsupported command: $COMMAND" ;;
  esac
}

main "$@"
