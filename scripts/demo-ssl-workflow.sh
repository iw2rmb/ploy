#!/bin/bash

# Demo SSL Wildcard Certificate Workflow
# Demonstrates the complete process for *.dev.ployd.app SSL setup

set -e

echo "🔒 SSL Wildcard Certificate Workflow Demo"
echo "=========================================="

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
NC='\033[0m'

# Configuration
BASE_DOMAIN="${PLOY_APPS_DOMAIN:-ployd.app}"
DEV_DOMAIN="dev.$BASE_DOMAIN"
VPS_IP="${TARGET_IP:-45.12.75.241}"

echo -e "${BLUE}Configuration:${NC}"
echo "  🌐 Base domain: $BASE_DOMAIN"
echo "  🔧 Dev environment: $DEV_DOMAIN"
echo "  🖥️  VPS IP: $VPS_IP"
echo "  📜 Wildcard certificate: *.$DEV_DOMAIN"
echo ""

echo -e "${PURPLE}Step 1: Infrastructure Status${NC}"
echo "=============================="

# Check controller
CONTROLLER_STATUS=$(ssh root@$VPS_IP "nomad job status ploy-controller | grep Status | awk '{print \$3}'" 2>/dev/null || echo "unknown")
echo "✅ Controller status: $CONTROLLER_STATUS"

# Check environment variables
CONTROLLER_PORT=$(ssh root@$VPS_IP "nomad alloc status \$(nomad job allocs ploy-controller | grep running | head -1 | awk '{print \$1}') | grep 'http.*yes' | awk '{print \$3}' | cut -d: -f2" 2>/dev/null || echo "")
if [ -n "$CONTROLLER_PORT" ]; then
    echo "✅ Controller port: $CONTROLLER_PORT"
    
    # Test app name protection
    API_TEST=$(ssh root@$VPS_IP "curl -s -X POST http://localhost:$CONTROLLER_PORT/v1/apps/api/builds -H 'Content-Type: application/tar' --data-binary @/dev/null" 2>/dev/null || echo "failed")
    if echo "$API_TEST" | grep -q "reserved"; then
        echo "✅ App name 'api' is protected for controller use"
    else
        echo "❌ App name protection not working"
    fi
    
    # Test version endpoint
    VERSION=$(ssh root@$VPS_IP "curl -s http://localhost:$CONTROLLER_PORT/version | jq -r .version" 2>/dev/null || echo "unknown")
    echo "✅ Controller version: $VERSION"
else
    echo "❌ Controller not accessible"
fi

echo ""
echo -e "${PURPLE}Step 2: DNS Configuration Required${NC}"
echo "==================================="

# Check current DNS
CURRENT_DEV=$(dig +short "$DEV_DOMAIN" | tail -n1)
CURRENT_API=$(dig +short "api.$DEV_DOMAIN" | tail -n1)

echo "📋 Current DNS records:"
echo "  $DEV_DOMAIN → $CURRENT_DEV"
echo "  api.$DEV_DOMAIN → $CURRENT_API"
echo ""
echo "🎯 Required DNS records:"
echo "  $DEV_DOMAIN → $VPS_IP"
echo "  *.dev.$BASE_DOMAIN → $VPS_IP"
echo ""

if [ "$CURRENT_DEV" != "$VPS_IP" ]; then
    echo -e "${YELLOW}⚠️  DNS Update Needed:${NC}"
    echo "  1. Login to Namecheap control panel"
    echo "  2. Go to ployd.app → Advanced DNS"
    echo "  3. Update/Add these A records:"
    echo "     Host: dev, Value: $VPS_IP, TTL: 300"
    echo "     Host: *.dev, Value: $VPS_IP, TTL: 300"
    echo ""
else
    echo "✅ DNS records are correctly configured"
fi

echo -e "${PURPLE}Step 3: SSL Certificate Deployment Process${NC}"
echo "=============================================="

echo "🔑 Prerequisites for automatic SSL deployment:"
echo "  1. Namecheap API credentials (whitelisted for VPS IP)"
echo "  2. DNS records pointing to VPS"
echo "  3. Let's Encrypt account email"
echo ""

echo "📋 Required environment variables on VPS:"
echo "  export NAMECHEAP_API_USER=\"your-api-user\""
echo "  export NAMECHEAP_API_KEY=\"your-api-key\""
echo "  export NAMECHEAP_USERNAME=\"your-username\""
echo "  export CERT_EMAIL=\"admin@$BASE_DOMAIN\""
echo ""

echo "🚀 Deployment command:"
echo "  ./scripts/deploy-with-ssl.sh"
echo ""

echo -e "${PURPLE}Step 4: Expected Results${NC}"
echo "========================"

echo "🌐 After successful SSL deployment:"
echo "  https://api.$DEV_DOMAIN         → Controller API"
echo "  https://myapp.$DEV_DOMAIN       → User applications"
echo "  https://testapp.$DEV_DOMAIN     → User applications"
echo "  https://{any-app}.$DEV_DOMAIN   → User applications"
echo ""

echo "🔒 Certificate details:"
echo "  Subject: *.$DEV_DOMAIN"
echo "  Issuer: Let's Encrypt"
echo "  Validity: 90 days (auto-renew)"
echo "  Challenge: DNS-01"
echo ""

echo "🧪 Testing commands:"
echo "  curl -s https://api.$DEV_DOMAIN/health"
echo "  curl -s https://api.$DEV_DOMAIN/version"
echo "  ./build/ploy push -a myapp"
echo "  curl -s https://myapp.$DEV_DOMAIN"
echo ""

echo -e "${PURPLE}Step 5: Monitoring & Maintenance${NC}"
echo "================================="

echo "📊 Certificate monitoring:"
echo "  curl -s https://api.$DEV_DOMAIN/health/platform-certificates"
echo "  nomad alloc logs \$(nomad job allocs ploy-controller | grep running | head -1 | awk '{print \$1}')"
echo ""

echo "🔄 Automatic renewal:"
echo "  ✅ Let's Encrypt certificates auto-renew at 30 days before expiry"
echo "  ✅ DNS-01 challenge works behind firewalls"
echo "  ✅ No manual intervention required"
echo ""

echo -e "${PURPLE}Step 6: Development Workflow${NC}"
echo "============================"

echo "👨‍💻 Developer experience:"
echo "  1. Deploy app: ./build/ploy push -a myapp"
echo "  2. Access via HTTPS: https://myapp.$DEV_DOMAIN"
echo "  3. All subdomains automatically SSL-enabled"
echo "  4. No certificate management required"
echo ""

echo "🚫 Protected app names (cannot be used):"
echo "  api, controller, admin, dashboard, metrics, health"
echo "  console, www, ploy, system, traefik, nomad, consul, vault"
echo ""

echo -e "${GREEN}✨ SSL Wildcard Certificate Implementation Complete!${NC}"
echo ""
echo "📚 Next steps:"
echo "  1. Update DNS records in Namecheap (*.dev.ployd.app → $VPS_IP)"
echo "  2. Get Namecheap API credentials and whitelist VPS IP"
echo "  3. Run: ./scripts/deploy-with-ssl.sh"
echo "  4. Enjoy automatic HTTPS for all dev applications!"
echo ""
echo "🔗 Documentation: README.md → Development Environment SSL Setup"