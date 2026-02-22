#!/usr/bin/env bash
set -euo pipefail

# ensure_local_descriptor recreates the local default cluster descriptor when it
# is missing or broken. It uses deploy/local/generated-tokens.env produced by
# deploy/local/run.sh.
ensure_local_descriptor() {
  local repo_root="${1:?repo_root is required}"
  local config_home="${2:?config_home is required}"
  local clusters_dir="${config_home}/clusters"
  local marker="${clusters_dir}/default"
  local descriptor_path=""
  local server_url=""
  local cluster_id="${CLUSTER_ID:-local}"
  local token=""
  local generated_tokens=""

  if descriptor_marker_valid "$marker" "$clusters_dir"; then
    descriptor_path="$(resolve_descriptor_path "$marker" "$clusters_dir")"
    server_url="$(descriptor_value "$descriptor_path" "address")"
    token="$(descriptor_value "$descriptor_path" "token")"
    if token_works "$server_url" "$token"; then
      return 0
    fi
    echo "[e2e] Existing descriptor token is invalid; attempting recovery..." >&2
    cluster_id="$(descriptor_value "$descriptor_path" "cluster_id")"
    if [[ -z "$cluster_id" ]]; then
      cluster_id="local"
    fi
  fi

  if [[ -z "$server_url" ]]; then
    server_url="${PLOY_SERVER_URL:-http://localhost:${PLOY_SERVER_PORT:-8080}}"
  fi

  generated_tokens="${repo_root}/deploy/local/generated-tokens.env"
  if [[ -f "$generated_tokens" ]]; then
    # shellcheck disable=SC1090
    source "$generated_tokens"
    if [[ -n "${ADMIN_TOKEN:-}" ]]; then
      write_descriptor "$clusters_dir" "$marker" "$cluster_id" "$server_url" "$ADMIN_TOKEN"
      if token_works "$server_url" "$ADMIN_TOKEN"; then
        return 0
      fi
      echo "[e2e] Token from ${generated_tokens} is invalid for ${server_url}; trying minted token..." >&2
    fi
  fi

  if token="$(mint_valid_local_admin_token "$server_url" "$cluster_id" "$repo_root")"; then
    write_descriptor "$clusters_dir" "$marker" "$cluster_id" "$server_url" "$token"
    return 0
  fi

  echo "error: failed to prepare a valid local descriptor at ${marker}" >&2
  echo "hint: run ./deploy/local/run.sh to reprovision local cluster credentials" >&2
  return 1
}

descriptor_marker_valid() {
  local marker="$1"
  local clusters_dir="$2"

  if [[ -f "$marker" ]]; then
    return 0
  fi

  if [[ ! -L "$marker" ]]; then
    return 1
  fi

  local target
  target="$(readlink "$marker" || true)"
  if [[ -z "$target" ]]; then
    return 1
  fi

  if [[ "$target" = /* ]]; then
    [[ -f "$target" ]]
    return
  fi

  [[ -f "${clusters_dir}/${target}" ]]
}

resolve_descriptor_path() {
  local marker="$1"
  local clusters_dir="$2"
  local target=""

  if [[ -L "$marker" ]]; then
    target="$(readlink "$marker")"
    if [[ "$target" = /* ]]; then
      printf '%s\n' "$target"
      return
    fi
    printf '%s\n' "${clusters_dir}/${target}"
    return
  fi

  printf '%s\n' "$marker"
}

descriptor_value() {
  local descriptor_path="$1"
  local key="$2"
  python3 - "$descriptor_path" "$key" <<'PY'
import json, sys
path, key = sys.argv[1], sys.argv[2]
try:
    with open(path, "r", encoding="utf-8") as f:
        data = json.load(f)
except Exception:
    print("")
    raise SystemExit(0)
value = data.get(key, "")
print(value if isinstance(value, str) else "")
PY
}

write_descriptor() {
  local clusters_dir="$1"
  local marker="$2"
  local cluster_id="$3"
  local server_url="$4"
  local token="$5"

  mkdir -p "$clusters_dir"
  cat > "${clusters_dir}/local.json" <<JSON
{
  "cluster_id": "${cluster_id}",
  "address": "${server_url}",
  "token": "${token}"
}
JSON
  ln -sf local.json "$marker"
  echo "[e2e] Rebuilt default descriptor at ${marker}" >&2
}

token_works() {
  local server_url="$1"
  local token="$2"
  local body=""
  local code=""

  if [[ -z "$server_url" || -z "$token" ]]; then
    return 1
  fi

  body="$(mktemp)"
  code="$(curl -sS -m 3 -o "$body" -w '%{http_code}' \
    -H "Authorization: Bearer ${token}" \
    "${server_url}/v1/mods" || true)"

  if [[ "$code" == "200" ]]; then
    rm -f "$body"
    return 0
  fi

  if grep -q "invalid token" "$body"; then
    rm -f "$body"
    return 1
  fi

  rm -f "$body"
  return 1
}

mint_valid_local_admin_token() {
  local server_url="$1"
  local cluster_id="$2"
  local repo_root="$3"
  local secret=""
  local token=""
  local -a secrets=()

  if [[ -n "${PLOY_AUTH_SECRET:-}" ]]; then
    secrets+=("${PLOY_AUTH_SECRET}")
  fi
  if [[ -f "${repo_root}/deploy/local/auth-secret.txt" ]]; then
    secrets+=("$(cat "${repo_root}/deploy/local/auth-secret.txt")")
  fi

  local container_secret=""
  container_secret="$(running_server_secret || true)"
  if [[ -n "$container_secret" ]]; then
    secrets+=("$container_secret")
  fi

  # Local Docker compose default when PLOY_AUTH_SECRET is unset.
  secrets+=("changeme-insecure-local-secret")

  for secret in "${secrets[@]}"; do
    [[ -z "$secret" ]] && continue
    token="$(mint_admin_token "$secret" "$cluster_id")"
    if token_works "$server_url" "$token"; then
      printf '%s\n' "$token"
      return 0
    fi
  done

  return 1
}

running_server_secret() {
  if command -v docker >/dev/null 2>&1; then
    docker inspect local_server_1 --format '{{range .Config.Env}}{{println .}}{{end}}' 2>/dev/null \
      | sed -n 's/^PLOY_AUTH_SECRET=//p' \
      | head -n 1
    return 0
  fi

  return 0
}

mint_admin_token() {
  local secret="$1"
  local cluster_id="$2"
  python3 - "$secret" "$cluster_id" <<'PY'
import base64, hashlib, hmac, json, secrets, sys, time
secret = sys.argv[1].encode("utf-8")
cluster_id = sys.argv[2]
now = int(time.time())
header = {"alg": "HS256", "typ": "JWT"}
payload = {
    "cluster_id": cluster_id,
    "role": "cli-admin",
    "token_type": "api",
    "iat": now,
    "exp": now + 365 * 24 * 60 * 60,
    "jti": secrets.token_urlsafe(16),
}
def enc(obj):
    return base64.urlsafe_b64encode(
        json.dumps(obj, separators=(",", ":")).encode("utf-8")
    ).decode("utf-8").rstrip("=")
head = enc(header)
body = enc(payload)
sig = base64.urlsafe_b64encode(
    hmac.new(secret, f"{head}.{body}".encode("utf-8"), hashlib.sha256).digest()
).decode("utf-8").rstrip("=")
print(f"{head}.{body}.{sig}")
PY
}
