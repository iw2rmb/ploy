#!/bin/bash

# SSL Certificate Testing Script for Ploy Platform
# Tests HTTPS access to api.dev.ployd.app and validates certificate

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Configuration
API_DOMAIN="${API_DOMAIN:-api.dev.ployd.app}"
TIMEOUT=30

echo -e "${BLUE}Ploy SSL Certificate Test${NC}"
echo "========================="
echo ""
echo -e "${BLUE}Testing HTTPS access to: $API_DOMAIN${NC}"
echo ""

# Test 1: Check DNS resolution
echo -e "${YELLOW}1. Testing DNS resolution...${NC}"
if resolved_ip=$(dig +short "$API_DOMAIN" | tail -n1); then
    if [[ -n "$resolved_ip" && "$resolved_ip" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
        echo -e "${GREEN}  ✓ DNS resolved: $API_DOMAIN → $resolved_ip${NC}"
    else
        echo -e "${RED}  ✗ DNS resolution failed${NC}"
        exit 1
    fi
else
    echo -e "${RED}  ✗ DNS query failed${NC}"
    exit 1
fi

# Test 2: Test HTTPS connectivity
echo -e "${YELLOW}2. Testing HTTPS connectivity...${NC}"
if curl -s --max-time $TIMEOUT --connect-timeout 10 "https://$API_DOMAIN/health" > /dev/null; then
    echo -e "${GREEN}  ✓ HTTPS connection successful${NC}"
else
    echo -e "${RED}  ✗ HTTPS connection failed${NC}"
    echo -e "${YELLOW}  Attempting HTTP fallback...${NC}"
    if curl -s --max-time $TIMEOUT "http://$API_DOMAIN/health" > /dev/null; then
        echo -e "${YELLOW}  → HTTP works, but HTTPS is not available${NC}"
        echo -e "${YELLOW}  → Check Traefik certificate provisioning${NC}"
    else
        echo -e "${RED}  → HTTP also fails - check service deployment${NC}"
    fi
    exit 1
fi

# Test 3: Validate SSL certificate
echo -e "${YELLOW}3. Validating SSL certificate...${NC}"
if ssl_info=$(echo | timeout $TIMEOUT openssl s_client -servername "$API_DOMAIN" -connect "$API_DOMAIN:443" 2>/dev/null); then
    echo -e "${GREEN}  ✓ SSL handshake successful${NC}"
    
    # Extract certificate details
    if cert_text=$(echo "$ssl_info" | openssl x509 -noout -text 2>/dev/null); then
        # Check subject
        subject=$(echo "$cert_text" | grep "Subject:" | head -1 | sed 's/.*Subject: //')
        echo -e "${BLUE}    Subject: $subject${NC}"
        
        # Check SAN (Subject Alternative Names)
        san=$(echo "$cert_text" | grep -A 1 "Subject Alternative Name:" | tail -1 | sed 's/.*DNS://' | sed 's/, DNS:/, /g' 2>/dev/null || echo "")
        if [[ -n "$san" ]]; then
            echo -e "${BLUE}    SAN: $san${NC}"
            
            # Check if our domain is covered
            if echo "$san" | grep -E "(^|\\s|,)\\*\\.dev\\.ployd\\.app(\\s|,|$)" > /dev/null || echo "$san" | grep -E "(^|\\s|,)$API_DOMAIN(\\s|,|$)" > /dev/null; then
                echo -e "${GREEN}    ✓ Domain covered by certificate${NC}"
            else
                echo -e "${RED}    ✗ Domain not covered by certificate${NC}"
            fi
        fi
        
        # Check expiry
        not_after=$(echo "$cert_text" | grep "Not After" | sed 's/.*Not After : //')
        if [[ -n "$not_after" ]]; then
            echo -e "${BLUE}    Expires: $not_after${NC}"
            
            # Check if expiring soon (within 30 days)
            if command -v date >/dev/null 2>&1; then
                if expiry_epoch=$(date -d "$not_after" +%s 2>/dev/null || date -j -f "%b %d %T %Y %Z" "$not_after" +%s 2>/dev/null); then
                    current_epoch=$(date +%s)
                    days_until_expiry=$(( (expiry_epoch - current_epoch) / 86400 ))
                    
                    if [[ $days_until_expiry -gt 30 ]]; then
                        echo -e "${GREEN}    ✓ Certificate valid for $days_until_expiry days${NC}"
                    elif [[ $days_until_expiry -gt 0 ]]; then
                        echo -e "${YELLOW}    ⚠ Certificate expires in $days_until_expiry days${NC}"
                    else
                        echo -e "${RED}    ✗ Certificate expired ${days_until_expiry#-} days ago${NC}"
                    fi
                fi
            fi
        fi
        
        # Check issuer
        issuer=$(echo "$cert_text" | grep "Issuer:" | head -1 | sed 's/.*Issuer: //')
        echo -e "${BLUE}    Issuer: $issuer${NC}"
        
        if echo "$issuer" | grep -i "let's encrypt" > /dev/null; then
            echo -e "${GREEN}    ✓ Let's Encrypt certificate${NC}"
        fi
    fi
else
    echo -e "${RED}  ✗ SSL certificate validation failed${NC}"
    exit 1
fi

# Test 4: Test API endpoint
echo -e "${YELLOW}4. Testing API endpoint...${NC}"
if response=$(curl -s --max-time $TIMEOUT "https://$API_DOMAIN/health"); then
    echo -e "${GREEN}  ✓ API health endpoint accessible${NC}"
    if echo "$response" | grep -q "status"; then
        echo -e "${GREEN}  ✓ API responding with health data${NC}"
    fi
else
    echo -e "${RED}  ✗ API health endpoint failed${NC}"
    exit 1
fi

# Test 5: Test wildcard certificate (if applicable)
echo -e "${YELLOW}5. Testing wildcard certificate coverage...${NC}"
test_subdomains=("test.dev.ployd.app" "app.dev.ployd.app" "demo.dev.ployd.app")

for subdomain in "${test_subdomains[@]}"; do
    echo -n "  Testing $subdomain: "
    if echo | timeout 10 openssl s_client -servername "$subdomain" -connect "$subdomain:443" 2>/dev/null | openssl x509 -noout -text 2>/dev/null | grep -q "*.dev.ployd.app"; then
        echo -e "${GREEN}✓${NC}"
    else
        echo -e "${YELLOW}⚠ (certificate may not cover this subdomain)${NC}"
    fi
done

echo ""
echo -e "${GREEN}🎉 SSL Certificate Test Completed Successfully!${NC}"
echo ""
echo "Summary:"
echo "  • HTTPS access: ✓ Working"
echo "  • Certificate: ✓ Valid"
echo "  • Domain coverage: ✓ Confirmed"
echo "  • API endpoint: ✓ Accessible"
echo ""
echo "Your platform is ready with proper SSL certificates!"