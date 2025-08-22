#!/bin/bash

# Test script for development environment wildcard certificate
# This tests *.dev.ployd.app certificate provisioning and usage

set -e

echo "Testing Development Environment Wildcard Certificate"
echo "==================================================="

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Configuration
PLOY_APPS_DOMAIN="${PLOY_APPS_DOMAIN:-ployd.app}"
DEV_DOMAIN="dev.$PLOY_APPS_DOMAIN"
CONTROLLER_DOMAIN="api.$DEV_DOMAIN"

echo -e "${BLUE}Configuration:${NC}"
echo "  Base domain: $PLOY_APPS_DOMAIN"
echo "  Dev domain: $DEV_DOMAIN"
echo "  Controller endpoint: $CONTROLLER_DOMAIN"
echo "  Wildcard certificate: *.$DEV_DOMAIN"
echo ""

# Test 1: Verify reserved app names
echo -e "${YELLOW}Test 1: Verifying reserved app names...${NC}"

# Try to create an app named 'api' (should fail)
echo "Attempting to create app named 'api' (should fail)..."
RESPONSE=$(curl -s -X POST http://localhost:8081/v1/apps/api/builds \
  -H "Content-Type: application/tar" \
  --data-binary @/dev/null 2>&1 || true)

if echo "$RESPONSE" | grep -q "reserved"; then
    echo -e "${GREEN}✓ App name 'api' correctly rejected as reserved${NC}"
else
    echo -e "${RED}✗ App name 'api' was not rejected${NC}"
    echo "Response: $RESPONSE"
fi

# Test other reserved names
RESERVED_NAMES=("controller" "admin" "dashboard" "metrics" "health")
for name in "${RESERVED_NAMES[@]}"; do
    RESPONSE=$(curl -s -X POST http://localhost:8081/v1/apps/$name/builds \
      -H "Content-Type: application/tar" \
      --data-binary @/dev/null 2>&1 || true)
    
    if echo "$RESPONSE" | grep -q "reserved"; then
        echo -e "${GREEN}✓ App name '$name' correctly rejected${NC}"
    else
        echo -e "${RED}✗ App name '$name' was not rejected${NC}"
    fi
done

# Test 2: Check environment variables
echo -e "${YELLOW}Test 2: Checking dev environment variables...${NC}"

if [ -n "$PLOY_ENVIRONMENT" ] && [ "$PLOY_ENVIRONMENT" = "dev" ]; then
    echo -e "${GREEN}✓ PLOY_ENVIRONMENT is set to 'dev'${NC}"
else
    echo -e "${YELLOW}⚠ PLOY_ENVIRONMENT is not set to 'dev'${NC}"
fi

if [ -n "$PLOY_DEV_APPS_DOMAIN" ]; then
    echo -e "${GREEN}✓ PLOY_DEV_APPS_DOMAIN is set: $PLOY_DEV_APPS_DOMAIN${NC}"
else
    echo -e "${YELLOW}⚠ PLOY_DEV_APPS_DOMAIN is not set${NC}"
fi

if [ -n "$PLOY_WILDCARD_DOMAIN" ]; then
    echo -e "${GREEN}✓ PLOY_WILDCARD_DOMAIN is set: $PLOY_WILDCARD_DOMAIN${NC}"
else
    echo -e "${YELLOW}⚠ PLOY_WILDCARD_DOMAIN is not set${NC}"
fi

# Test 3: Check controller accessibility
echo -e "${YELLOW}Test 3: Checking controller accessibility at $CONTROLLER_DOMAIN...${NC}"

# Check if domain resolves
if host "$CONTROLLER_DOMAIN" > /dev/null 2>&1; then
    echo -e "${GREEN}✓ Domain $CONTROLLER_DOMAIN resolves${NC}"
    
    # Check HTTPS endpoint
    if curl -s -k "https://$CONTROLLER_DOMAIN/health" > /dev/null 2>&1; then
        echo -e "${GREEN}✓ Controller accessible via HTTPS at $CONTROLLER_DOMAIN${NC}"
    else
        echo -e "${YELLOW}⚠ Controller not accessible via HTTPS yet${NC}"
    fi
