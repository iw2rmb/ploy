#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

REMOTE_USER="s_v.v.kovalev"
REMOTE_HOST="10.120.34.186"
REMOTE_HOME="/home/idm/s_v.v.kovalev"
REMOTE_ROOT="$REMOTE_HOME/opt/ploy"
REMOTE_APP_DIR="${REMOTE_ROOT}"
REMOTE_IMAGES_TAR="$REMOTE_HOME/tmp/ploy-images.tar"

SSH_TARGET="s_v.v.kovalev@10.120.34.186"
PYTHON_BIN="${PYTHON_BIN:-python3}"
AUTH_SECRET_PATH="${AUTH_SECRET_PATH:-$ROOT_DIR/deploy/vps/auth-secret.txt}"
#PLOY_DB_DSN="postgres://$REMOTE_USER@host.docker.internal/ploy"
PLOY_DB_DSN="postgres://$REMOTE_USER@localhost/ploy"
PLOY_CA_CERTS="${PLOY_CA_CERTS:-}"
PLOY_SERVER_PORT="8080"
PLOY_REGISTRY_PORT="${PLOY_REGISTRY_PORT:-5000}"
PLOY_CONTAINER_REGISTRY="${PLOY_CONTAINER_REGISTRY:-127.0.0.1:${PLOY_REGISTRY_PORT}/ploy}"
CLUSTER_ID="${CLUSTER_ID:-local}"
NODE_ID="${NODE_ID:-local1}"

CLEAN=0
DROP_DB=0

RUNTIME_IMAGE_REFS=(
  "ploy-server:local"
  "ploy-node:local"
  "ploy-garage-init:local"
)

SERVICE_IMAGE_REFS=(
  "dxflrs/amd64_garage:v2.2.0"
  "amd64/registry:3"
  "gradle/build-cache-node:21.2"
)

log() {
  echo "[$(date -u +%H:%M:%S)] $*"
}

need() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "error: missing dependency: $1" >&2
    exit 1
  fi
}

usage() {
  cat <<'USAGE'
Usage: ./deploy/vps/run.sh [--clean] [--drop-db]

Deploys the local Docker-based stack to:
  s_v.v.kovalev@10.120.34.186

Options:
  --clean    Rebuild local binaries/images instead of reusing existing ones
  --drop-db  Drop and recreate the remote ploy database before deploy
  -h, --help Show this help

Required environment:
  PLOY_DB_DSN  PostgreSQL DSN reachable from both the VPS host and containers

Optional environment:
  PLOY_CA_CERTS      PEM CA bundle used the same way as deploy/local/run.sh
  PLOY_SERVER_PORT   Remote server port (default: 8080)
  PLOY_REGISTRY_PORT Remote registry port (default: 5000)
USAGE
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --clean)
        CLEAN=1
        ;;
      --drop-db)
        DROP_DB=1
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

