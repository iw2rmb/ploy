#!/usr/bin/env bash
set -euo pipefail

# ensure_local_descriptor prepares env-only CLI auth for legacy e2e callers.
# It keeps the historical function name because multiple e2e scripts source it.
ensure_local_descriptor() {
  local repo_root="${1:?repo_root is required}"
  local base_dir="${2:?config_home is required}"
  local server_url="${PLOY_SERVER_URL:-http://localhost:${PLOY_SERVER_PORT:-8080}}"
  local token="${PLOY_AUTH_TOKEN:-}"
  local generated_tokens="${base_dir}/generated-tokens.env"

  if token_works "$server_url" "$token"; then
    export PLOY_SERVER_URL="$server_url"
    export PLOY_AUTH_TOKEN="$token"
    return 0
  fi

  if [[ -f "$generated_tokens" ]]; then
    # shellcheck disable=SC1090
    source "$generated_tokens"
    if token_works "$server_url" "${ADMIN_TOKEN:-}"; then
      export PLOY_SERVER_URL="$server_url"
      export PLOY_AUTH_TOKEN="$ADMIN_TOKEN"
      return 0
    fi
    echo "[e2e] Token from ${generated_tokens} is invalid for ${server_url}; trying minted token..." >&2
  fi

  if token="$(mint_valid_local_admin_token "$server_url" "$repo_root" "$base_dir")"; then
    mkdir -p "$base_dir"
    {
      printf 'ADMIN_TOKEN=%q\n' "$token"
      printf 'PLOY_SERVER_URL=%q\n' "$server_url"
    } > "$generated_tokens"
    export PLOY_SERVER_URL="$server_url"
    export PLOY_AUTH_TOKEN="$token"
    echo "[e2e] Prepared PLOY_SERVER_URL and PLOY_AUTH_TOKEN for ${server_url}" >&2
    return 0
  fi

  echo "error: failed to prepare valid local CLI auth for ${server_url}" >&2
  echo "hint: start the local stack with /Users/v.v.kovalev/@scale/ploy-lib/images/docker-compose.yml and retry" >&2
  return 1
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
    "${server_url}/v1/migs" || true)"

  rm -f "$body"
  [[ "$code" == "200" ]]
}

mint_valid_local_admin_token() {
  local server_url="$1"
  local repo_root="$2"
  local base_dir="$3"
  local secret=""
  local token=""
  local -a secrets=()

  if [[ -n "${PLOY_AUTH_SECRET:-}" ]]; then
    secrets+=("${PLOY_AUTH_SECRET}")
  fi
  if [[ -f "${base_dir}/auth-secret.txt" ]]; then
    secrets+=("$(cat "${base_dir}/auth-secret.txt")")
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
    token="$(mint_admin_token "$secret")"
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
  python3 - "$secret" <<'PY'
import base64, hashlib, hmac, json, secrets, sys, time

secret = sys.argv[1].encode("utf-8")
now = int(time.time())
header = {"alg": "HS256", "typ": "JWT"}
payload = {
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
