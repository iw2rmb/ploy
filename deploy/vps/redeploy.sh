#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

PYTHON_BIN="${PYTHON_BIN:-python3}"
PLATFORM="${PLATFORM:-linux/amd64}"
PLOY_CONFIG_HOME="${PLOY_CONFIG_HOME:-$ROOT_DIR/deploy/vps/cli}"
AUTH_SECRET_PATH="${AUTH_SECRET_PATH:-$ROOT_DIR/deploy/vps/auth-secret.txt}"
PLOY_VPS_DB_DSN="${PLOY_VPS_DB_DSN:-postgres://s_v.v.kovalev@host.docker.internal/ploy}"
PLOY_VPS_CLUSTER_ID="${PLOY_VPS_CLUSTER_ID:-local}"
PLOY_VPS_NODE1_ID="${PLOY_VPS_NODE1_ID:-local1}"
PLOY_VPS_NODE2_ID="${PLOY_VPS_NODE2_ID:-local2}"
PLOY_VPS_NODE1_NAME="${PLOY_VPS_NODE1_NAME:-local-node-0001}"
PLOY_VPS_NODE2_NAME="${PLOY_VPS_NODE2_NAME:-local-node-0002}"
PLOY_VPS_NODE1_CONCURRENCY="${PLOY_VPS_NODE1_CONCURRENCY:-1}"
PLOY_VPS_NODE2_CONCURRENCY="${PLOY_VPS_NODE2_CONCURRENCY:-1}"
PLOY_VPS_WORKDIR_ROOT="${PLOY_VPS_WORKDIR_ROOT:-/var/tmp/ploy-vps}"
PLOY_SERVER_PORT="${PLOY_SERVER_PORT:-8080}"
PLOY_REGISTRY_PORT="${PLOY_REGISTRY_PORT:-5000}"
PLOY_CONTAINER_SOCKET_PATH="${PLOY_CONTAINER_SOCKET_PATH:-/var/run/docker.sock}"
PLOY_SKIP_BUILD="${PLOY_SKIP_BUILD:-0}"
PLOY_DOCKER_NETWORK="${PLOY_DOCKER_NETWORK:-ploy-vps_default}"
PLOY_CONTAINER_REGISTRY="${PLOY_CONTAINER_REGISTRY:-127.0.0.1:${PLOY_REGISTRY_PORT}/ploy}"

SSH_HOST="10.120.34.186"
SSH_USER="${SSH_USER:-s_v.v.kovalev}"
SSH_PORT="${SSH_PORT:-22}"
SSH_IDENTITY="${SSH_IDENTITY:-$HOME/.ssh/id_rsa}"
REMOTE_DIR="${REMOTE_DIR:-/opt/ploy-vps}"
DROP_DB=0
LOW_DISK_MODE=0

log() {
  echo "[$(date -u +%H:%M:%S)] $*"
}

need() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "error: missing dependency: $1" >&2
    exit 1
  fi
}

run_as_root() {
  if [[ "$(id -u)" -eq 0 ]]; then
    "$@"
    return
  fi

  if command -v sudo >/dev/null 2>&1 && sudo -n true >/dev/null 2>&1; then
    sudo "$@"
    return
  fi

  return 1
}

is_true() {
  case "${1:-}" in
    1|true|TRUE|True|yes|YES|Yes|on|ON|On) return 0 ;;
    *) return 1 ;;
  esac
}

docker_image_exists() {
  docker image inspect "$1" >/dev/null 2>&1
}

maybe_run_buildx_load() {
  local dockerfile="$1"
  local context="$2"
  local tag="$3"
  shift 3

  if (( LOW_DISK_MODE )) && docker_image_exists "$tag"; then
    log "Reusing existing local image ${tag}"
    return
  fi

  run_buildx_load "$dockerfile" "$context" "$tag" "$@"
}

maybe_build_context_image() {
  local ref="$1"
  shift

  if (( LOW_DISK_MODE )) && docker_image_exists "$ref"; then
    log "Reusing existing local image ${ref}"
    return
  fi

  docker buildx build "$@" -t "$ref" --load
}

maybe_pull_image() {
  local ref="$1"

  if (( LOW_DISK_MODE )) && docker_image_exists "$ref"; then
    log "Reusing existing local image ${ref}"
    return
  fi

  docker pull "$ref" >/dev/null
}

