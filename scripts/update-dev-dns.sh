#!/bin/bash

# Update DNS Records for Dev Environment
# Updates existing *.dev.ployd.app records to point to VPS IP

set -e

echo "Updating DNS Records for Dev Environment"
echo "========================================"

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

# Get controller endpoint
CONTROLLER_PORT=$(ssh root@$TARGET_HOST "ALLOC_ID=\$(/opt/hashicorp/bin/nomad-job-manager.sh running-alloc ploy-api 2>/dev/null) && /opt/hashicorp/bin/nomad-job-manager.sh alloc-status \"\$ALLOC_ID\" 2>/dev/null | jq -r '.Resources.Networks[0].DynamicPorts[] | select(.Label == \"http\") | .Value // empty'" 2>/dev/null || echo "")

if [ -z "$CONTROLLER_PORT" ]; then
    echo -e "${RED}✗ Could not find running controller${NC}"
    echo "Make sure the controller is running on the VPS."
    exit 1
fi

CONTROLLER_URL="http://$TARGET_HOST:$CONTROLLER_PORT"
echo "Controller URL: $CONTROLLER_URL"

# Check if DNS API is available
echo -e "${YELLOW}Testing DNS API access...${NC}"
DNS_STATUS=$(curl -s "$CONTROLLER_URL/v1/dns/status" || echo "failed")

if echo "$DNS_STATUS" | grep -q "dns_provider.*available"; then
    echo -e "${GREEN}✓ DNS API is available${NC}"
else
    echo -e "${RED}✗ DNS API not available or not configured${NC}"
    echo "Response: $DNS_STATUS"
    echo ""
    echo "Manual DNS update required:"
    echo "1. Log into Namecheap control panel"
    echo "2. Go to Domain List → ployd.app → Manage"
    echo "3. Advanced DNS tab"
    echo "4. Update these A records:"
    echo "   Host: dev, Value: $TARGET_HOST"
    echo "   Host: *.dev, Value: $TARGET_HOST"
    exit 1
fi

# Update dev subdomain A record
echo -e "${YELLOW}Updating A record for $DEV_DOMAIN...${NC}"
UPDATE_RESPONSE=$(curl -s -X PUT "$CONTROLLER_URL/v1/dns/records" \
    -H "Content-Type: application/json" \
    -d "{
        \"hostname\": \"$DEV_DOMAIN\",
        \"type\": \"A\",
        \"value\": \"$TARGET_HOST\",
        \"ttl\": 300
    }")

if echo "$UPDATE_RESPONSE" | grep -q "success\|updated"; then
    echo -e "${GREEN}✓ Updated $DEV_DOMAIN → $TARGET_HOST${NC}"
else
    echo -e "${RED}✗ Failed to update $DEV_DOMAIN${NC}"
    echo "Response: $UPDATE_RESPONSE"
fi

# Update wildcard A record
echo -e "${YELLOW}Updating wildcard A record for *.$DEV_DOMAIN...${NC}"
WILDCARD_RESPONSE=$(curl -s -X PUT "$CONTROLLER_URL/v1/dns/records" \
    -H "Content-Type: application/json" \
    -d "{
        \"hostname\": \"*.$DEV_DOMAIN\",
        \"type\": \"A\",
        \"value\": \"$TARGET_HOST\",
        \"ttl\": 300
    }")

if echo "$WILDCARD_RESPONSE" | grep -q "success\|updated"; then
    echo -e "${GREEN}✓ Updated *.$DEV_DOMAIN → $TARGET_HOST${NC}"
else
    echo -e "${RED}✗ Failed to update *.$DEV_DOMAIN${NC}"
    echo "Response: $WILDCARD_RESPONSE"
fi

echo ""
echo -e "${GREEN}DNS update complete!${NC}"
echo ""
echo "Updated records:"
echo "  $DEV_DOMAIN → $TARGET_HOST"
echo "  *.$DEV_DOMAIN → $TARGET_HOST"
echo ""
echo "⏱ DNS propagation typically takes 5-10 minutes."
echo "Run: ./scripts/test-dns-propagation.sh to check status"
echo ""
echo "Once DNS propagates, deploy SSL with:"
echo "  ./scripts/deploy.sh"