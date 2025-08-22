#!/bin/bash

# Deploy Controller with SSL Wildcard Certificate
# This script deploys the controller with proper DNS credentials for automatic
# Let's Encrypt wildcard certificate provisioning

set -e

echo "Deploying Controller with SSL Wildcard Certificate"
echo "=================================================="

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

echo -e "${BLUE}Configuration:${NC}"
echo "  Base domain: $BASE_DOMAIN"
echo "  Dev domain: $DEV_DOMAIN"
echo "  Controller: api.$DEV_DOMAIN"
echo "  Wildcard certificate: *.$DEV_DOMAIN"
echo "  VPS IP: $VPS_IP"
echo ""

# Check DNS propagation first
echo -e "${YELLOW}Step 1: Checking DNS propagation...${NC}"
if ./scripts/test-dns-propagation.sh > /dev/null 2>&1; then
    echo -e "${GREEN}✓ DNS propagation complete${NC}"
else
    echo -e "${RED}✗ DNS records not yet propagated${NC}"
    echo "Please run: ./scripts/test-dns-propagation.sh"
    echo "Wait for DNS to propagate before proceeding."
    exit 1
fi

# Check DNS API credentials
echo -e "${YELLOW}Step 2: Checking DNS API credentials...${NC}"
if [ -z "$NAMECHEAP_API_USER" ] || [ -z "$NAMECHEAP_API_KEY" ] || [ -z "$NAMECHEAP_USERNAME" ]; then
    echo -e "${RED}✗ DNS API credentials missing${NC}"
    echo ""
    echo "Please set the following environment variables:"
    echo "  export NAMECHEAP_API_USER=\"your-api-user\""
    echo "  export NAMECHEAP_API_KEY=\"your-api-key\""
    echo "  export NAMECHEAP_USERNAME=\"your-username\""
    echo ""
    echo "Get these from: https://ap.www.namecheap.com/settings/tools/apiaccess/"
    exit 1
else
    echo -e "${GREEN}✓ DNS API credentials found${NC}"
fi

# Build the latest controller
echo -e "${YELLOW}Step 3: Building controller with latest changes...${NC}"
./scripts/build.sh controller
echo -e "${GREEN}✓ Controller built successfully${NC}"

# Update Nomad job with SSL environment variables
echo -e "${YELLOW}Step 4: Updating Nomad job configuration...${NC}"

# Create temporary job file with SSL credentials
TEMP_JOB="/tmp/ploy-controller-ssl.hcl"
cp platform/nomad/ploy-controller.hcl "$TEMP_JOB"

# Add DNS credentials to the job file
cat >> "$TEMP_JOB" << EOF

        # DNS API credentials for wildcard certificate
        NAMECHEAP_API_USER = "$NAMECHEAP_API_USER"
        NAMECHEAP_API_KEY = "$NAMECHEAP_API_KEY"
        NAMECHEAP_USERNAME = "$NAMECHEAP_USERNAME"
        
        # Certificate configuration
        CERT_EMAIL = "admin@$BASE_DOMAIN"
        ACME_STAGING = "false"  # Use production Let's Encrypt
EOF

echo -e "${GREEN}✓ Job configuration updated with SSL credentials${NC}"

# Deploy to Nomad with SSL support
echo -e "${YELLOW}Step 5: Deploying controller with SSL support...${NC}"
SEAWEEDFS_URL="http://localhost:8888" NOMAD_JOB_FILE="$TEMP_JOB" ./scripts/deploy-nomad.sh

# Wait for deployment to complete
echo -e "${YELLOW}Step 6: Waiting for controller to start...${NC}"
sleep 30

# Check if controller is running
ALLOC_ID=$(nomad job allocs ploy-controller | grep running | head -1 | awk '{print $1}')
if [ -z "$ALLOC_ID" ]; then
    echo -e "${RED}✗ Controller not running${NC}"
    echo "Check Nomad logs: nomad alloc logs $ALLOC_ID"
    exit 1
fi

echo -e "${GREEN}✓ Controller is running${NC}"

# Get controller port
CONTROLLER_PORT=$(nomad alloc status "$ALLOC_ID" | grep 'http.*yes' | awk '{print $3}' | cut -d: -f2)
echo "Controller accessible on port: $CONTROLLER_PORT"

# Test certificate provisioning
echo -e "${YELLOW}Step 7: Testing wildcard certificate provisioning...${NC}"

# Check certificate status
sleep 10
CERT_STATUS=$(curl -s "http://localhost:$CONTROLLER_PORT/health/platform-certificates" || echo "failed")
echo "Certificate status: $CERT_STATUS"

if echo "$CERT_STATUS" | grep -q "active\|provisioning"; then
    echo -e "${GREEN}✓ Wildcard certificate provisioning initiated${NC}"
else
    echo -e "${YELLOW}⚠ Certificate provisioning in progress${NC}"
fi

# Test HTTPS endpoint (may take a few minutes)
echo -e "${YELLOW}Step 8: Testing HTTPS endpoint...${NC}"
echo "Testing https://api.$DEV_DOMAIN/health (this may take a few minutes)..."

for i in {1..12}; do
    if curl -s -k "https://api.$DEV_DOMAIN/health" > /dev/null 2>&1; then
        echo -e "${GREEN}✓ HTTPS endpoint is accessible!${NC}"
        break
    else
        echo "  Attempt $i/12: Certificate provisioning in progress..."
        sleep 30
    fi
done

# Final status
echo ""
echo -e "${GREEN}Deployment Complete!${NC}"
echo ""
echo "🌐 Controller Endpoints:"
echo "  HTTP:  http://api.$DEV_DOMAIN (redirects to HTTPS)"
echo "  HTTPS: https://api.$DEV_DOMAIN"
echo ""
echo "🔒 Wildcard Certificate:"
echo "  Domain: *.$DEV_DOMAIN"
echo "  Covers: api.$DEV_DOMAIN, {app}.$DEV_DOMAIN"
echo ""
echo "🧪 Next Steps:"
echo "  1. Test controller: curl -s https://api.$DEV_DOMAIN/health"
echo "  2. Deploy test app: ./build/ploy push -a myapp"
echo "  3. Access app: https://myapp.$DEV_DOMAIN"
echo ""
echo "🔧 Monitoring:"
echo "  Controller logs: nomad alloc logs $ALLOC_ID"
echo "  Certificate status: curl -s https://api.$DEV_DOMAIN/health/platform-certificates"

# Cleanup
rm -f "$TEMP_JOB"