wait_for_docker_engine_ready() {
  local i
  for i in {1..30}; do
    if docker info >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done

  echo "error: docker engine did not become ready in time after CA installation" >&2
  exit 1
}

configure_colima_registry_ca() {
  local ca_path="$1"
  local colima_bin="${COLIMA_BIN:-}"

  if [[ -z "$colima_bin" ]]; then
    colima_bin="$(command -v colima || true)"
  fi
  if [[ -z "$colima_bin" ]]; then
    echo "error: docker context is colima but colima CLI is not installed or not visible in PATH" >&2
    exit 1
  fi

  log "Installing PLOY_CA_CERTS into colima docker registry trust..."
  "$colima_bin" ssh -- sudo mkdir -p \
    /etc/docker/certs.d/docker.io \
    /etc/docker/certs.d/registry-1.docker.io \
    /etc/docker/certs.d/auth.docker.io \
    /etc/docker/certs.d/index.docker.io
  cat "$ca_path" | "$colima_bin" ssh -- sudo tee /etc/docker/certs.d/docker.io/ca.crt >/dev/null
  cat "$ca_path" | "$colima_bin" ssh -- sudo tee /etc/docker/certs.d/registry-1.docker.io/ca.crt >/dev/null
  cat "$ca_path" | "$colima_bin" ssh -- sudo tee /etc/docker/certs.d/auth.docker.io/ca.crt >/dev/null
  cat "$ca_path" | "$colima_bin" ssh -- sudo tee /etc/docker/certs.d/index.docker.io/ca.crt >/dev/null
  "$colima_bin" ssh -- sudo chmod 0644 \
    /etc/docker/certs.d/docker.io/ca.crt \
    /etc/docker/certs.d/registry-1.docker.io/ca.crt \
    /etc/docker/certs.d/auth.docker.io/ca.crt \
    /etc/docker/certs.d/index.docker.io/ca.crt
  "$colima_bin" ssh -- sudo mkdir -p /usr/local/share/ca-certificates/ploy
  cat "$ca_path" | "$colima_bin" ssh -- sudo tee /usr/local/share/ca-certificates/ploy/ploy-ca.crt >/dev/null
  "$colima_bin" ssh -- sudo update-ca-certificates >/dev/null
  "$colima_bin" ssh -- sudo systemctl restart docker
}

configure_linux_registry_ca() {
  local ca_path="$1"

  log "Installing PLOY_CA_CERTS into local docker registry trust..."
  if ! run_as_root mkdir -p \
    /etc/docker/certs.d/docker.io \
    /etc/docker/certs.d/registry-1.docker.io \
    /etc/docker/certs.d/auth.docker.io \
    /etc/docker/certs.d/index.docker.io; then
    echo "error: cannot create /etc/docker/certs.d (root privileges are required)" >&2
    exit 1
  fi
  if ! run_as_root install -m 0644 "$ca_path" /etc/docker/certs.d/docker.io/ca.crt; then
    echo "error: cannot install CA bundle to /etc/docker/certs.d/docker.io/ca.crt" >&2
    exit 1
  fi
  if ! run_as_root install -m 0644 "$ca_path" /etc/docker/certs.d/registry-1.docker.io/ca.crt; then
    echo "error: cannot install CA bundle to /etc/docker/certs.d/registry-1.docker.io/ca.crt" >&2
    exit 1
  fi
  if ! run_as_root install -m 0644 "$ca_path" /etc/docker/certs.d/auth.docker.io/ca.crt; then
    echo "error: cannot install CA bundle to /etc/docker/certs.d/auth.docker.io/ca.crt" >&2
    exit 1
  fi
  if ! run_as_root install -m 0644 "$ca_path" /etc/docker/certs.d/index.docker.io/ca.crt; then
    echo "error: cannot install CA bundle to /etc/docker/certs.d/index.docker.io/ca.crt" >&2
    exit 1
  fi
  if ! run_as_root mkdir -p /usr/local/share/ca-certificates/ploy; then
    echo "error: cannot create /usr/local/share/ca-certificates/ploy" >&2
    exit 1
  fi
  if ! run_as_root install -m 0644 "$ca_path" /usr/local/share/ca-certificates/ploy/ploy-ca.crt; then
    echo "error: cannot install CA bundle to /usr/local/share/ca-certificates/ploy/ploy-ca.crt" >&2
    exit 1
  fi
  if command -v update-ca-certificates >/dev/null 2>&1; then
    if ! run_as_root update-ca-certificates >/dev/null; then
      echo "error: failed to update system CA trust via update-ca-certificates" >&2
      exit 1
    fi
  fi

  if command -v systemctl >/dev/null 2>&1; then
    if ! run_as_root systemctl restart docker; then
      echo "error: failed to restart docker via systemctl after CA install" >&2
      exit 1
    fi
    return
  fi
  if command -v service >/dev/null 2>&1; then
    if ! run_as_root service docker restart; then
      echo "error: failed to restart docker via service after CA install" >&2
      exit 1
    fi
    return
  fi

  echo "error: installed CA bundle but could not restart docker daemon (no systemctl/service)" >&2
  exit 1
}

