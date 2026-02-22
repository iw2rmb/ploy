#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "$REPO_ROOT"

COMPOSE_CMD="${COMPOSE_CMD:-docker compose -f local/docker-compose.yml}"
export PLOY_CONFIG_HOME="${PLOY_CONFIG_HOME:-$REPO_ROOT/local/cli}"
PLOY_DB_DSN="${PLOY_DB_DSN:-}"

REPO_URL="${PLOY_E2E_REPO_OVERRIDE:-https://gitlab.com/iw2rmb/ploy-orw-java11-maven.git}"
GARAGE_ENDPOINT="${GARAGE_ENDPOINT:-http://localhost:3900}"
GARAGE_BUCKET="${GARAGE_BUCKET:-ploy}"
GARAGE_ACCESS_KEY="${GARAGE_ACCESS_KEY:-GK000000000000000000000001}"
GARAGE_SECRET_KEY="${GARAGE_SECRET_KEY:-0000000000000000000000000000000000000000000000000000000000000001}"
GARAGE_REGION="${GARAGE_REGION:-garage}"

TS="$(date +%y%m%d%H%M%S)"
ARTIFACT_BASE="${PLOY_E2E_ARTIFACT_BASE:-$REPO_ROOT/tmp/garage-smoke}"
OUT_DIR="${PLOY_E2E_ARTIFACT_DIR:-${ARTIFACT_BASE}/${TS}}"
RUN_ARTIFACT_DIR="${OUT_DIR}/run-artifacts"
FETCH_DIR="${OUT_DIR}/fetch-artifacts"
DIFF_PATH="${OUT_DIR}/latest.patch"
RUN_LOG_PATH="${OUT_DIR}/run-logs.txt"

mkdir -p "$RUN_ARTIFACT_DIR" "$FETCH_DIR"

log() {
  echo "[$(date -u +%H:%M:%S)] $*"
}

need() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "error: missing dependency: $1" >&2
    exit 1
  fi
}

query_object_key() {
  local table="$1"
  local order_by="$2"

  psql "$PLOY_DB_DSN" -Atq -c "SET search_path TO ploy, public; SELECT object_key FROM ${table} WHERE run_id='${RUN_ID}' AND object_key IS NOT NULL ORDER BY ${order_by} LIMIT 1;"
}

verify_garage_keys() {
  local tmp_go
  tmp_go="$(mktemp -t ploy-garage-head-XXXXXX.go)"

  cat > "$tmp_go" <<'GOCODE'
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
)

func required(name string) string {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		fmt.Fprintf(os.Stderr, "missing required env: %s\n", name)
		os.Exit(2)
	}
	return v
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: headcheck <key> [<key> ...]")
		os.Exit(2)
	}

	endpoint := required("GARAGE_ENDPOINT")
	bucket := required("GARAGE_BUCKET")
	access := required("GARAGE_ACCESS_KEY")
	secret := required("GARAGE_SECRET_KEY")
	region := required("GARAGE_REGION")

	ctx := context.Background()
	cfg, err := awsconfig.LoadDefaultConfig(
		ctx,
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(access, secret, "")),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load aws config: %v\n", err)
		os.Exit(1)
	}

	client := awss3.NewFromConfig(cfg, func(o *awss3.Options) {
		o.UsePathStyle = true
		o.BaseEndpoint = aws.String(endpoint)
	})

	for _, key := range os.Args[1:] {
		if _, err := client.HeadObject(ctx, &awss3.HeadObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(strings.TrimSpace(key)),
		}); err != nil {
			fmt.Fprintf(os.Stderr, "head object %s: %v\n", key, err)
			os.Exit(1)
		}
		fmt.Printf("garage head ok: %s\n", key)
	}
}
GOCODE

  GARAGE_ENDPOINT="$GARAGE_ENDPOINT" \
  GARAGE_BUCKET="$GARAGE_BUCKET" \
  GARAGE_ACCESS_KEY="$GARAGE_ACCESS_KEY" \
  GARAGE_SECRET_KEY="$GARAGE_SECRET_KEY" \
  GARAGE_REGION="$GARAGE_REGION" \
    go run "$tmp_go" "$@"

  rm -f "$tmp_go"
}

