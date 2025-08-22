#!/bin/bash

# SSL Wildcard Certificate Activation Script
# Guides through the complete activation process

set -e

echo "🔒 SSL Wildcard Certificate Activation"
echo "======================================"

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
NC='\033[0m'

VPS_IP="${TARGET_IP:-45.12.75.241}"
DEV_DOMAIN="dev.ployd.app"

echo -e "${BLUE}Target Configuration:${NC}"
echo "  🌐 Wildcard: *.dev.ployd.app"
echo "  🖥️  VPS IP: $VPS_IP"
echo "  🎯 Controller: api.dev.ployd.app"
echo ""

echo -e "${PURPLE}Step 1: DNS Records Update${NC}"
echo "=========================="
echo ""
echo "📋 Please update DNS records in Namecheap:"
echo "  1. Login to Namecheap.com"
echo "  2. Go to Domain List → ployd.app → Manage → Advanced DNS"
echo "  3. Add/Update these A records:"
echo ""
echo "     Type: A, Host: dev, Value: $VPS_IP, TTL: 300"
echo "     Type: A, Host: *.dev, Value: $VPS_IP, TTL: 300"
echo ""
echo "📄 See DNS_UPDATE_GUIDE.md for detailed instructions"
echo ""

# Check if DNS is already updated
echo -e "${YELLOW}Checking current DNS status...${NC}"
CURRENT_DEV=$(dig +short "$DEV_DOMAIN" | tail -n1)
CURRENT_API=$(dig +short "api.$DEV_DOMAIN" | tail -n1)

echo "  Current: dev.ployd.app → $CURRENT_DEV"
echo "  Current: api.dev.ployd.app → $CURRENT_API"
echo "  Required: $VPS_IP"
echo ""

if [ "$CURRENT_DEV" = "$VPS_IP" ] && [ "$CURRENT_API" = "$VPS_IP" ]; then
    echo -e "${GREEN}✅ DNS records are already correct!${NC}"
    DNS_READY=true
else
    echo -e "${YELLOW}⏳ DNS records need updating or are still propagating${NC}"
    DNS_READY=false
fi

if [ "$DNS_READY" = false ]; then
    echo ""
    read -p "Have you updated the DNS records? (y/N): " dns_confirm
    if [[ ! $dns_confirm =~ ^[Yy]$ ]]; then
        echo ""
        echo "Please update DNS records first, then run this script again."
        echo "Command: ./scripts/activate-ssl.sh"
        exit 0
    fi
    
    echo ""
    echo -e "${YELLOW}Waiting for DNS propagation (up to 10 minutes)...${NC}"
    for i in {1..20}; do
        sleep 30
        CURRENT_DEV=$(dig +short "$DEV_DOMAIN" | tail -n1)
        CURRENT_API=$(dig +short "api.$DEV_DOMAIN" | tail -n1)
        
        if [ "$CURRENT_DEV" = "$VPS_IP" ] && [ "$CURRENT_API" = "$VPS_IP" ]; then
            echo -e "${GREEN}✅ DNS propagation complete! ($i/20)${NC}"
            DNS_READY=true
            break
        else
            echo "  Attempt $i/20: Still propagating... (dev: $CURRENT_DEV, api: $CURRENT_API)"
        fi
    done
    
    if [ "$DNS_READY" = false ]; then
        echo -e "${RED}❌ DNS propagation taking longer than expected${NC}"
        echo "Please wait longer and try again, or check DNS configuration."
        exit 1
    fi
fi

echo ""
echo -e "${PURPLE}Step 2: Namecheap API Credentials${NC}"
echo "================================="
echo ""
echo "🔑 For automatic SSL certificate provisioning, we need Namecheap API access."
echo "📄 See NAMECHEAP_API_SETUP.md for detailed instructions"
echo ""

# Check if credentials are already set
if [ -n "$NAMECHEAP_API_USER" ] && [ -n "$NAMECHEAP_API_KEY" ] && [ -n "$NAMECHEAP_USERNAME" ]; then
    echo -e "${GREEN}✅ Namecheap API credentials are already set${NC}"
    API_READY=true
else
    echo -e "${YELLOW}⚠️  Namecheap API credentials needed${NC}"
    echo ""
    echo "Required steps:"
    echo "  1. Login to Namecheap → Profile → Tools → API Access"
    echo "  2. Enable API Access"
    echo "  3. Whitelist VPS IP: $VPS_IP"
    echo "  4. Note down API key and username"
    echo ""
    
    read -p "Do you have Namecheap API credentials ready? (y/N): " api_confirm
    if [[ ! $api_confirm =~ ^[Yy]$ ]]; then
        echo ""
        echo "Please set up Namecheap API credentials first:"
        echo ""
        echo "  export NAMECHEAP_API_USER=\"your-username\""
        echo "  export NAMECHEAP_API_KEY=\"your-api-key\""
        echo "  export NAMECHEAP_USERNAME=\"your-username\""
        echo "  export CERT_EMAIL=\"admin@ployd.app\""
        echo ""
        echo "Then run: ./scripts/activate-ssl.sh"
        exit 0
    fi
    
    # Prompt for credentials
    echo ""
    read -p "Enter Namecheap API User: " api_user
    read -p "Enter Namecheap API Key: " api_key
    read -p "Enter Namecheap Username: " username
    
    export NAMECHEAP_API_USER="$api_user"
    export NAMECHEAP_API_KEY="$api_key"
    export NAMECHEAP_USERNAME="$username"
    export CERT_EMAIL="admin@ployd.app"
    
    echo -e "${GREEN}✅ Credentials set for this session${NC}"
    API_READY=true
fi

echo ""
echo -e "${PURPLE}Step 3: SSL Certificate Deployment${NC}"
echo "=================================="
echo ""

if [ "$DNS_READY" = true ] && [ "$API_READY" = true ]; then
    echo -e "${GREEN}🚀 All prerequisites met! Deploying SSL certificate...${NC}"
    echo ""
    
    # Run the SSL deployment
    if [ -f "./scripts/deploy-with-ssl.sh" ]; then
        echo "Executing: ./scripts/deploy-with-ssl.sh"
        echo ""
        ./scripts/deploy-with-ssl.sh
    else
        echo -e "${RED}❌ deploy-with-ssl.sh script not found${NC}"
        echo "Please run this from the ploy repository root directory."
        exit 1
    fi
else
    echo -e "${RED}❌ Prerequisites not met${NC}"
    echo "Please complete the previous steps first."
    exit 1
fi

echo ""
echo -e "${GREEN}🎉 SSL Wildcard Certificate Activation Complete!${NC}"
echo ""
echo "🌐 Your dev environment now has HTTPS:"
echo "  https://api.dev.ployd.app (Controller)"
echo "  https://{app}.dev.ployd.app (Applications)"
echo ""
echo "🧪 Test commands:"
echo "  curl -s https://api.dev.ployd.app/health"
echo "  curl -s https://api.dev.ployd.app/version"
echo ""
echo "🚀 Deploy an app:"
echo "  ./build/ploy push -a myapp"
echo "  curl -s https://myapp.dev.ployd.app"