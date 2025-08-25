#!/bin/bash

# DNS Propagation Test Script
# Tests DNS propagation for dev environment domains

set -e

echo "Testing DNS Propagation for Dev Environment"
echo "==========================================="

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Configuration
BASE_DOMAIN="${PLOY_APPS_DOMAIN:-ployd.app}"
DEV_SUBDOMAIN="${PLOY_DEV_SUBDOMAIN:-dev}"
DEV_DOMAIN="$DEV_SUBDOMAIN.$BASE_DOMAIN"
EXPECTED_IP="${TARGET_IP:-45.12.75.241}"

echo -e "${BLUE}Testing domains:${NC}"
echo "  Dev domain: $DEV_DOMAIN"
echo "  Controller: api.$DEV_DOMAIN"
echo "  Expected IP: $EXPECTED_IP"
echo ""

# Test dev domain
echo -e "${YELLOW}Testing $DEV_DOMAIN...${NC}"
RESOLVED_IP=$(dig +short "$DEV_DOMAIN" @8.8.8.8 | tail -n1)
if [ "$RESOLVED_IP" = "$EXPECTED_IP" ]; then
    echo -e "${GREEN}✓ $DEV_DOMAIN resolves to $RESOLVED_IP${NC}"
else
    echo -e "${RED}✗ $DEV_DOMAIN does not resolve correctly${NC}"
    echo "  Expected: $EXPECTED_IP"
    echo "  Got: $RESOLVED_IP"
fi

# Test controller subdomain
echo -e "${YELLOW}Testing api.$DEV_DOMAIN...${NC}"
CONTROLLER_IP=$(dig +short "api.$DEV_DOMAIN" @8.8.8.8 | tail -n1)
if [ "$CONTROLLER_IP" = "$EXPECTED_IP" ]; then
    echo -e "${GREEN}✓ api.$DEV_DOMAIN resolves to $CONTROLLER_IP${NC}"
else
    echo -e "${RED}✗ api.$DEV_DOMAIN does not resolve correctly${NC}"
    echo "  Expected: $EXPECTED_IP"
    echo "  Got: $CONTROLLER_IP"
fi

# Test sample app subdomain
echo -e "${YELLOW}Testing myapp.$DEV_DOMAIN...${NC}"
APP_IP=$(dig +short "myapp.$DEV_DOMAIN" @8.8.8.8 | tail -n1)
if [ "$APP_IP" = "$EXPECTED_IP" ]; then
    echo -e "${GREEN}✓ myapp.$DEV_DOMAIN resolves to $APP_IP (wildcard working)${NC}"
else
    echo -e "${RED}✗ myapp.$DEV_DOMAIN does not resolve correctly${NC}"
    echo "  Expected: $EXPECTED_IP"
    echo "  Got: $APP_IP"
fi

# Test multiple DNS servers
echo ""
echo -e "${YELLOW}Testing propagation across DNS servers...${NC}"
DNS_SERVERS=(
    "8.8.8.8"      # Google
    "1.1.1.1"      # Cloudflare
    "208.67.222.222" # OpenDNS
)

for dns_server in "${DNS_SERVERS[@]}"; do
    echo -n "  $dns_server: "
    resolved=$(dig +short "api.$DEV_DOMAIN" "@$dns_server" | tail -n1)
    if [ "$resolved" = "$EXPECTED_IP" ]; then
        echo -e "${GREEN}✓ $resolved${NC}"
    else
        echo -e "${RED}✗ $resolved${NC}"
    fi
done

echo ""
echo -e "${BLUE}Summary:${NC}"
if [ "$RESOLVED_IP" = "$EXPECTED_IP" ] && [ "$CONTROLLER_IP" = "$EXPECTED_IP" ] && [ "$APP_IP" = "$EXPECTED_IP" ]; then
    echo -e "${GREEN}✓ DNS propagation complete! All domains resolve correctly.${NC}"
    echo ""
    echo "Ready for wildcard certificate provisioning!"
    echo "Run: ./scripts/deploy-with-ssl.sh"
    exit 0
else
    echo -e "${YELLOW}⚠ DNS propagation still in progress.${NC}"
    echo ""
    echo "Please wait a few more minutes and try again."
    echo "DNS changes can take up to 24 hours to fully propagate worldwide."
    exit 1
fi