configure_docker_registry_ca_if_needed() {
  local context os_name engine_name

  if [[ -z "${PLOY_CA_CERTS:-}" ]]; then
    return 0
  fi

  context="$(docker context show 2>/dev/null || true)"
  os_name="$(uname -s)"
  engine_name="$(docker info --format '{{.Name}}' 2>/dev/null || true)"

  if [[ "$context" == "colima" || "$engine_name" == "colima" ]]; then
    configure_colima_registry_ca "$PLOY_CA_CERTS"
    wait_for_docker_engine_ready
    return
  fi

  if [[ "$os_name" == "Linux" ]]; then
    configure_linux_registry_ca "$PLOY_CA_CERTS"
    wait_for_docker_engine_ready
    return
  fi

  echo "error: PLOY_CA_CERTS auto-install is not supported for docker context '${context}' (engine='${engine_name}') on ${os_name}" >&2
  echo "error: configure docker daemon trust for docker.io manually, then rerun deploy/vps/redeploy.sh" >&2
  exit 1
}

usage() {
  cat <<'USAGE'
Usage: ./deploy/vps/redeploy.sh --address <host-or-ip> [flags]

Flags:
  --address <host>     SSH target hostname or IP
  --user <user>        SSH username (default: root)
  --identity <path>    SSH private key path (default: ~/.ssh/id_rsa)
  --ssh-port <port>    SSH port (default: 22)
  --remote-dir <path>  Remote install directory (default: /opt/ploy-vps)
  --drop-db            Drop and recreate the ploy database before deploy
  --low-disk           Reuse existing local artifacts when possible and stream upload payloads

Environment:
  PLOY_VPS_DB_DSN          Required PostgreSQL DSN reachable from the VPS host and containers
  PLOY_VPS_CLUSTER_ID      Cluster identifier written into tokens and descriptor (default: local)
  PLOY_VPS_NODE1_ID        First node ID (default: local1)
  PLOY_VPS_NODE2_ID        Second node ID (default: local2)
  PLOY_VPS_WORKDIR_ROOT    Host workspace root mounted into both node containers (default: /var/tmp/ploy-vps)
  PLOY_SERVER_PORT         Host server port published by compose (default: 8080)
  PLOY_REGISTRY_PORT       Host registry port published by compose (default: 5000)
  PLOY_CONTAINER_REGISTRY  Registry prefix used for workflow images (default: 127.0.0.1:<port>/ploy)
  PLOY_SKIP_BUILD          Set to true-like to reuse existing local build artifacts
USAGE
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --address)
        SSH_HOST="$2"
        shift
        ;;
      --user)
        SSH_USER="$2"
        shift
        ;;
      --identity)
        SSH_IDENTITY="$2"
        shift
        ;;
      --ssh-port)
        SSH_PORT="$2"
        shift
        ;;
      --remote-dir)
        REMOTE_DIR="$2"
        shift
        ;;
      --drop-db)
        DROP_DB=1
        ;;
      --low-disk)
        LOW_DISK_MODE=1
        ;;
      -h|--help)
        usage
        exit 0
        ;;
      *)
        echo "error: unknown argument: $1" >&2
        usage >&2
        exit 1
        ;;
    esac
    shift
  done
}

