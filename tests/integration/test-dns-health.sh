#!/usr/bin/env bash
set -euo pipefail

BLUE='\033[0;34m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

info() { echo -e "${BLUE}INFO: $1${NC}"; }
pass() { echo -e "${GREEN}PASS: $1${NC}"; }
warn() { echo -e "${YELLOW}WARN: $1${NC}"; }
fail() { echo -e "${RED}FAIL: $1${NC}"; exit 1; }

if ! command -v dig >/dev/null 2>&1; then
  fail "dig utility not available; install bindutils"
fi

COREDNS_ADDR="${PLOY_COREDNS_ADDR:-127.0.0.1:1053}"
PLATFORM_DOMAIN="${PLOY_PLATFORM_DOMAIN:-ploy.local}"
TARGET_IP="${TARGET_HOST:-}"

info "Probing CoreDNS at $COREDNS_ADDR for platform zone $PLATFORM_DOMAIN"

lookup() {
  local name="$1" type="$2"
  dig +time=2 +tries=2 "$type" "$name" "@${COREDNS_ADDR}" +nocmd +noall +answer
}

assert_record() {
  local name="$1" type="$2" expect_addr="$3"
  local output
  if ! output=$(lookup "$name" "$type"); then
    warn "lookup failed for $name ($type)"
    return 1
  fi
  if [[ -z "$output" ]]; then
    warn "no records returned for $name ($type)"
    return 1
  fi
  echo "$output"
  if [[ -n "$expect_addr" ]] && ! grep -q "$expect_addr" <<<"$output"; then
    warn "$name ($type) does not resolve to $expect_addr"
    return 1
  fi
  return 0
}

SUMMARY_OK=1

if assert_record "nomad.control.${PLATFORM_DOMAIN}." A "$TARGET_IP"; then
  pass "Nomad control resolves"
else
  SUMMARY_OK=0
fi

if assert_record "seaweedfs-filer.storage.${PLATFORM_DOMAIN}." A "$TARGET_IP"; then
  pass "SeaweedFS filer resolves"
else
  SUMMARY_OK=0
fi

if assert_record "_seaweedfs._tcp.seaweedfs-filer.storage.${PLATFORM_DOMAIN}." SRV ""; then
  pass "SeaweedFS SRV record present"
else
  SUMMARY_OK=0
fi

metrics_url="http://${COREDNS_ADDR%%:*}:9153/metrics"
info "Attempting to scrape CoreDNS Prometheus metrics from $metrics_url"
if command -v curl >/dev/null 2>&1; then
  if curl -fsS "$metrics_url" | grep -q '^coredns_dns_requests_total'; then
    pass "CoreDNS metrics endpoint reachable"
  else
    warn "CoreDNS metrics endpoint missing standard counters"
    SUMMARY_OK=0
  fi
else
  warn "curl not available; skipping metrics scrape"
fi

if [[ "$SUMMARY_OK" -eq 1 ]]; then
  pass "CoreDNS health checks passed"
else
  fail "CoreDNS health checks detected issues"
fi