need docker
need jq
need go
need rg
need psql

if [[ -z "$PLOY_DB_DSN" ]]; then
  echo "error: PLOY_DB_DSN is required (example: postgres://ploy:ploy@host.containers.internal:5432/ploy?sslmode=disable)" >&2
  exit 1
fi

log "Deploying local stack"
"$REPO_ROOT/scripts/local-docker.sh"

MOD_CMD=$(cat <<EOC
sh -lc 'echo "[garage-smoke] start"; cd /workspace; target="\$(git ls-files | head -n 1)"; if [ -z "\$target" ]; then echo "no tracked files" >&2; exit 1; fi; echo "garage-smoke-${TS}" >> "\$target"; mkdir -p /out; echo "garage-smoke-artifact-${TS}" > /out/garage-smoke.txt; echo "[garage-smoke] done"'
EOC
)

log "Running deterministic smoke mod"
RUN_JSON="$($REPO_ROOT/dist/ploy mod run --json \
  --repo-url "$REPO_URL" \
  --repo-base-ref main \
  --repo-target-ref "e2e/garage-smoke-${TS}" \
  --mod-image alpine:3.20 \
  --mod-command "$MOD_CMD" \
  --follow \
  --artifact-dir "$RUN_ARTIFACT_DIR")"

RUN_ID="$(printf '%s' "$RUN_JSON" | jq -r '.run_id // empty')"
if [[ -z "$RUN_ID" ]]; then
  echo "error: could not parse run_id from mod run response" >&2
  printf '%s\n' "$RUN_JSON" >&2
  exit 1
fi

log "Run ID: ${RUN_ID}"

log "Reading run logs stream"
$REPO_ROOT/dist/ploy run logs "$RUN_ID" --timeout 8s --idle-timeout 3s --max-retries 0 >"$RUN_LOG_PATH" 2>&1 || true
if ! rg -q "garage-smoke" "$RUN_LOG_PATH"; then
  echo "error: expected garage-smoke logs in $RUN_LOG_PATH" >&2
  exit 1
fi

log "Downloading artifacts via CLI"
$REPO_ROOT/dist/ploy mod fetch --run "$RUN_ID" --artifact-dir "$FETCH_DIR"
if [[ ! -f "$FETCH_DIR/manifest.json" ]]; then
  echo "error: expected artifact manifest at $FETCH_DIR/manifest.json" >&2
  exit 1
fi

log "Downloading latest diff via CLI"
$REPO_ROOT/dist/ploy run diff --download --output "$DIFF_PATH" "$RUN_ID"
if [[ ! -s "$DIFF_PATH" ]]; then
  echo "error: expected non-empty diff at $DIFF_PATH" >&2
  exit 1
fi

log "Querying object keys from DB"
LOG_KEY="$(query_object_key logs id | tr -d '\r' | tail -n 1)"
DIFF_KEY="$(query_object_key diffs created_at | tr -d '\r' | tail -n 1)"
ARTIFACT_KEY="$(query_object_key artifact_bundles created_at | tr -d '\r' | tail -n 1)"

if [[ -z "$LOG_KEY" || -z "$DIFF_KEY" || -z "$ARTIFACT_KEY" ]]; then
  echo "error: missing object keys (log='$LOG_KEY', diff='$DIFF_KEY', artifact='$ARTIFACT_KEY')" >&2
  exit 1
fi

log "Verifying objects exist in Garage bucket"
verify_garage_keys "$LOG_KEY" "$DIFF_KEY" "$ARTIFACT_KEY"

log "Smoke passed"
log "Artifacts: $OUT_DIR"