prepare_ca_certs_path() {
  local raw="${PLOY_CA_CERTS:-}"
  if [[ -z "$raw" ]]; then
    PLOY_CA_CERTS=""
    return
  fi
  if [[ "$raw" != /* ]]; then
    raw="$ROOT_DIR/$raw"
  fi
  if [[ ! -f "$raw" ]]; then
    echo "error: PLOY_CA_CERTS file not found: $raw" >&2
    exit 1
  fi
  if [[ ! -s "$raw" ]]; then
    echo "error: PLOY_CA_CERTS file is empty: $raw" >&2
    exit 1
  fi
  PLOY_CA_CERTS="$raw"
}

discover_mig_dirs() {
  local root_migs="deploy/images/migs"
  local root_mig="deploy/images/mig"
  {
    if [[ -d "$root_migs" ]]; then
      find "$root_migs" -mindepth 1 -maxdepth 1 -type d -print | while read -r d; do
        name="$(basename "$d")"
        printf 'migs/%s\n' "$name"
      done
    fi
    if [[ -d "$root_mig" ]]; then
      find "$root_mig" -mindepth 1 -maxdepth 1 -type d -print | while read -r d; do
        name="$(basename "$d")"
        printf 'mig/%s\n' "$name"
      done
    fi
  } | sort
}

mig_repo_name() {
  local entry="$1"
  local name="${entry##*/}"
  case "$name" in
    mig-*) echo "migs-${name#mig-}" ;;
    *) echo "$name" ;;
  esac
}

