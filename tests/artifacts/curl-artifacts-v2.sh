#!/usr/bin/env bash
set -euo pipefail

# E2E helper: upload and download an artifact via HTTPS-only /v2 endpoints.
# Requires: CA PEM at $CA_PEM, API host in $API_HOST (api.<cluster>), optional token in $AUTH.

CA_PEM=${CA_PEM:-./ca.pem}
API_HOST=${API_HOST:-api.example.ploy}

if [[ ! -s "$CA_PEM" ]]; then
  echo "CA_PEM missing or empty: $CA_PEM" >&2
  exit 1
fi

echo 'hello-https' > payload.bin
digest="sha256:$( (sha256sum payload.bin 2>/dev/null || shasum -a 256 payload.bin) | awk '{print $1}')"

url="https://${API_HOST}/v2/artifacts/upload?job_id=e2e-https&kind=report&digest=${digest}"
authHeader=()
if [[ -n "${AUTH:-}" ]]; then authHeader=(-H "Authorization: Bearer ${AUTH}"); fi

resp=$(curl -sS --cacert "$CA_PEM" -X POST "$url" -H 'Content-Type: application/octet-stream' "${authHeader[@]}" --data-binary @payload.bin)
echo "$resp" | jq . >/dev/null 2>&1 || { echo "$resp"; echo "non-JSON response" >&2; exit 2; }
id=$(echo "$resp" | jq -r '.artifact.id // empty')
[[ -n "$id" ]] || { echo "missing artifact id" >&2; echo "$resp"; exit 2; }

curl -sS --cacert "$CA_PEM" "https://${API_HOST}/v2/artifacts/${id}?download=true" ${authHeader[@]} -o download.bin
echo "OK: uploaded+downloaded via HTTPS (artifact ${id})"

