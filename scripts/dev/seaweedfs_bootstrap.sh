#!/usr/bin/env bash
# Bootstrap SeaweedFS collections and directories for local development.
set -euo pipefail

SEAWEED_BIN=${SEAWEED_BIN:-$(command -v weed || true)}
SEAWEED_MASTER=${SEAWEED_MASTER:-localhost:9333}
SEAWEED_FILER=${SEAWEED_FILER:-http://localhost:8888}
COLLECTIONS=("test-collection" "artifacts" "test-bucket")

if [[ -z "${SEAWEED_BIN}" ]]; then
  echo "error: weed binary not found in PATH" >&2
  exit 1
fi

if ! curl -fsS --connect-timeout 2 "${SEAWEED_FILER}/?pretty=y" >/dev/null; then
  echo "error: SeaweedFS filer at ${SEAWEED_FILER} is not reachable. Start SeaweedFS first." >&2
  exit 2
fi

echo "Bootstrapping SeaweedFS collections via filer API..."
for collection in "${COLLECTIONS[@]}"; do
  "${SEAWEED_BIN}" shell -master="${SEAWEED_MASTER}" >/dev/null <<EOF
fs.configure -locationPrefix=/${collection} -collection=${collection} -replication=000 -apply
fs.configure -locationPrefix=/${collection}/tmp -collection=${collection} -replication=000 -apply
fs.configure -locationPrefix=/${collection}/scratch -collection=${collection} -replication=000 -apply
exit
EOF
  for subdir in "" "tmp" "scratch"; do
    path="${collection}"
    if [[ -n "${subdir}" ]]; then
      path+="/${subdir}"
    fi
    http_code=$(curl -s -o /dev/null -w "%{http_code}" -X POST "${SEAWEED_FILER}/${path}/") || http_code="000"
    case "${http_code}" in
      200|201|202|204|409)
        echo "  ensured directory /${path} (status ${http_code})"
        ;;
      *)
        echo "warning: failed to create /${path} (status ${http_code})" >&2
        ;;
    esac
  done
done

# Sanity check: ensure top-level directories now exist
for collection in "${COLLECTIONS[@]}"; do
  if ! curl -fsS --head "${SEAWEED_FILER}/${collection}/" >/dev/null; then
    echo "warning: collection ${collection} not reachable via filer at ${SEAWEED_FILER}" >&2
  fi
done

echo "SeaweedFS bootstrap complete."