run_as_root() {
  if [[ "${EUID:-$(id -u)}" -eq 0 ]]; then
    "$@"
    return
  fi

  if command -v sudo >/dev/null 2>&1 && sudo -n true >/dev/null 2>&1; then
    sudo "$@"
    return
  fi

  return 1
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

  if ! command -v colima >/dev/null 2>&1; then
    echo "error: docker context is colima but colima CLI is not installed" >&2
    exit 1
  fi

  log "Installing PLOY_CA_CERTS into colima docker registry trust..."
  colima ssh -- sudo mkdir -p \
    /etc/docker/certs.d/docker.io \
    /etc/docker/certs.d/registry-1.docker.io \
    /etc/docker/certs.d/auth.docker.io \
    /etc/docker/certs.d/index.docker.io
  cat "$ca_path" | colima ssh -- sudo tee /etc/docker/certs.d/docker.io/ca.crt >/dev/null
  cat "$ca_path" | colima ssh -- sudo tee /etc/docker/certs.d/registry-1.docker.io/ca.crt >/dev/null
  cat "$ca_path" | colima ssh -- sudo tee /etc/docker/certs.d/auth.docker.io/ca.crt >/dev/null
  cat "$ca_path" | colima ssh -- sudo tee /etc/docker/certs.d/index.docker.io/ca.crt >/dev/null
  colima ssh -- sudo chmod 0644 \
    /etc/docker/certs.d/docker.io/ca.crt \
    /etc/docker/certs.d/registry-1.docker.io/ca.crt \
    /etc/docker/certs.d/auth.docker.io/ca.crt \
    /etc/docker/certs.d/index.docker.io/ca.crt
  colima ssh -- sudo mkdir -p /usr/local/share/ca-certificates/ploy
  cat "$ca_path" | colima ssh -- sudo tee /usr/local/share/ca-certificates/ploy/ploy-ca.crt >/dev/null
  colima ssh -- sudo update-ca-certificates >/dev/null
  colima ssh -- sudo systemctl restart docker
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
  local ca_path context os_name engine_name current_ca="${PLOY_CA_CERTS:-}"

  if [[ -z "$current_ca" ]]; then
    return 0
  fi

  ca_path="$current_ca"
  if [[ "$ca_path" != /* ]]; then
    ca_path="$ROOT_DIR/$ca_path"
  fi
  if [[ ! -f "$ca_path" ]]; then
    echo "error: PLOY_CA_CERTS file not found: $ca_path" >&2
    exit 1
  fi
  if [[ ! -s "$ca_path" ]]; then
    echo "error: PLOY_CA_CERTS file is empty: $ca_path" >&2
    exit 1
  fi

  PLOY_CA_CERTS="$ca_path"
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
  echo "error: configure docker daemon trust manually, then rerun deploy/vps/run.sh" >&2
  exit 1
}

maybe_run_buildx_load() {
  local dockerfile="$1"
  local context="$2"
  local ref="$3"
  shift 3
  local -a extra_args=("$@")
  local -a cmd=(docker buildx build --platform linux/amd64)

  if (( CLEAN == 0 )) && docker image inspect "$ref" >/dev/null 2>&1; then
    log "Reusing local image ${ref}"
    return 0
  fi

  if ((${#extra_args[@]} > 0)); then
    cmd+=("${extra_args[@]}")
  fi
  cmd+=(
    --provenance=false
    --sbom=false
    --pull
    --progress=plain
    -f "$dockerfile"
    -t "$ref"
    --load
    "$context"
  )
  "${cmd[@]}"
}

maybe_run_inline_buildx_load() {
  local ref="$1"
  local dockerfile_content="$2"
  shift 2
  local -a extra_args=("$@")
  local -a cmd=(docker buildx build --platform linux/amd64)

  if (( CLEAN == 0 )) && docker image inspect "$ref" >/dev/null 2>&1; then
    log "Reusing local image ${ref}"
    return 0
  fi

  if ((${#extra_args[@]} > 0)); then
    cmd+=("${extra_args[@]}")
  fi
  cmd+=(
    --provenance=false
    --sbom=false
    --pull
    --progress=plain
    -t "$ref"
    --load
    -f -
    .
  )
  "${cmd[@]}" <<EOF
${dockerfile_content}
EOF
}

maybe_pull_image() {
  local ref="$1"

  if (( CLEAN == 0 )) && docker image inspect "$ref" >/dev/null 2>&1; then
    log "Reusing local image ${ref}"
    return 0
  fi

  docker pull "$ref"
}

maybe_pull_and_tag() {
  local source_ref="$1"
  local target_ref="$2"

  if (( CLEAN == 0 )) && docker image inspect "$target_ref" >/dev/null 2>&1; then
    log "Reusing local image ${target_ref}"
    return 0
  fi

  maybe_pull_image "$source_ref"
  docker tag "$source_ref" "$target_ref"
}

discover_mig_dirs() {
  local root_migs="deploy/images/migs"
  local root_mig="deploy/images/mig"
  {
    if [[ -d "$root_migs" ]]; then
      find "$root_migs" -mindepth 1 -maxdepth 1 -type d -print | while read -r d; do
        printf 'migs/%s\n' "$(basename "$d")"
      done
    fi

    if [[ -d "$root_mig" ]]; then
      find "$root_mig" -mindepth 1 -maxdepth 1 -type d -print | while read -r d; do
        printf 'mig/%s\n' "$(basename "$d")"
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

build_runtime_images() {
  local -a extra_args=()
  local server_dockerfile
  local node_dockerfile
  if [[ -n "${PLOY_CA_CERTS:-}" ]]; then
    extra_args=(--secret "id=ploy_ca_bundle,src=${PLOY_CA_CERTS}")
  fi

  read -r -d '' server_dockerfile <<'EOF' || true
ARG ALPINE_VERSION=3.20
FROM alpine:${ALPINE_VERSION}
RUN --mount=type=secret,id=ploy_ca_bundle,target=/run/secrets/ploy_ca_bundle,required=false \
    if [ -s /run/secrets/ploy_ca_bundle ]; then mkdir -p /etc/ssl/certs && cat /run/secrets/ploy_ca_bundle >> /etc/ssl/certs/ca-certificates.crt; fi && \
    apk add --no-cache ca-certificates bash tzdata curl jq git && \
    if [ -s /run/secrets/ploy_ca_bundle ]; then cat /run/secrets/ploy_ca_bundle >> /etc/ssl/certs/ca-certificates.crt; fi && \
    adduser -D -H -s /sbin/nologin ploy && \
    mkdir -p /etc/ploy /etc/ploy/pki /etc/ploy/gates /etc/ploy/schemas /var/lib/ploy && \
    chown -R ploy:ploy /etc/ploy /var/lib/ploy
COPY gates/ /etc/ploy/gates/
COPY docs/schemas/gate_profile.schema.json /etc/ploy/schemas/gate_profile.schema.json
USER ploy
EXPOSE 8443 9100
ENTRYPOINT ["/bin/sh", "-lc"]
CMD ["/usr/local/bin/ployd --config /etc/ploy/ployd.yaml"]
EOF

  read -r -d '' node_dockerfile <<'EOF' || true
ARG ALPINE_VERSION=3.20
FROM alpine:${ALPINE_VERSION}
RUN --mount=type=secret,id=ploy_ca_bundle,target=/run/secrets/ploy_ca_bundle,required=false \
    if [ -s /run/secrets/ploy_ca_bundle ]; then mkdir -p /etc/ssl/certs && cat /run/secrets/ploy_ca_bundle >> /etc/ssl/certs/ca-certificates.crt; fi && \
    apk add --no-cache ca-certificates bash tzdata curl jq docker-cli git rsync && \
    if [ -s /run/secrets/ploy_ca_bundle ]; then cat /run/secrets/ploy_ca_bundle >> /etc/ssl/certs/ca-certificates.crt; fi && \
    adduser -D -H -s /sbin/nologin ploy && \
    mkdir -p /etc/ploy /etc/ploy/pki /etc/ploy/gates /var/lib/ploy && \
    chown -R ploy:ploy /etc/ploy /var/lib/ploy
COPY gates/ /etc/ploy/gates/
USER root
VOLUME ["/var/run/docker.sock"]
USER ploy
ENTRYPOINT ["/bin/sh", "-lc"]
CMD ["/usr/local/bin/ployd-node --config /etc/ploy/ployd-node.yaml"]
EOF

  maybe_run_buildx_load "deploy/local/garage/Dockerfile" "." "ploy-garage-init:local"
  maybe_run_inline_buildx_load "ploy-server:local" "$server_dockerfile" "${extra_args[@]}"
  maybe_run_inline_buildx_load "ploy-node:local" "$node_dockerfile" "${extra_args[@]}"
}

build_workflow_images() {
  local refs_file="$1"
  local -a extra_args=()
  local entry source_group dir ref

  : > "$refs_file"
  if [[ -n "${PLOY_CA_CERTS:-}" ]]; then
    extra_args=(--secret "id=ploy_ca_bundle,src=${PLOY_CA_CERTS}")
  fi

  while read -r entry; do
    [[ -n "$entry" ]] || continue
    source_group="${entry%%/*}"
    dir="${entry##*/}"
    ref="${PLOY_CONTAINER_REGISTRY}/$(mig_repo_name "$entry"):latest"
    if [[ "$source_group" == "migs" && "$dir" == "mig-codex" ]]; then
      maybe_run_buildx_load "deploy/images/migs/mig-codex/Dockerfile" "." "$ref" "${extra_args[@]}"
    elif [[ "$source_group" == "mig" && ( "$dir" == "orw-cli-gradle" || "$dir" == "orw-cli-maven" ) ]]; then
      maybe_run_buildx_load "deploy/images/mig/${dir}/Dockerfile" "." "$ref" "${extra_args[@]}"
    else
      maybe_run_buildx_load "deploy/images/${source_group}/${dir}/Dockerfile" "deploy/images/${source_group}/${dir}" "$ref" "${extra_args[@]}"
    fi
    printf '%s\n' "$ref" >> "$refs_file"
  done < <(discover_mig_dirs)

  ref="${PLOY_CONTAINER_REGISTRY}/ploy-gate-gradle:jdk11"
  maybe_run_buildx_load "deploy/images/gates/gradle/Dockerfile.jdk11" "deploy/images/gates/gradle" "$ref" "${extra_args[@]}"
  printf '%s\n' "$ref" >> "$refs_file"

  ref="${PLOY_CONTAINER_REGISTRY}/ploy-gate-gradle:jdk17"
  maybe_run_buildx_load "deploy/images/gates/gradle/Dockerfile.jdk17" "deploy/images/gates/gradle" "$ref" "${extra_args[@]}"
  printf '%s\n' "$ref" >> "$refs_file"

  maybe_pull_and_tag "maven:3-eclipse-temurin-11" "${PLOY_CONTAINER_REGISTRY}/maven:3-eclipse-temurin-11"
  maybe_pull_and_tag "maven:3-eclipse-temurin-17" "${PLOY_CONTAINER_REGISTRY}/maven:3-eclipse-temurin-17"
  # maybe_pull_and_tag "golang:1.22" "${PLOY_CONTAINER_REGISTRY}/golang:1.22"
  # maybe_pull_and_tag "rust:1.76" "${PLOY_CONTAINER_REGISTRY}/rust:1.76"

  printf '%s\n' \
    "${PLOY_CONTAINER_REGISTRY}/maven:3-eclipse-temurin-11" \
    "${PLOY_CONTAINER_REGISTRY}/maven:3-eclipse-temurin-17" \
    # "${PLOY_CONTAINER_REGISTRY}/golang:1.22" \
    # "${PLOY_CONTAINER_REGISTRY}/rust:1.76" \
    >> "$refs_file"

  sort -u -o "$refs_file" "$refs_file"
}

save_images() {
  local refs_file="$1"
  local output="$2"
  local -a refs=()

  refs+=("${RUNTIME_IMAGE_REFS[@]}")
  refs+=("${SERVICE_IMAGE_REFS[@]}")
  while IFS= read -r ref; do
    [[ -n "$ref" ]] || continue
    refs+=("$ref")
  done < "$refs_file"

  docker save -o "$output" "${refs[@]}"
}

generate_tokens() {
  "$PYTHON_BIN" <<'PY'
import os, base64, json, hmac, hashlib, secrets, time

secret = os.environ["PLOY_AUTH_SECRET"]
cluster_id = os.environ.get("CLUSTER_ID", "local")

def b64url(data: bytes) -> str:
    return base64.urlsafe_b64encode(data).rstrip(b"=").decode("ascii")

def gen_token(role: str):
    now = int(time.time())
    exp = now + 365*24*60*60
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
worker_token, worker_id, worker_hash = gen_token("worker")

print(f"ADMIN_TOKEN={admin_token}")
print(f"ADMIN_TOKEN_ID={admin_id}")
print(f"ADMIN_TOKEN_HASH={admin_hash}")
print(f"WORKER_TOKEN={worker_token}")
print(f"WORKER_TOKEN_ID={worker_id}")
print(f"WORKER_TOKEN_HASH={worker_hash}")
PY
}

write_env_file() {
  local path="$1"
  shift
  : > "$path"
  while [[ $# -gt 1 ]]; do
    printf '%s=%s\n' "$1" "$2" >> "$path"
    shift 2
  done
}

prepare_bundle() {
  local bundle_dir="$1"
  local workflow_refs_file="$2"
  local remote_ca_path="${3:-}"

  mkdir -p \
    "$bundle_dir/deploy/local" \
    "$bundle_dir/deploy/vps" \
    "$bundle_dir/dist"

  cp -R deploy/local/server "$bundle_dir/deploy/local/"
  cp -R deploy/local/node "$bundle_dir/deploy/local/"
  cp -R deploy/local/garage "$bundle_dir/deploy/local/"
  cp -R deploy/local/registry "$bundle_dir/deploy/local/"
  cp -R deploy/local/gradle-build-cache "$bundle_dir/deploy/local/"
  cp deploy/local/docker-compose.yml "$bundle_dir/deploy/local/docker-compose.yml"
  cp dist/ployd-linux "$bundle_dir/dist/ployd-linux"
  cp dist/ployd-node-linux "$bundle_dir/dist/ployd-node-linux"

  printf '%s' "$WORKER_TOKEN" > "$bundle_dir/deploy/local/node/bearer-token"
  chmod 600 "$bundle_dir/deploy/local/node/bearer-token"
  cp "$workflow_refs_file" "$bundle_dir/deploy/vps/workflow-images.txt"

  write_env_file "$bundle_dir/deploy/vps/stack.env" \
    PLOY_DB_DSN "$PLOY_DB_DSN" \
    PLOY_AUTH_SECRET "$PLOY_AUTH_SECRET" \
    PLOY_SERVER_PORT "$PLOY_SERVER_PORT" \
    PLOY_REGISTRY_PORT "$PLOY_REGISTRY_PORT" \
    PLOY_CONTAINER_REGISTRY "$PLOY_CONTAINER_REGISTRY" \
    PLOY_CA_CERTS "$remote_ca_path"

  write_env_file "$bundle_dir/deploy/vps/tokens.env" \
    CLUSTER_ID "$CLUSTER_ID" \
    NODE_ID "$NODE_ID" \
    ADMIN_TOKEN "$ADMIN_TOKEN" \
    ADMIN_TOKEN_ID "$ADMIN_TOKEN_ID" \
    ADMIN_TOKEN_HASH "$ADMIN_TOKEN_HASH" \
    WORKER_TOKEN "$WORKER_TOKEN" \
    WORKER_TOKEN_ID "$WORKER_TOKEN_ID" \
    WORKER_TOKEN_HASH "$WORKER_TOKEN_HASH"

  if [[ -n "$remote_ca_path" ]]; then
    mkdir -p "$bundle_dir/certs"
    cp "$PLOY_CA_CERTS" "$bundle_dir/certs/ploy-ca.pem"
  fi
}

upload_bundle() {
  local bundle_dir="$1"

  log "Uploading bundle to ${SSH_TARGET}:${REMOTE_APP_DIR} ..."
  tar -C "$bundle_dir" -czf - . | ssh "$SSH_TARGET" \
    "rm -rf '$REMOTE_APP_DIR' && mkdir -p '$REMOTE_APP_DIR' $REMOTE_HOME/tmp && chmod 1777 $REMOTE_HOME/tmp && tar -xzf - -C '$REMOTE_APP_DIR'"
}

upload_images_tar() {
  local images_tar="$1"

  log "Uploading image archive to ${SSH_TARGET}:${REMOTE_IMAGES_TAR} ..."
  cat "$images_tar" | ssh "$SSH_TARGET" "cat > '$REMOTE_IMAGES_TAR'"
}

remote_deploy() {
  local remote_ca_path="${1:-}"

  ssh "$SSH_TARGET" \
    "REMOTE_APP_DIR='$REMOTE_APP_DIR' REMOTE_IMAGES_TAR='$REMOTE_IMAGES_TAR' DROP_DB='$DROP_DB' REMOTE_CA_PATH='$remote_ca_path' bash -s" <<'EOF'
set -Eeuo pipefail

# band-aid
echo $VPS_PWD | sudo -S echo
sudo chmod 777 /private/tmp

log() {
  echo "[$(date -u +%H:%M:%S)] $*"
}

need() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "error: missing remote dependency: $1" >&2
    exit 1
  fi
}

load_env_file() {
  local path="$1"
  while IFS= read -r line || [[ -n "$line" ]]; do
    [[ -n "$line" ]] || continue
    [[ "$line" == \#* ]] && continue
    export "$line"
  done < "$path"
}

derive_admin_pg_dsn() {
  python3 <<'PY'
import os
from urllib.parse import urlsplit, urlunsplit

dsn = os.environ["PLOY_LOCAL_PG_DSN"].strip()
if not dsn:
    raise SystemExit("error: PLOY_DB_DSN is required")
if "://" not in dsn:
    raise SystemExit("error: PLOY_DB_DSN must be a URL DSN")
u = urlsplit(dsn)
if u.scheme not in ("postgres", "postgresql"):
    raise SystemExit("error: PLOY_DB_DSN must use postgres:// or postgresql://")
if u.path.strip("/") != "ploy":
    raise SystemExit("error: PLOY_DB_DSN must target database ploy")
print(urlunsplit((u.scheme, u.netloc, "/postgres", u.query, u.fragment)))
PY
}

wait_for_postgres() {
  local admin_dsn="$1"
  local last_status=""
  for i in {1..60}; do
    if last_status="$(PGCONNECT_TIMEOUT=2 pg_isready -d "$admin_dsn" 2>&1)"; then
      return 0
    fi
    if (( i == 1 || i % 10 == 0 )); then
      log "PostgreSQL not ready yet (${i}/60): ${last_status}"
    fi
    sleep 1
  done
  echo "error: PostgreSQL did not become ready in time (last status: ${last_status})" >&2
  exit 1
}

drop_and_recreate_ploy_db() {
  local admin_dsn="$1"
  psql "$admin_dsn" -v ON_ERROR_STOP=1 -qX -c "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = 'ploy' AND pid <> pg_backend_pid();" >/dev/null
  psql "$admin_dsn" -v ON_ERROR_STOP=1 -qX -c "DROP DATABASE IF EXISTS ploy;" >/dev/null
  psql "$admin_dsn" -v ON_ERROR_STOP=1 -qX -c "CREATE DATABASE ploy;" >/dev/null
}

ensure_ploy_db_exists() {
  local admin_dsn="$1"
  local exists
  exists="$(psql "$admin_dsn" -v ON_ERROR_STOP=1 -qXAt -c "SELECT 1 FROM pg_database WHERE datname = 'ploy' LIMIT 1;")"
  if [[ "$exists" == "1" ]]; then
    return 0
  fi
  psql "$admin_dsn" -v ON_ERROR_STOP=1 -qX -c "CREATE DATABASE ploy;" >/dev/null
}

wait_for_registry_health() {
  for i in {1..90}; do
    if python3 <<PY >/dev/null 2>&1
import sys
import urllib.error
import urllib.request

try:
    with urllib.request.urlopen("http://127.0.0.1:${PLOY_REGISTRY_PORT}/v2/", timeout=2) as resp:
        sys.exit(0 if 200 <= resp.status < 500 else 1)
except urllib.error.HTTPError as e:
    sys.exit(0 if 200 <= e.code < 500 else 1)
except Exception:
    sys.exit(1)
PY
    then
      return 0
    fi
    sleep 1
  done

  echo "error: registry did not become ready in time" >&2
  "${COMPOSE_CMD[@]}" ps || true
  "${COMPOSE_CMD[@]}" logs registry || true
  exit 1
}

wait_for_garage_bootstrap() {
  local garage_cid garage_init_cid garage_health init_state init_exit

  garage_cid="$("${COMPOSE_CMD[@]}" ps -a -q garage)"
  garage_init_cid="$("${COMPOSE_CMD[@]}" ps -a -q garage-init)"
  if [[ -z "$garage_cid" || -z "$garage_init_cid" ]]; then
    echo "error: could not resolve garage container IDs" >&2
    "${COMPOSE_CMD[@]}" ps || true
    exit 1
  fi

  for i in {1..90}; do
    garage_health="$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' "$garage_cid" 2>/dev/null || true)"
    init_state="$(docker inspect -f '{{.State.Status}}' "$garage_init_cid" 2>/dev/null || true)"
    init_exit="$(docker inspect -f '{{.State.ExitCode}}' "$garage_init_cid" 2>/dev/null || true)"

    if [[ "$garage_health" == "healthy" && "$init_state" == "exited" && "$init_exit" == "0" ]]; then
      return 0
    fi

    if [[ "$init_state" == "exited" && "$init_exit" != "0" ]]; then
      echo "error: garage-init failed with exit code ${init_exit}" >&2
      "${COMPOSE_CMD[@]}" ps || true
      "${COMPOSE_CMD[@]}" logs garage garage-init || true
      exit 1
    fi

    sleep 1
  done

  echo "error: garage bootstrap did not complete in time" >&2
  "${COMPOSE_CMD[@]}" ps || true
  "${COMPOSE_CMD[@]}" logs garage garage-init || true
  exit 1
}

wait_for_server_health() {
  local server_cid server_health server_state server_exit

  server_cid="$("${COMPOSE_CMD[@]}" ps -a -q server)"
  if [[ -z "$server_cid" ]]; then
    echo "error: could not resolve server container ID" >&2
    "${COMPOSE_CMD[@]}" ps || true
    exit 1
  fi

  for i in {1..90}; do
    server_health="$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' "$server_cid" 2>/dev/null || true)"
    server_state="$(docker inspect -f '{{.State.Status}}' "$server_cid" 2>/dev/null || true)"
    server_exit="$(docker inspect -f '{{.State.ExitCode}}' "$server_cid" 2>/dev/null || true)"

    if [[ "$server_health" == "healthy" ]]; then
      return 0
    fi
    if [[ "$server_health" == "none" && "$server_state" == "running" ]]; then
      return 0
    fi
    if [[ "$server_state" == "exited" || "$server_state" == "dead" ]]; then
      echo "error: server container is ${server_state} (exit=${server_exit})" >&2
      "${COMPOSE_CMD[@]}" ps || true
      "${COMPOSE_CMD[@]}" logs server || true
      exit 1
    fi

    sleep 1
  done

  echo "error: server container did not become healthy in time" >&2
  "${COMPOSE_CMD[@]}" ps || true
  "${COMPOSE_CMD[@]}" logs server || true
  exit 1
}

seed_tokens() {
  psql "$PLOY_DB_DSN" -v ON_ERROR_STOP=1 -qX -c "
    SET search_path TO ploy, public;
    INSERT INTO api_tokens (token_hash, token_id, cluster_id, role, description, issued_at, expires_at)
    VALUES (
      '${ADMIN_TOKEN_HASH}',
      '${ADMIN_TOKEN_ID}',
      '${CLUSTER_ID}',
      'cli-admin',
      'Initial admin token for VPS deploy',
      NOW(),
      NOW() + INTERVAL '365 days'
    )
    ON CONFLICT (token_hash) DO NOTHING;"

  psql "$PLOY_DB_DSN" -v ON_ERROR_STOP=1 -qX -c "
    SET search_path TO ploy, public;
    INSERT INTO api_tokens (token_hash, token_id, cluster_id, role, description, issued_at, expires_at)
    VALUES (
      '${WORKER_TOKEN_HASH}',
      '${WORKER_TOKEN_ID}',
      '${CLUSTER_ID}',
      'worker',
      'Worker token for node ${NODE_ID}',
      NOW(),
      NOW() + INTERVAL '365 days'
    )
    ON CONFLICT (token_hash) DO NOTHING;"
}

seed_node_record() {
  psql "$PLOY_DB_DSN" -v ON_ERROR_STOP=1 -qX -c "
    SET search_path TO ploy, public;
    INSERT INTO nodes (id, name, ip_address, version, concurrency)
    VALUES ('${NODE_ID}', 'vps-node-0001', '127.0.0.1', 'dev', 1)
    ON CONFLICT (id) DO NOTHING;"
}

set_global_env() {
  local key="$1"
  local value="$2"
  local payload

  payload="$(KEY="$key" VALUE="$value" python3 <<'PY'
import json
import os

print(json.dumps({
    "value": os.environ["VALUE"],
    "scope": "all",
    "secret": True,
}))
PY
)"

  curl -fsS -X PUT \
    "http://127.0.0.1:${PLOY_SERVER_PORT}/v1/config/env/${key}" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}" \
    -H "Content-Type: application/json" \
    --data "$payload" >/dev/null
}

install_remote_ca_if_needed() {
  local ca_path="$1"

  [[ -n "$ca_path" ]] || return 0
  [[ -f "$ca_path" ]] || { echo "error: remote CA file missing: $ca_path" >&2; exit 1; }

  sudo mkdir -p \
    /etc/docker/certs.d/docker.io \
    /etc/docker/certs.d/registry-1.docker.io \
    /etc/docker/certs.d/auth.docker.io \
    /etc/docker/certs.d/index.docker.io \
    /usr/local/share/ca-certificates/ploy
  sudo install -m 0644 "$ca_path" /etc/docker/certs.d/docker.io/ca.crt
  sudo install -m 0644 "$ca_path" /etc/docker/certs.d/registry-1.docker.io/ca.crt
  sudo install -m 0644 "$ca_path" /etc/docker/certs.d/auth.docker.io/ca.crt
  sudo install -m 0644 "$ca_path" /etc/docker/certs.d/index.docker.io/ca.crt
  sudo install -m 0644 "$ca_path" /usr/local/share/ca-certificates/ploy/ploy-ca.crt
  if command -v update-ca-certificates >/dev/null 2>&1; then
    sudo update-ca-certificates >/dev/null
  fi
  if command -v systemctl >/dev/null 2>&1; then
    sudo systemctl restart docker
  elif command -v service >/dev/null 2>&1; then
    sudo service docker restart
  else
    echo "error: cannot restart docker after CA install" >&2
    exit 1
  fi

  for i in {1..30}; do
    if docker info >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done

  echo "error: docker did not become ready after remote CA install" >&2
  exit 1
}

need docker
need sudo
need psql
need pg_isready
need curl
need python3

load_env_file "${REMOTE_APP_DIR}/deploy/vps/stack.env"
load_env_file "${REMOTE_APP_DIR}/deploy/vps/tokens.env"

install_remote_ca_if_needed "$REMOTE_CA_PATH"

admin_pg_dsn="$(derive_admin_pg_dsn)"
wait_for_postgres "$admin_pg_dsn"
if [[ "$DROP_DB" == "1" ]]; then
  drop_and_recreate_ploy_db "$admin_pg_dsn"
else
  ensure_ploy_db_exists "$admin_pg_dsn"
fi

if [[ -f "$REMOTE_IMAGES_TAR" ]]; then
    docker load -i "$REMOTE_IMAGES_TAR" >/dev/null
    rm -f "$REMOTE_IMAGES_TAR"
fi

cd "$REMOTE_APP_DIR"
COMPOSE_CMD=(docker compose --project-name local --env-file deploy/vps/stack.env -f deploy/local/docker-compose.yml)

"${COMPOSE_CMD[@]}" down --remove-orphans || true
"${COMPOSE_CMD[@]}" up -d --no-build garage garage-init registry gradle-build-cache server

wait_for_garage_bootstrap
wait_for_registry_health

while IFS= read -r ref || [[ -n "$ref" ]]; do
  [[ -n "$ref" ]] || continue
  docker push "$ref"
done < deploy/vps/workflow-images.txt

wait_for_server_health
seed_tokens
seed_node_record
set_global_env PLOY_GRADLE_BUILD_CACHE_URL "http://gradle-build-cache:5071/cache/"
set_global_env PLOY_GRADLE_BUILD_CACHE_PUSH "true"

"${COMPOSE_CMD[@]}" up -d --no-build node
"${COMPOSE_CMD[@]}" ps
EOF
}

main() {
  local bundle_dir images_tar workflow_refs_file remote_ca_path=""

  parse_args "$@"

  log "Checking prerequisites..."
  need docker
  need git
  need make
  need openssl
  need ssh
  need tar
  need "$PYTHON_BIN"
  if ! docker buildx version >/dev/null 2>&1; then
    echo "error: docker buildx not available (install docker buildx plugin)" >&2
    exit 1
  fi

  if [[ -z "$PLOY_DB_DSN" ]]; then
    echo "error: PLOY_DB_DSN is required (must be reachable from the VPS host and its containers)" >&2
    exit 1
  fi

  configure_docker_registry_ca_if_needed

  if (( CLEAN == 1 )) || [[ ! -f "dist/ployd-linux" || ! -f "dist/ployd-node-linux" ]]; then
    log "Building linux binaries..."
    make build
  else
    log "Reusing existing dist/*-linux binaries"
  fi

  if [[ ! -f "dist/ployd-linux" || ! -f "dist/ployd-node-linux" ]]; then
    echo "error: missing dist/ployd-linux or dist/ployd-node-linux after build step" >&2
    exit 1
  fi

  if [[ ! -f "$AUTH_SECRET_PATH" ]]; then
    log "Generating auth secret at ${AUTH_SECRET_PATH} ..."
    mkdir -p "$(dirname "$AUTH_SECRET_PATH")"
    openssl rand -hex 32 > "$AUTH_SECRET_PATH"
  fi

  PLOY_AUTH_SECRET="$(cat "$AUTH_SECRET_PATH")"
  export PLOY_AUTH_SECRET CLUSTER_ID
  # shellcheck disable=SC2046
  eval "$(generate_tokens)"

  # log "Preparing local Docker images..."
  # build_runtime_images
  # for ref in "${SERVICE_IMAGE_REFS[@]}"; do
  #   maybe_pull_image "$ref"
  # done

  # workflow_refs_file="$(mktemp)"
  # build_workflow_images "$workflow_refs_file"

  # bundle_dir="$(mktemp -d)"
  # images_tar="$(mktemp)"
  # trap 'rm -rf "$bundle_dir" "$workflow_refs_file" "$images_tar"' EXIT

  # if [[ -n "${PLOY_CA_CERTS:-}" ]]; then
  #   remote_ca_path="${REMOTE_APP_DIR}/certs/ploy-ca.pem"
  # fi
  # prepare_bundle "$bundle_dir" "$workflow_refs_file" "$remote_ca_path"

  # log "Packing Docker images..."
  # save_images "$workflow_refs_file" "$images_tar"

  # upload_bundle "$bundle_dir"
  # upload_images_tar "$images_tar"
  remote_deploy "$remote_ca_path"

  log "VPS deploy complete."
  log "Server: http://10.120.34.186:${PLOY_SERVER_PORT}/health"
  log "Registry: http://10.120.34.186:${PLOY_REGISTRY_PORT}/v2/"
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  main "$@"
fi
