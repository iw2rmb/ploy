#!/bin/bash

# DNS Records Validation Script
# Validates that DNS records are properly configured for SSL certificate provisioning

set -e

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Configuration
BASE_DOMAIN="dev.ployd.app"
CONTROLLER_DOMAIN="api.dev.ployman.app"
TARGET_IP="45.12.75.241"

echo -e "${BLUE}🌐 DNS Records Validation${NC}"
echo "=========================="
echo ""

echo -e "${BLUE}Configuration:${NC}"
echo "  🎯 Target IP: $TARGET_IP"
echo "  🌐 Base Domain: $BASE_DOMAIN"
echo "  🎯 Controller: $CONTROLLER_DOMAIN"
echo ""

# Function to resolve DNS record
resolve_dns() {
    local domain="$1"
    local ip=$(dig +short "$domain" | tail -n1)
    echo "$ip"
}

# Function to check DNS record
check_dns_record() {
    local domain="$1"
    local expected_ip="$2"
    local description="$3"
    
    echo -e "${YELLOW}Checking $description...${NC}"
    
    local current_ip=$(resolve_dns "$domain")
    
    if [ -z "$current_ip" ]; then
        echo -e "${RED}❌ No DNS record found for $domain${NC}"
        return 1
    elif [ "$current_ip" = "$expected_ip" ]; then
        echo -e "${GREEN}✅ $domain → $current_ip (correct)${NC}"
        return 0
    else
        echo -e "${RED}❌ $domain → $current_ip (expected: $expected_ip)${NC}"
        return 1
    fi
}

# Function to test wildcard resolution
test_wildcard() {
    local base_domain="$1"
    local expected_ip="$2"
    
    echo -e "${YELLOW}Testing wildcard resolution...${NC}"
    
    # Test a few sample subdomains
    local test_domains=("test.$base_domain" "app1.$base_domain" "myapp.$base_domain")
    local wildcard_working=true
    
    for test_domain in "${test_domains[@]}"; do
        local current_ip=$(resolve_dns "$test_domain")
        if [ "$current_ip" = "$expected_ip" ]; then
            echo -e "${GREEN}✅ $test_domain → $current_ip (wildcard working)${NC}"
        else
            echo -e "${RED}❌ $test_domain → $current_ip (expected: $expected_ip)${NC}"
            wildcard_working=false
        fi
    done
    
    if [ "$wildcard_working" = true ]; then
        return 0
    else
        return 1
    fi
}

# Function to check DNS propagation timing
check_propagation_timing() {
    local domain="$1"
    local expected_ip="$2"
    
    echo -e "${YELLOW}Testing DNS propagation from multiple nameservers...${NC}"
    
    # Test against different DNS servers
    local nameservers=("8.8.8.8" "1.1.1.1" "208.67.222.222")
    local propagated=true
    
    for ns in "${nameservers[@]}"; do
        local ip=$(dig "@$ns" +short "$domain" | tail -n1)
        if [ "$ip" = "$expected_ip" ]; then
            echo -e "${GREEN}✅ $ns: $domain → $ip${NC}"
        else
            echo -e "${RED}❌ $ns: $domain → $ip (expected: $expected_ip)${NC}"
            propagated=false
        fi
    done
    
    if [ "$propagated" = true ]; then
        return 0
    else
        echo -e "${YELLOW}⚠️  DNS propagation may still be in progress${NC}"
        return 1
    fi
}

# Main validation logic
echo -e "${BLUE}Step 1: Core DNS Records${NC}"
echo "------------------------"

dns_valid=true

# Check base domain
if ! check_dns_record "$BASE_DOMAIN" "$TARGET_IP" "base domain"; then
    dns_valid=false
fi

# Check controller domain
if ! check_dns_record "$CONTROLLER_DOMAIN" "$TARGET_IP" "controller domain"; then
    dns_valid=false
fi

echo ""

# Check wildcard functionality
echo -e "${BLUE}Step 2: Wildcard DNS Resolution${NC}"
echo "-------------------------------"

if ! test_wildcard "$BASE_DOMAIN" "$TARGET_IP"; then
    dns_valid=false
    echo -e "${YELLOW}⚠️  Wildcard DNS may not be properly configured${NC}"
fi

echo ""

# Check propagation
echo -e "${BLUE}Step 3: DNS Propagation Check${NC}"
echo "-----------------------------"

if ! check_propagation_timing "$BASE_DOMAIN" "$TARGET_IP"; then
    echo -e "${YELLOW}⚠️  DNS propagation incomplete - wait 5-10 minutes and retry${NC}"
fi

echo ""

# Final validation result
echo -e "${BLUE}Validation Summary${NC}"
echo "=================="

if [ "$dns_valid" = true ]; then
    echo -e "${GREEN}✅ DNS configuration is correct and ready for SSL certificate provisioning${NC}"
    echo ""
    echo -e "${GREEN}🚀 Next steps:${NC}"
    echo "  1. Ensure Namecheap API credentials are configured"
    echo "  2. Run: ./scripts/activate-ssl.sh"
    echo "  3. Test HTTPS access to your applications"
    echo ""
    exit 0
else
    echo -e "${RED}❌ DNS configuration issues detected${NC}"
    echo ""
    echo -e "${YELLOW}🔧 Required DNS records in Namecheap:${NC}"
    echo "  Type: A, Host: dev, Value: $TARGET_IP, TTL: 300"
    echo "  Type: A, Host: *.dev, Value: $TARGET_IP, TTL: 300"
    echo ""
    echo -e "${YELLOW}📋 Instructions:${NC}"
    echo "  1. Login to Namecheap.com"
    echo "  2. Go to Domain List → ployd.app → Manage → Advanced DNS"
    echo "  3. Add/Update the A records above"
    echo "  4. Wait 5-10 minutes for propagation"
    echo "  5. Re-run this script: ./scripts/ssl/validate-dns-records.sh"
    echo ""
    exit 1
fi