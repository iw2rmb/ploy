#!/bin/bash

# Test SSL Deployment for Dev Environment
# Simulates the complete SSL wildcard certificate deployment process

set -e

echo "Testing SSL Deployment for Dev Environment"
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
VPS_IP="${TARGET_IP:-45.12.75.241}"

echo -e "${BLUE}Testing Configuration:${NC}"
echo "  Base domain: $BASE_DOMAIN"
echo "  Dev domain: $DEV_DOMAIN"
echo "  Controller: api.$DEV_DOMAIN"
echo "  Wildcard: *.$DEV_DOMAIN"
echo "  VPS IP: $VPS_IP"
echo ""

# Test 1: Controller environment variables
echo -e "${YELLOW}Test 1: Checking controller environment...${NC}"
CONTROLLER_ENV=$(ssh root@$VPS_IP "ALLOC_ID=\$(/opt/hashicorp/bin/nomad-job-manager.sh running-alloc ploy-api 2>/dev/null) && nomad alloc exec \"\$ALLOC_ID\" env | grep PLOY_" 2>/dev/null || echo "failed")

if echo "$CONTROLLER_ENV" | grep -q "PLOY_ENVIRONMENT=dev"; then
    echo -e "${GREEN}✓ Controller has dev environment variables${NC}"
else
    echo -e "${RED}✗ Controller missing dev environment${NC}"
    echo "Controller environment: $CONTROLLER_ENV"
fi

# Test 2: Reserved app names
echo -e "${YELLOW}Test 2: Testing app name protection...${NC}"
CONTROLLER_PORT=$(ssh root@$VPS_IP "ALLOC_ID=\$(/opt/hashicorp/bin/nomad-job-manager.sh running-alloc ploy-api 2>/dev/null) && /opt/hashicorp/bin/nomad-job-manager.sh alloc-status \"\$ALLOC_ID\" 2>/dev/null | jq -r '.Resources.Networks[0].DynamicPorts[] | select(.Label == \"http\") | .Value // empty'" 2>/dev/null)

if [ -n "$CONTROLLER_PORT" ]; then
    API_RESPONSE=$(ssh root@$VPS_IP "curl -s -X POST http://localhost:$CONTROLLER_PORT/v1/apps/api/builds -H 'Content-Type: application/tar' --data-binary @/dev/null")
    if echo "$API_RESPONSE" | grep -q "reserved"; then
        echo -e "${GREEN}✓ App name 'api' is protected${NC}"
    else
        echo -e "${RED}✗ App name protection not working${NC}"
    fi
else
    echo -e "${RED}✗ Could not find controller port${NC}"
fi

# Test 3: Version endpoints
echo -e "${YELLOW}Test 3: Testing version endpoints...${NC}"
if [ -n "$CONTROLLER_PORT" ]; then
    VERSION_RESPONSE=$(ssh root@$VPS_IP "curl -s http://localhost:$CONTROLLER_PORT/version")
    if echo "$VERSION_RESPONSE" | grep -q "version"; then
        echo -e "${GREEN}✓ Version endpoint working${NC}"
        echo "  Version: $(echo "$VERSION_RESPONSE" | jq -r .version 2>/dev/null)"
    else
        echo -e "${RED}✗ Version endpoint not working${NC}"
    fi
fi

# Test 4: Platform certificate manager
echo -e "${YELLOW}Test 4: Testing platform certificate manager...${NC}"
if [ -n "$CONTROLLER_PORT" ]; then
    CERT_RESPONSE=$(ssh root@$VPS_IP "curl -s http://localhost:$CONTROLLER_PORT/health/platform-certificates" 2>/dev/null || echo "failed")
    if echo "$CERT_RESPONSE" | grep -q "platform\|certificate"; then
        echo -e "${GREEN}✓ Platform certificate manager accessible${NC}"
    else
        echo -e "${YELLOW}⚠ Platform certificate manager not responding${NC}"
        echo "  Response: $CERT_RESPONSE"
    fi
fi

# Test 5: DNS resolution (current state)
echo -e "${YELLOW}Test 5: Checking current DNS resolution...${NC}"
CURRENT_DEV_IP=$(dig +short "$DEV_DOMAIN" | tail -n1)
CURRENT_API_IP=$(dig +short "api.$DEV_DOMAIN" | tail -n1)

echo "  Current DNS:"
echo "    $DEV_DOMAIN → $CURRENT_DEV_IP"
echo "    api.$DEV_DOMAIN → $CURRENT_API_IP"
echo "  Expected: $VPS_IP"

if [ "$CURRENT_DEV_IP" = "$VPS_IP" ] && [ "$CURRENT_API_IP" = "$VPS_IP" ]; then
    echo -e "${GREEN}✓ DNS records are correct${NC}"
    DNS_READY=true
else
    echo -e "${YELLOW}⚠ DNS records need updating${NC}"
    DNS_READY=false
fi

# Test 6: SSL prerequisites
echo -e "${YELLOW}Test 6: SSL deployment prerequisites...${NC}"

# Check if all environment variables are set for SSL
SSL_ENV_CHECK=true
if [ -z "$NAMECHEAP_API_USER" ]; then
    echo -e "${YELLOW}⚠ NAMECHEAP_API_USER not set${NC}"
    SSL_ENV_CHECK=false
fi

if [ -z "$NAMECHEAP_API_KEY" ]; then
    echo -e "${YELLOW}⚠ NAMECHEAP_API_KEY not set${NC}"
    SSL_ENV_CHECK=false
fi

if [ -z "$NAMECHEAP_USERNAME" ]; then
    echo -e "${YELLOW}⚠ NAMECHEAP_USERNAME not set${NC}"
    SSL_ENV_CHECK=false
fi

if [ "$SSL_ENV_CHECK" = true ]; then
    echo -e "${GREEN}✓ DNS API credentials are set${NC}"
else
    echo -e "${YELLOW}⚠ DNS API credentials need to be set${NC}"
fi

# Summary
echo ""
echo -e "${BLUE}Summary:${NC}"
echo "=========="

if [ "$DNS_READY" = true ] && [ "$SSL_ENV_CHECK" = true ]; then
    echo -e "${GREEN}✅ Ready for SSL deployment!${NC}"
    echo ""
    echo "Run: ./scripts/deploy-with-ssl.sh"
    echo ""
    echo "Expected results:"
    echo "  - Wildcard certificate: *.dev.ployd.app"
    echo "  - Controller HTTPS: https://api.dev.ployman.app"
    echo "  - App HTTPS pattern: https://{app}.dev.ployd.app"
else
    echo -e "${YELLOW}⚠ Prerequisites need attention:${NC}"
    echo ""
    
    if [ "$DNS_READY" = false ]; then
        echo "📋 DNS Update Required:"
        echo "  1. Log into Namecheap control panel"
        echo "  2. Update *.dev.ployd.app records to $VPS_IP"
        echo "  3. Wait for DNS propagation (5-10 minutes)"
        echo ""
    fi
    
    if [ "$SSL_ENV_CHECK" = false ]; then
        echo "🔑 API Credentials Required:"
        echo "  export NAMECHEAP_API_USER=\"your-api-user\""
        echo "  export NAMECHEAP_API_KEY=\"your-api-key\""
        echo "  export NAMECHEAP_USERNAME=\"your-username\""
        echo ""
    fi
    
    echo "After completing prerequisites, run:"
    echo "  ./scripts/test-ssl-deployment.sh  # Verify readiness"
    echo "  ./scripts/deploy-with-ssl.sh     # Deploy SSL"
fi

echo ""
echo -e "${GREEN}SSL deployment test complete!${NC}"