run_buildx_load() {
  local dockerfile="$1"
  local context="$2"
  local tag="$3"
  shift 3

  local -a extra_args=()
  if (( $# > 0 )); then
    extra_args=("$@")
  fi
  if (( ${#extra_args[@]} > 0 )); then
    docker buildx build \
      --platform "$PLATFORM" \
      "${extra_args[@]}" \
      --provenance=false --sbom=false --pull --progress=plain \
      -f "$dockerfile" \
      -t "$tag" \
      --load \
      "$context"
  else
    docker buildx build \
      --platform "$PLATFORM" \
      --provenance=false --sbom=false --pull --progress=plain \
      -f "$dockerfile" \
      -t "$tag" \
      --load \
      "$context"
  fi
}

build_runtime_images() {
  local -a extra_args=()
  if [[ -n "${PLOY_CA_CERTS:-}" ]]; then
    extra_args+=(--secret "id=ploy_ca_bundle,src=${PLOY_CA_CERTS}")
  fi

  log "Building runtime images..."
  if (( ${#extra_args[@]} > 0 )); then
    maybe_run_buildx_load "deploy/images/server/Dockerfile" "." "ploy-server:vps" "${extra_args[@]}"
    maybe_run_buildx_load "deploy/images/node/Dockerfile" "." "ploy-node:vps" "${extra_args[@]}"
    maybe_run_buildx_load "deploy/local/garage/Dockerfile.init" "." "ploy-garage-init:vps" "${extra_args[@]}"
  else
    maybe_run_buildx_load "deploy/images/server/Dockerfile" "." "ploy-server:vps"
    maybe_run_buildx_load "deploy/images/node/Dockerfile" "." "ploy-node:vps"
    maybe_run_buildx_load "deploy/local/garage/Dockerfile.init" "." "ploy-garage-init:vps"
  fi
}

build_workflow_images() {
  local refs_file="$1"
  local -a extra_args=()
  local entry dir ref context

  : > "$refs_file"
  if [[ -n "${PLOY_CA_CERTS:-}" ]]; then
    extra_args+=(--secret "id=ploy_ca_bundle,src=${PLOY_CA_CERTS}")
  fi

  log "Building workflow images into ${PLOY_CONTAINER_REGISTRY} ..."
  while IFS= read -r entry; do
    [[ -z "$entry" ]] && continue
    dir="${entry##*/}"
    ref="${PLOY_CONTAINER_REGISTRY}/$(mig_repo_name "$entry"):latest"

    if [[ "$entry" == "migs/mig-codex" ]]; then
      context="."
      if (( ${#extra_args[@]} > 0 )); then
        maybe_run_buildx_load "deploy/images/migs/mig-codex/Dockerfile" "$context" "$ref" "${extra_args[@]}"
      else
        maybe_run_buildx_load "deploy/images/migs/mig-codex/Dockerfile" "$context" "$ref"
      fi
    elif [[ "$entry" == "mig/orw-cli-gradle" || "$entry" == "mig/orw-cli-maven" ]]; then
      context="."
      if (( ${#extra_args[@]} > 0 )); then
        maybe_run_buildx_load "deploy/images/mig/${dir}/Dockerfile" "$context" "$ref" "${extra_args[@]}"
      else
        maybe_run_buildx_load "deploy/images/mig/${dir}/Dockerfile" "$context" "$ref"
      fi
    else
      context="deploy/images/${entry}"
      if (( ${#extra_args[@]} > 0 )); then
        maybe_build_context_image "$ref" \
          --platform "$PLATFORM" \
          "${extra_args[@]}" \
          --provenance=false --sbom=false --pull --progress=plain \
          "$context"
      else
        maybe_build_context_image "$ref" \
          --platform "$PLATFORM" \
          --provenance=false --sbom=false --pull --progress=plain \
          "$context"
      fi
    fi
    printf '%s\n' "$ref" >> "$refs_file"
  done < <(discover_mig_dirs)

  if (( ${#extra_args[@]} > 0 )); then
    maybe_run_buildx_load "deploy/images/gates/gradle/Dockerfile.jdk11" "deploy/images/gates/gradle" "${PLOY_CONTAINER_REGISTRY}/ploy-gate-gradle:jdk11" "${extra_args[@]}"
    maybe_run_buildx_load "deploy/images/gates/gradle/Dockerfile.jdk17" "deploy/images/gates/gradle" "${PLOY_CONTAINER_REGISTRY}/ploy-gate-gradle:jdk17" "${extra_args[@]}"
  else
    maybe_run_buildx_load "deploy/images/gates/gradle/Dockerfile.jdk11" "deploy/images/gates/gradle" "${PLOY_CONTAINER_REGISTRY}/ploy-gate-gradle:jdk11"
    maybe_run_buildx_load "deploy/images/gates/gradle/Dockerfile.jdk17" "deploy/images/gates/gradle" "${PLOY_CONTAINER_REGISTRY}/ploy-gate-gradle:jdk17"
  fi
  printf '%s\n' "${PLOY_CONTAINER_REGISTRY}/ploy-gate-gradle:jdk11" >> "$refs_file"
  printf '%s\n' "${PLOY_CONTAINER_REGISTRY}/ploy-gate-gradle:jdk17" >> "$refs_file"

  log "Mirroring upstream build-gate base images into ${PLOY_CONTAINER_REGISTRY} ..."
  maybe_pull_image maven:3-eclipse-temurin-11
  docker tag maven:3-eclipse-temurin-11 "${PLOY_CONTAINER_REGISTRY}/maven:3-eclipse-temurin-11"
  printf '%s\n' "${PLOY_CONTAINER_REGISTRY}/maven:3-eclipse-temurin-11" >> "$refs_file"

  maybe_pull_image maven:3-eclipse-temurin-17
  docker tag maven:3-eclipse-temurin-17 "${PLOY_CONTAINER_REGISTRY}/maven:3-eclipse-temurin-17"
  printf '%s\n' "${PLOY_CONTAINER_REGISTRY}/maven:3-eclipse-temurin-17" >> "$refs_file"

  maybe_pull_image golang:1.22
  docker tag golang:1.22 "${PLOY_CONTAINER_REGISTRY}/golang:1.22"
  printf '%s\n' "${PLOY_CONTAINER_REGISTRY}/golang:1.22" >> "$refs_file"

  maybe_pull_image rust:1.76
  docker tag rust:1.76 "${PLOY_CONTAINER_REGISTRY}/rust:1.76"
  printf '%s\n' "${PLOY_CONTAINER_REGISTRY}/rust:1.76" >> "$refs_file"
}

pull_service_images() {
  log "Pulling offline service images..."
  maybe_pull_image dxflrs/garage:v2.2.0
  maybe_pull_image registry:2.8.3
  maybe_pull_image gradle/build-cache-node:21.2
}

generate_tokens() {
  "$PYTHON_BIN" <<'PY'
import base64
import hashlib
import hmac
import json
import os
import secrets
import time

secret = os.environ["PLOY_AUTH_SECRET"]
cluster_id = os.environ["PLOY_VPS_CLUSTER_ID"]

def b64url(data: bytes) -> str:
    return base64.urlsafe_b64encode(data).rstrip(b"=").decode("ascii")

def gen_token(role: str):
    now = int(time.time())
    exp = now + 365 * 24 * 60 * 60
    header = {"alg": "HS256", "typ": "JWT"}
    jti = secrets.token_urlsafe(16)
    payload = {
        "cluster_id": cluster_id,
        "role": role,
        "token_type": "api",
        "iat": now,
        "exp": exp,
        "jti": jti,
    }
    header_b64 = b64url(json.dumps(header, separators=(",", ":")).encode("utf-8"))
    payload_b64 = b64url(json.dumps(payload, separators=(",", ":")).encode("utf-8"))
    unsigned = f"{header_b64}.{payload_b64}"
    sig = hmac.new(secret.encode("utf-8"), unsigned.encode("utf-8"), hashlib.sha256).digest()
    token = unsigned + "." + b64url(sig)
    token_hash = hashlib.sha256(token.encode("utf-8")).hexdigest()
    return token, jti, token_hash

admin_token, admin_id, admin_hash = gen_token("cli-admin")
worker1_token, worker1_id, worker1_hash = gen_token("worker")
worker2_token, worker2_id, worker2_hash = gen_token("worker")

print(f"ADMIN_TOKEN={admin_token}")
print(f"ADMIN_TOKEN_ID={admin_id}")
print(f"ADMIN_TOKEN_HASH={admin_hash}")
print(f"WORKER1_TOKEN={worker1_token}")
print(f"WORKER1_TOKEN_ID={worker1_id}")
print(f"WORKER1_TOKEN_HASH={worker1_hash}")
print(f"WORKER2_TOKEN={worker2_token}")
print(f"WORKER2_TOKEN_ID={worker2_id}")
print(f"WORKER2_TOKEN_HASH={worker2_hash}")
PY
}

write_stack_files() {
  local bundle_dir="$1"

  mkdir -p "$bundle_dir/server" "$bundle_dir/node1" "$bundle_dir/node2" "$bundle_dir/garage" "$bundle_dir/registry" "$bundle_dir/gradle-build-cache"

  cp deploy/vps/docker-compose.yml "$bundle_dir/docker-compose.yml"
  cp deploy/vps/remote-redeploy.sh "$bundle_dir/remote-redeploy.sh"
  cp deploy/local/server/ployd.yaml "$bundle_dir/server/ployd.yaml"
  cp deploy/local/garage/config.toml "$bundle_dir/garage/config.toml"
  cp deploy/local/garage/setup.sh "$bundle_dir/garage/setup.sh"
  cp deploy/local/registry/config.yml "$bundle_dir/registry/config.yml"
  cp deploy/local/gradle-build-cache/config.yaml "$bundle_dir/gradle-build-cache/config.yaml"
  cp deploy/local/gradle-build-cache/entrypoint.sh "$bundle_dir/gradle-build-cache/entrypoint.sh"

  cat > "$bundle_dir/node1/ployd-node.yaml" <<EOF
server_url: "http://server:8080"
node_id: "${PLOY_VPS_NODE1_ID}"
cluster_id: "${PLOY_VPS_CLUSTER_ID}"

http:
  listen: ":8444"

heartbeat:
  interval: 30s
  timeout: 10s
EOF

  cat > "$bundle_dir/node2/ployd-node.yaml" <<EOF
server_url: "http://server:8080"
node_id: "${PLOY_VPS_NODE2_ID}"
cluster_id: "${PLOY_VPS_CLUSTER_ID}"

http:
  listen: ":8444"

heartbeat:
  interval: 30s
  timeout: 10s
EOF

  printf '%s' "$WORKER1_TOKEN" > "$bundle_dir/node1/bearer-token"
  printf '%s' "$WORKER2_TOKEN" > "$bundle_dir/node2/bearer-token"
  chmod 600 "$bundle_dir/node1/bearer-token" "$bundle_dir/node2/bearer-token"

  cat > "$bundle_dir/stack.env" <<EOF
PLOY_DB_DSN=${PLOY_VPS_DB_DSN}
PLOY_AUTH_SECRET=${PLOY_AUTH_SECRET}
PLOY_CLUSTER_ID=${PLOY_VPS_CLUSTER_ID}
PLOY_SERVER_PORT=${PLOY_SERVER_PORT}
PLOY_REGISTRY_PORT=${PLOY_REGISTRY_PORT}
PLOY_CONTAINER_REGISTRY=${PLOY_CONTAINER_REGISTRY}
PLOY_CONTAINER_SOCKET_PATH=${PLOY_CONTAINER_SOCKET_PATH}
PLOY_DOCKER_NETWORK=${PLOY_DOCKER_NETWORK}
PLOY_NODE1_ID=${PLOY_VPS_NODE1_ID}
PLOY_NODE2_ID=${PLOY_VPS_NODE2_ID}
PLOY_NODE1_NAME=${PLOY_VPS_NODE1_NAME}
PLOY_NODE2_NAME=${PLOY_VPS_NODE2_NAME}
PLOY_NODE1_IP=${SSH_HOST}
PLOY_NODE2_IP=${SSH_HOST}
PLOY_NODE1_CONCURRENCY=${PLOY_VPS_NODE1_CONCURRENCY}
PLOY_NODE2_CONCURRENCY=${PLOY_VPS_NODE2_CONCURRENCY}
PLOY_NODE1_WORKDIR=${PLOY_VPS_WORKDIR_ROOT}/node1
PLOY_NODE2_WORKDIR=${PLOY_VPS_WORKDIR_ROOT}/node2
EOF

  cat > "$bundle_dir/tokens.env" <<EOF
ADMIN_TOKEN=${ADMIN_TOKEN}
ADMIN_TOKEN_ID=${ADMIN_TOKEN_ID}
ADMIN_TOKEN_HASH=${ADMIN_TOKEN_HASH}
WORKER1_TOKEN=${WORKER1_TOKEN}
WORKER1_TOKEN_ID=${WORKER1_TOKEN_ID}
WORKER1_TOKEN_HASH=${WORKER1_TOKEN_HASH}
WORKER2_TOKEN=${WORKER2_TOKEN}
WORKER2_TOKEN_ID=${WORKER2_TOKEN_ID}
WORKER2_TOKEN_HASH=${WORKER2_TOKEN_HASH}
EOF
}

save_images() {
  local refs_file="$1"
  local output="$2"
  local ref
  local -a refs=(
    "ploy-server:vps"
    "ploy-node:vps"
    "ploy-garage-init:vps"
    "dxflrs/garage:v2.2.0"
    "registry:2.8.3"
    "gradle/build-cache-node:21.2"
  )

  log "Saving Docker images to ${output} ..."
  while IFS= read -r ref; do
    [[ -z "$ref" ]] && continue
    refs+=("$ref")
  done < "$refs_file"
  docker save -o "$output" "${refs[@]}"
}

stream_remote_bundle() {
  local bundle_dir="$1"
  local refs_file="$2"
  local remote_prepare
  local ref
  local -a refs=(
    "ploy-server:vps"
    "ploy-node:vps"
    "ploy-garage-init:vps"
    "dxflrs/garage:v2.2.0"
    "registry:2.8.3"
    "gradle/build-cache-node:21.2"
  )

  remote_prepare="set -euo pipefail; mkdir -p '$REMOTE_DIR/current'; rm -rf '$REMOTE_DIR/current'/*"
  log "Streaming bundle to ${SSH_USER}@${SSH_HOST}:${REMOTE_DIR}/current ..."
  "${ssh_cmd[@]}" "${SSH_USER}@${SSH_HOST}" "$remote_prepare"
  tar -C "$bundle_dir" -cf - . | "${ssh_cmd[@]}" "${SSH_USER}@${SSH_HOST}" "tar -C '$REMOTE_DIR/current' -xf -"

  log "Streaming Docker images to ${SSH_USER}@${SSH_HOST}:${REMOTE_DIR}/current/docker-images.tar ..."
  while IFS= read -r ref; do
    [[ -z "$ref" ]] && continue
    refs+=("$ref")
  done < "$refs_file"
  docker save "${refs[@]}" | "${ssh_cmd[@]}" "${SSH_USER}@${SSH_HOST}" "cat > '$REMOTE_DIR/current/docker-images.tar'"
}

build_local_descriptor() {
  local cluster_dir server_url

  server_url="http://${SSH_HOST}:${PLOY_SERVER_PORT}"
  cluster_dir="$PLOY_CONFIG_HOME/clusters"
  mkdir -p "$cluster_dir"
  cat > "$cluster_dir/${PLOY_VPS_CLUSTER_ID}.json" <<EOF
{
  "cluster_id": "${PLOY_VPS_CLUSTER_ID}",
  "address": "${server_url}",
  "ssh_identity_path": "${SSH_IDENTITY}",
  "token": "${ADMIN_TOKEN}"
}
EOF
  ln -sf "${PLOY_VPS_CLUSTER_ID}.json" "$cluster_dir/default"
}

main() {
  local skip_build_flag=0
  local stage_root bundle_dir archive_path workflow_refs_file remote_exec
  local -a ssh_cmd
  local -a scp_cmd

  parse_args "$@"

  if [[ -z "$SSH_HOST" ]]; then
    echo "error: --address is required" >&2
    exit 1
  fi
  if [[ -z "$PLOY_VPS_DB_DSN" ]]; then
    echo "error: PLOY_VPS_DB_DSN is required" >&2
    exit 1
  fi
  if [[ ! -f "$SSH_IDENTITY" ]]; then
    echo "error: SSH identity file not found: $SSH_IDENTITY" >&2
    exit 1
  fi
  if [[ "$PLATFORM" == *,* ]]; then
    echo "error: PLATFORM must be a single platform when using docker buildx --load" >&2
    exit 1
  fi

  need docker
  need ssh
  need scp
  need tar
  need make
  need openssl
  need "$PYTHON_BIN"
  if ! docker buildx version >/dev/null 2>&1; then
    echo "error: docker buildx not available" >&2
    exit 1
  fi

  prepare_ca_certs_path
  configure_docker_registry_ca_if_needed

  if is_true "$PLOY_SKIP_BUILD"; then
    skip_build_flag=1
  fi
  if (( LOW_DISK_MODE )) && [[ -f "dist/ploy" ]]; then
    skip_build_flag=1
  fi

  if (( skip_build_flag )); then
    log "Skipping make build and reusing local dist artifacts..."
  else
    log "Building local binaries (make build)..."
    make build
  fi

  if [[ ! -f "dist/ploy" ]]; then
    echo "error: dist/ploy not found; run make build or unset PLOY_SKIP_BUILD" >&2
    exit 1
  fi

  if [[ ! -f "$AUTH_SECRET_PATH" ]]; then
    log "Generating auth secret at $AUTH_SECRET_PATH ..."
    mkdir -p "$(dirname "$AUTH_SECRET_PATH")"
    openssl rand -hex 32 > "$AUTH_SECRET_PATH"
  fi
  PLOY_AUTH_SECRET="$(cat "$AUTH_SECRET_PATH")"
  export PLOY_AUTH_SECRET PLOY_VPS_CLUSTER_ID

  build_runtime_images
  pull_service_images

  stage_root="$(mktemp -d)"
  bundle_dir="$stage_root/bundle"
  mkdir -p "$bundle_dir"
  workflow_refs_file="$bundle_dir/workflow-images.txt"

  build_workflow_images "$workflow_refs_file"
  eval "$(generate_tokens)"
  write_stack_files "$bundle_dir"

  ssh_cmd=(ssh -o BatchMode=yes -o StrictHostKeyChecking=accept-new -i "$SSH_IDENTITY" -p "$SSH_PORT")
  scp_cmd=(scp -o BatchMode=yes -o StrictHostKeyChecking=accept-new -i "$SSH_IDENTITY" -P "$SSH_PORT")

  if (( LOW_DISK_MODE )); then
    stream_remote_bundle "$bundle_dir" "$workflow_refs_file"
    remote_exec="set -euo pipefail; cd '$REMOTE_DIR/current'; chmod +x ./remote-redeploy.sh; ./remote-redeploy.sh"
  else
    save_images "$workflow_refs_file" "$bundle_dir/docker-images.tar"
    archive_path="$stage_root/ploy-vps-bundle.tar"
    tar -C "$bundle_dir" -cf "$archive_path" .

    log "Uploading bundle to ${SSH_USER}@${SSH_HOST}:${REMOTE_DIR} ..."
    "${ssh_cmd[@]}" "${SSH_USER}@${SSH_HOST}" "sudo mkdir -p '$REMOTE_DIR/current'"
    "${scp_cmd[@]}" "$archive_path" "${SSH_USER}@${SSH_HOST}:$REMOTE_DIR/ploy-vps-bundle.tar"
    remote_exec="set -euo pipefail; rm -rf '$REMOTE_DIR/current'/*; tar -C '$REMOTE_DIR/current' -xf '$REMOTE_DIR/ploy-vps-bundle.tar'; cd '$REMOTE_DIR/current'; chmod +x ./remote-redeploy.sh; ./remote-redeploy.sh"
  fi

  if (( DROP_DB )); then
    remote_exec="$remote_exec --drop-db"
  fi
  "${ssh_cmd[@]}" "${SSH_USER}@${SSH_HOST}" "$remote_exec"

  build_local_descriptor

  log "Offline VPS bundle deployed."
  log "Local CLI descriptor written under ${PLOY_CONFIG_HOME}."
  rm -rf "$stage_root"
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  main "$@"
fi
