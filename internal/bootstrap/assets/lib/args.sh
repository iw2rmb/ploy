# shellcheck shell=bash
# shellcheck disable=SC2034

usage() {
  cat <<'DOC'
Usage: bootstrap.sh [--cluster-id ID] [--node-id ID] [--node-address ADDR] [--primary]

Arguments:
  --cluster-id ID     Cluster identifier (required for joining existing clusters)
  --node-id ID        Node identifier recorded with issued certificates (default: control)
  --node-address ADDR Hostname or IP used in control-plane certificates (default: cluster ID)
  --primary           Indicates this node bootstraps the control-plane CA
DOC
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --cluster-id)
        if [[ $# -lt 2 ]]; then
          fail "--cluster-id requires a value"
        fi
        CLUSTER_ID="$2"
        shift 2
        ;;
      --node-id)
        if [[ $# -lt 2 ]]; then
          fail "--node-id requires a value"
        fi
        NODE_ID="$2"
        shift 2
        ;;
      --node-address)
        if [[ $# -lt 2 ]]; then
          fail "--node-address requires a value"
        fi
        NODE_ADDRESS="$2"
        shift 2
        ;;
      --primary)
        BOOTSTRAP_PRIMARY="true"
        shift
        ;;
      --help|-h)
        usage
        exit 0
        ;;
      --)
        shift
        break
        ;;
      *)
        fail "unknown argument: $1"
        ;;
    esac
  done
}

generate_cluster_id() {
  local rand
  if command -v hexdump >/dev/null 2>&1; then
    rand="$(hexdump -vn8 -e ' /1 "%02x"' /dev/urandom 2>/dev/null)"
  fi
  if [[ -z "$rand" ]]; then
    rand="$(date +%s)"
  fi
  printf 'cluster-%s' "$rand"
}
