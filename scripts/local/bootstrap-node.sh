#!/usr/bin/env bash
set -euo pipefail

# Insert the local node record into Postgres via the running compose stack.
#
# UUID is fixed by request. Other attributes can be overridden via env vars.
#
# Env (optional):
#   NODE_NAME          default: local-node-0001
#   NODE_IP            default: 127.0.0.1
#   NODE_VERSION       default: dev
#   NODE_CONCURRENCY   default: 1
#   COMPOSE_CMD        default: "docker compose -f local/docker-compose.yml"

UUID="00000000-0000-0000-0000-000000000001"
NAME="${NODE_NAME:-local-node-0001}"
IP="${NODE_IP:-127.0.0.1}"
VERSION="${NODE_VERSION:-dev}"
CONCURRENCY="${NODE_CONCURRENCY:-1}"
COMPOSE_CMD="${COMPOSE_CMD:-docker compose -f local/docker-compose.yml}"

echo "Inserting node ${UUID} (${NAME} @ ${IP}) into ploy.nodes…" >&2

${COMPOSE_CMD} exec -T db psql -U ploy -d ploy -v ON_ERROR_STOP=1 -c "\
  SET search_path TO ploy, public; \
  INSERT INTO nodes (id, name, ip_address, version, concurrency) \
  VALUES ('${UUID}', '${NAME}', '${IP}', '${VERSION}', ${CONCURRENCY}) \
  ON CONFLICT (id) DO NOTHING;"

echo "Done." >&2

