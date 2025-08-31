#!/bin/bash

# SSL/DNS Diagnostic Script for Ploy Infrastructure
# Comprehensive diagnosis of domain access and certificate issues

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
BASE_DOMAIN="${PLOY_APPS_DOMAIN:-ployd.app}"
DEV_SUBDOMAIN="${PLOY_DEV_SUBDOMAIN:-dev}"
DEV_DOMAIN="$DEV_SUBDOMAIN.$BASE_DOMAIN"
API_DOMAIN="api.$DEV_DOMAIN"
# TARGET_HOST should already be set globally
TIMEOUT=10

echo -e "${BLUE}Ploy SSL/DNS Diagnostic Tool${NC}"
echo "============================"
echo ""
echo -e "${BLUE}Configuration:${NC}"
echo "  Base domain: $BASE_DOMAIN"
echo "  Dev domain: $DEV_DOMAIN"
echo "  API domain: $API_DOMAIN"
echo "  Target IP: $TARGET_HOST"
echo ""

# Function to check command availability
check_command() {
    local cmd="$1"
    if ! command -v "$cmd" &> /dev/null; then
        echo -e "${RED}✗ Required command not found: $cmd${NC}"
        return 1
    fi
    return 0
}

# Function to test DNS resolution
test_dns() {
    local domain="$1"
    echo -e "${YELLOW}Testing DNS resolution for $domain...${NC}"
    
    # Test with different resolvers
    local resolvers=("8.8.8.8" "1.1.1.1" "208.67.222.222")
    local resolved_ips=()
    
    for resolver in "${resolvers[@]}"; do
        echo -n "  $resolver: "
        if resolved_ip=$(dig @"$resolver" +short "$domain" A | head -1); then
            if [[ -n "$resolved_ip" && "$resolved_ip" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
                resolved_ips+=("$resolved_ip")
                if [[ "$resolved_ip" == "$TARGET_HOST" ]]; then
                    echo -e "${GREEN}$resolved_ip ✓${NC}"
                else
                    echo -e "${YELLOW}$resolved_ip (expected $TARGET_HOST)${NC}"
                fi
            else
                echo -e "${RED}Failed to resolve${NC}"
            fi
        else
            echo -e "${RED}Query failed${NC}"
        fi
    done
    
    # Check consistency
    if [[ ${#resolved_ips[@]} -gt 0 ]]; then
        local first_ip="${resolved_ips[0]}"
        local consistent=true
        for ip in "${resolved_ips[@]}"; do
            if [[ "$ip" != "$first_ip" ]]; then
                consistent=false
                break
            fi
        done
        
        if [[ "$consistent" == "true" ]]; then
            echo -e "${GREEN}  ✓ DNS resolution consistent across resolvers${NC}"
        else
            echo -e "${YELLOW}  ⚠ DNS resolution inconsistent across resolvers${NC}"
        fi
        
        if [[ "$first_ip" == "$TARGET_HOST" ]]; then
            echo -e "${GREEN}  ✓ DNS points to correct IP${NC}"
            return 0
        else
            echo -e "${RED}  ✗ DNS points to wrong IP (got $first_ip, expected $TARGET_HOST)${NC}"
            return 1
        fi
    else
        echo -e "${RED}  ✗ No DNS resolution successful${NC}"
        return 1
    fi
}

# Function to test port connectivity
test_port() {
    local host="$1"
    local port="$2"
    local protocol="$3"
    
    echo -e "${YELLOW}Testing $protocol connectivity to $host:$port...${NC}"
    
    if timeout $TIMEOUT bash -c "cat < /dev/null > /dev/tcp/$host/$port" 2>/dev/null; then
        echo -e "${GREEN}  ✓ Port $port ($protocol) is open and accessible${NC}"
        return 0
    else
        echo -e "${RED}  ✗ Port $port ($protocol) is not accessible${NC}"
        return 1
    fi
}

# Function to test HTTP/HTTPS endpoints
test_http() {
    local url="$1"
    local protocol="${url%%://*}"
    
    echo -e "${YELLOW}Testing HTTP(S) connectivity to $url...${NC}"
    
    # Test basic connectivity
    if response=$(curl -s -I --max-time $TIMEOUT --connect-timeout 5 "$url" 2>/dev/null); then
        local status_code=$(echo "$response" | head -1 | cut -d' ' -f2)
        echo -e "${GREEN}  ✓ HTTP response received (status: $status_code)${NC}"
        
        # Check for specific headers
        if echo "$response" | grep -i "server:" > /dev/null; then
            local server=$(echo "$response" | grep -i "server:" | cut -d' ' -f2- | tr -d '\r')
            echo -e "${BLUE}    Server: $server${NC}"
        fi
        
        # Check for Traefik headers
        if echo "$response" | grep -i "x-forwarded" > /dev/null; then
            echo -e "${BLUE}    ✓ Traefik headers detected${NC}"
        fi
        
        return 0
    else
        echo -e "${RED}  ✗ No HTTP response received${NC}"
        
        # Test with verbose curl to get more info
        echo -e "${YELLOW}  Attempting verbose connection test...${NC}"
        if curl -v --max-time $TIMEOUT --connect-timeout 5 "$url" &>/tmp/curl_debug.log; then
            echo -e "${YELLOW}  Curl debug output saved to /tmp/curl_debug.log${NC}"
        fi
        
        return 1
    fi
}

# Function to test SSL certificate
test_ssl_cert() {
    local domain="$1"
    echo -e "${YELLOW}Testing SSL certificate for $domain...${NC}"
    
    # Test SSL connection
    if ssl_info=$(echo | timeout $TIMEOUT openssl s_client -servername "$domain" -connect "$domain:443" 2>/dev/null); then
        echo -e "${GREEN}  ✓ SSL connection established${NC}"
        
        # Extract certificate info
        if cert_text=$(echo "$ssl_info" | openssl x509 -noout -text 2>/dev/null); then
            # Check subject
            local subject=$(echo "$cert_text" | grep "Subject:" | head -1 | sed 's/.*Subject: //')
            echo -e "${BLUE}    Subject: $subject${NC}"
            
            # Check SAN
            local san=$(echo "$cert_text" | grep -A 1 "Subject Alternative Name:" | tail -1 | sed 's/.*DNS://' | sed 's/, DNS:/, /g')
            if [[ -n "$san" ]]; then
                echo -e "${BLUE}    SAN: $san${NC}"
                
                # Check if our domain is covered
                if echo "$san" | grep -E "(^|\s|,)\*\.$DEV_DOMAIN(\s|,|$)" > /dev/null || echo "$san" | grep -E "(^|\s|,)$domain(\s|,|$)" > /dev/null; then
                    echo -e "${GREEN}    ✓ Domain covered by certificate${NC}"
                else
                    echo -e "${RED}    ✗ Domain not covered by certificate${NC}"
                fi
            fi
            
            # Check expiry
            local not_after=$(echo "$cert_text" | grep "Not After" | sed 's/.*Not After : //')
            if [[ -n "$not_after" ]]; then
                echo -e "${BLUE}    Expires: $not_after${NC}"
                
                # Check if expiring soon (within 30 days)
                local expiry_epoch=$(date -d "$not_after" +%s 2>/dev/null || date -j -f "%b %d %T %Y %Z" "$not_after" +%s 2>/dev/null)
                local current_epoch=$(date +%s)
                local days_until_expiry=$(( (expiry_epoch - current_epoch) / 86400 ))
                
                if [[ $days_until_expiry -gt 30 ]]; then
                    echo -e "${GREEN}    ✓ Certificate valid for $days_until_expiry days${NC}"
                elif [[ $days_until_expiry -gt 0 ]]; then
                    echo -e "${YELLOW}    ⚠ Certificate expires in $days_until_expiry days${NC}"
                else
                    echo -e "${RED}    ✗ Certificate expired ${days_until_expiry#-} days ago${NC}"
                fi
            fi
            
            # Check issuer
            local issuer=$(echo "$cert_text" | grep "Issuer:" | head -1 | sed 's/.*Issuer: //')
            echo -e "${BLUE}    Issuer: $issuer${NC}"
            
            if echo "$issuer" | grep -i "let's encrypt" > /dev/null; then
                echo -e "${GREEN}    ✓ Let's Encrypt certificate${NC}"
            fi
        fi
        
        return 0
    else
        echo -e "${RED}  ✗ SSL connection failed${NC}"
        return 1
    fi
}

# Function to check Traefik service
check_traefik() {
    echo -e "${YELLOW}Checking Traefik configuration...${NC}"
    
    # Try to access Traefik API if available
    local traefik_endpoints=("http://$TARGET_HOST:8080/api/rawdata" "http://$TARGET_HOST:9090/api/rawdata")
    
    for endpoint in "${traefik_endpoints[@]}"; do
        echo -e "${YELLOW}  Checking Traefik at $endpoint...${NC}"
        if traefik_config=$(curl -s --max-time 5 "$endpoint" 2>/dev/null); then
            echo -e "${GREEN}  ✓ Traefik API accessible${NC}"
            
            # Check if our domain is configured
            if echo "$traefik_config" | grep -q "$API_DOMAIN"; then
                echo -e "${GREEN}    ✓ $API_DOMAIN found in Traefik configuration${NC}"
            else
                echo -e "${RED}    ✗ $API_DOMAIN not found in Traefik configuration${NC}"
            fi
            
            # Check for ploy-api service
            if echo "$traefik_config" | grep -q "ploy-api"; then
                echo -e "${GREEN}    ✓ ploy-api service found${NC}"
            else
                echo -e "${RED}    ✗ ploy-api service not found${NC}"
            fi
            
            return 0
        fi
    done
    
    echo -e "${YELLOW}  Traefik API not accessible, checking via SSH...${NC}"
    return 1
}

# Function to check Nomad service
check_nomad_service() {
    echo -e "${YELLOW}Checking Nomad ploy-api service...${NC}"
    
    # Try to get Nomad job status
    local nomad_endpoints=("http://$TARGET_HOST:4646/v1/job/ploy-api")
    
    for endpoint in "${nomad_endpoints[@]}"; do
        echo -e "${YELLOW}  Checking Nomad at $endpoint...${NC}"
        if nomad_status=$(curl -s --max-time 5 "$endpoint" 2>/dev/null); then
            echo -e "${GREEN}  ✓ Nomad API accessible${NC}"
            
            # Check job status
            if echo "$nomad_status" | grep -q '"Status":"running"'; then
                echo -e "${GREEN}    ✓ ploy-api job is running${NC}"
            else
                echo -e "${RED}    ✗ ploy-api job not running${NC}"
            fi
            
            return 0
        fi
    done
    
    echo -e "${YELLOW}  Nomad API not accessible${NC}"
    return 1
}

# Function to generate recommendations
generate_recommendations() {
    echo ""
    echo -e "${BLUE}Diagnostic Summary and Recommendations${NC}"
    echo "======================================"
    
    local issues_found=false
    
    # DNS Issues
    if ! test_dns "$API_DOMAIN" &>/dev/null; then
        issues_found=true
        echo -e "${RED}DNS Issues Detected:${NC}"
        echo "  1. Check Namecheap DNS settings for $BASE_DOMAIN"
        echo "  2. Ensure A record exists: $DEV_SUBDOMAIN → $TARGET_HOST"
        echo "  3. Ensure A record exists: *.$DEV_SUBDOMAIN → $TARGET_HOST"
        echo "  4. Wait for DNS propagation (5-10 minutes)"
        echo "  5. Run: ./scripts/setup-dev-dns.sh"
        echo ""
    fi
    
    # Port connectivity issues
    if ! test_port "$TARGET_HOST" 443 "HTTPS" &>/dev/null; then
        issues_found=true
        echo -e "${RED}Port Connectivity Issues:${NC}"
        echo "  1. Check if Traefik is running and binding to port 443"
        echo "  2. Verify firewall settings allow HTTPS traffic"
        echo "  3. Check Nomad/Consul service status"
        echo ""
    fi
    
    # SSL Certificate issues
    if ! test_ssl_cert "$API_DOMAIN" &>/dev/null; then
        issues_found=true
        echo -e "${RED}SSL Certificate Issues:${NC}"
        echo "  1. Check if certificate is properly issued"
        echo "  2. Verify Let's Encrypt ACME challenge completion"
        echo "  3. Ensure wildcard certificate covers $API_DOMAIN"
        echo "  4. Check certificate renewal automation"
        echo ""
    fi
    
    # Service issues
    if ! check_traefik &>/dev/null || ! check_nomad_service &>/dev/null; then
        issues_found=true
        echo -e "${RED}Service Configuration Issues:${NC}"
        echo "  1. Verify Traefik is properly configured with SSL"
        echo "  2. Check ploy-api Nomad job status"
        echo "  3. Ensure Consul service registration is working"
        echo "  4. Verify Traefik routing rules match domain pattern"
        echo ""
    fi
    
    if [[ "$issues_found" == "false" ]]; then
        echo -e "${GREEN}✓ All diagnostics passed! SSL/DNS configuration appears healthy.${NC}"
        echo ""
        echo "If you're still experiencing issues, try:"
        echo "  1. Clear browser cache and try incognito mode"
        echo "  2. Test from a different network/device"
        echo "  3. Check application logs for specific errors"
    else
        echo -e "${YELLOW}Next Steps:${NC}"
        echo "  1. Fix the identified issues above"
        echo "  2. Run this diagnostic script again to verify fixes"
        echo "  3. Test manually: curl -v https://$API_DOMAIN/health"
        echo "  4. Check VPS logs: ssh root@$TARGET_HOST"
    fi
    echo ""
}

# Main diagnostic flow
echo -e "${BLUE}Starting comprehensive diagnostics...${NC}"
echo ""

# Check required tools
echo -e "${YELLOW}Checking diagnostic tools...${NC}"
required_commands=("dig" "curl" "openssl" "timeout")
for cmd in "${required_commands[@]}"; do
    if check_command "$cmd"; then
        echo -e "${GREEN}  ✓ $cmd available${NC}"
    else
        echo -e "${RED}  ✗ $cmd not available${NC}"
        echo "Please install missing tools and run again."
        exit 1
    fi
done
echo ""

# Run diagnostics
echo -e "${BLUE}Phase 1: DNS Resolution${NC}"
test_dns "$DEV_DOMAIN"
test_dns "$API_DOMAIN"
echo ""

echo -e "${BLUE}Phase 2: Port Connectivity${NC}"
test_port "$TARGET_HOST" 80 "HTTP"
test_port "$TARGET_HOST" 443 "HTTPS"
test_port "$TARGET_HOST" 8500 "Consul"
test_port "$TARGET_HOST" 4646 "Nomad"
echo ""

echo -e "${BLUE}Phase 3: HTTP/HTTPS Connectivity${NC}"
test_http "http://$API_DOMAIN/health"
test_http "https://$API_DOMAIN/health"
echo ""

echo -e "${BLUE}Phase 4: SSL Certificate Validation${NC}"
test_ssl_cert "$API_DOMAIN"
echo ""

echo -e "${BLUE}Phase 5: Service Health${NC}"
check_traefik
check_nomad_service
echo ""

# Generate recommendations
generate_recommendations

echo -e "${BLUE}Diagnostic complete!${NC}"
echo "Log files may contain additional details:"
echo "  - /tmp/curl_debug.log (if connectivity issues found)"