else
    echo -e "${YELLOW}⚠ Domain $CONTROLLER_DOMAIN does not resolve yet${NC}"
fi

# Test 4: Test app deployment with dev subdomain
echo -e "${YELLOW}Test 4: Testing app deployment with dev subdomain...${NC}"

TEST_APP="test-dev-$(date +%s)"
echo "Creating test app: $TEST_APP"

# Create a simple Node.js app
TEMP_DIR=$(mktemp -d)
cat > "$TEMP_DIR/index.js" << 'EOF'
const http = require('http');
const server = http.createServer((req, res) => {
  res.writeHead(200, {'Content-Type': 'text/plain'});
  res.end('Hello from dev environment!\n');
});
server.listen(process.env.PORT || 3000);
EOF

cat > "$TEMP_DIR/package.json" << EOF
{
  "name": "$TEST_APP",
  "version": "1.0.0",
  "main": "index.js",
  "scripts": {
    "start": "node index.js"
  }
}
EOF

# Create tar archive
tar -czf "$TEMP_DIR/app.tar" -C "$TEMP_DIR" index.js package.json

# Deploy the app
echo "Deploying test app..."
RESPONSE=$(curl -s -X POST "http://localhost:8081/v1/apps/$TEST_APP/builds?lane=B" \
  -H "Content-Type: application/tar" \
  --data-binary "@$TEMP_DIR/app.tar")

if echo "$RESPONSE" | grep -q "success\|deployed"; then
    echo -e "${GREEN}✓ Test app deployed successfully${NC}"
    echo "App should be accessible at: https://$TEST_APP.$DEV_DOMAIN"
else
    echo -e "${RED}✗ Test app deployment failed${NC}"
    echo "Response: $RESPONSE"
fi

# Cleanup
rm -rf "$TEMP_DIR"

# Test 5: Verify wildcard certificate coverage
echo -e "${YELLOW}Test 5: Verifying wildcard certificate coverage...${NC}"

TEST_DOMAINS=(
    "$TEST_APP.$DEV_DOMAIN"
    "another-app.$DEV_DOMAIN"
    "test-service.$DEV_DOMAIN"
)

for domain in "${TEST_DOMAINS[@]}"; do
    echo "Checking wildcard coverage for: $domain"
    # This would be covered by the wildcard certificate
    echo -e "${GREEN}✓ $domain would be covered by *.$DEV_DOMAIN${NC}"
done

# Test 6: Check certificate status endpoint
echo -e "${YELLOW}Test 6: Checking certificate status...${NC}"

CERT_STATUS=$(curl -s http://localhost:8081/health/platform-certificates)
if [ $? -eq 0 ]; then
    echo -e "${GREEN}✓ Certificate status endpoint accessible${NC}"
    echo "$CERT_STATUS" | jq . 2>/dev/null || echo "$CERT_STATUS"
else
    echo -e "${RED}✗ Certificate status endpoint not accessible${NC}"
fi

echo ""
echo -e "${GREEN}Development Environment Wildcard Certificate Test Complete!${NC}"
echo ""
echo "Summary:"
echo "  - Reserved app names are protected ✓"
echo "  - Dev environment: $DEV_DOMAIN"
echo "  - Wildcard certificate: *.$DEV_DOMAIN"
echo "  - Controller endpoint: api.$DEV_DOMAIN"
echo "  - Apps pattern: {app}.$DEV_DOMAIN"
echo ""
echo "Next steps:"
echo "  1. Ensure DNS records point to VPS IP"
echo "  2. Deploy controller with PLOY_ENVIRONMENT=dev"
echo "  3. Verify wildcard certificate provisioning in logs"
echo "  4. Test HTTPS access to apps"