#!/bin/bash

# DNS Setup Script for Dev Environment
# Creates DNS records for *.dev.ployd.app wildcard certificate

set -e

echo "Setting up DNS records for dev environment"
echo "=========================================="

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
# TARGET_HOST should already be set globally

echo -e "${BLUE}Configuration:${NC}"
echo "  Base domain: $BASE_DOMAIN"
echo "  Dev domain: $DEV_DOMAIN"
echo "  VPS IP: $TARGET_HOST"
echo ""

# Check if we have DNS credentials
if [ -z "$NAMECHEAP_API_USER" ] || [ -z "$NAMECHEAP_API_KEY" ]; then
    echo -e "${YELLOW}DNS credentials not found. Manual DNS setup required.${NC}"
    echo ""
    echo "Please add the following DNS records to your Namecheap control panel:"
    echo ""
    echo "  Type: A"
    echo "  Host: $DEV_SUBDOMAIN"
    echo "  Value: $TARGET_HOST"
    echo "  TTL: 300 (5 minutes)"
    echo ""
    echo "  Type: A"
    echo "  Host: *.$DEV_SUBDOMAIN"
    echo "  Value: $TARGET_HOST"
    echo "  TTL: 300 (5 minutes)"
    echo ""
    echo "This will create:"
    echo "  - $DEV_DOMAIN → $TARGET_HOST"
    echo "  - api.$DEV_DOMAIN → $TARGET_HOST"
    echo "  - {app}.$DEV_DOMAIN → $TARGET_HOST (wildcard)"
    echo ""
    echo "After adding these records, wait 5-10 minutes for DNS propagation."
    echo "Then run: ./scripts/test-dns-propagation.sh"
    exit 0
fi

# Automatic DNS setup using Namecheap API
echo -e "${YELLOW}Setting up DNS records automatically...${NC}"

# Build the ploy CLI to use DNS management
if [ ! -f "bin/ploy" ]; then
    echo "Building Ploy CLI..."
    ./scripts/build.sh cli
fi

# Add dev subdomain A record
echo -e "${YELLOW}Adding A record for $DEV_DOMAIN...${NC}"
./bin/ploy domains add-dns "$DEV_DOMAIN" "$TARGET_HOST" --type A --ttl 300

if [ $? -eq 0 ]; then
    echo -e "${GREEN}✓ A record added: $DEV_DOMAIN → $TARGET_HOST${NC}"
else
    echo -e "${RED}✗ Failed to add A record for $DEV_DOMAIN${NC}"
    exit 1
fi

# Add wildcard A record
echo -e "${YELLOW}Adding wildcard A record for *.$DEV_DOMAIN...${NC}"
./bin/ploy domains add-dns "*.$DEV_DOMAIN" "$TARGET_HOST" --type A --ttl 300

if [ $? -eq 0 ]; then
    echo -e "${GREEN}✓ Wildcard A record added: *.$DEV_DOMAIN → $TARGET_HOST${NC}"
else
    echo -e "${RED}✗ Failed to add wildcard A record${NC}"
    exit 1
fi

echo ""
echo -e "${GREEN}DNS setup complete!${NC}"
echo ""
echo "Records created:"
echo "  $DEV_DOMAIN → $TARGET_HOST"
echo "  *.$DEV_DOMAIN → $TARGET_HOST"
echo ""
echo "This enables:"
echo "  - api.$DEV_DOMAIN (controller endpoint)"
echo "  - {app}.$DEV_DOMAIN (app endpoints)"
echo ""
echo "Waiting for DNS propagation (this may take 5-10 minutes)..."
echo "Run: ./scripts/test-dns-propagation.sh to check status"