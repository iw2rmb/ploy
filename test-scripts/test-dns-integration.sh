#!/bin/bash

# Test script for DNS integration functionality
# Tests both Cloudflare and Namecheap providers

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Test configuration
CONTROLLER_URL="${PLOY_CONTROLLER:-https://api.dev.ployd.app}"
TEST_DOMAIN="${PLOY_TEST_DOMAIN:-ployd.app}"
TEST_IP="${PLOY_TEST_IP:-192.168.1.100}"

echo -e "${YELLOW}Starting DNS integration tests...${NC}"
echo "Controller URL: $CONTROLLER_URL"
echo "Test domain: $TEST_DOMAIN"
echo "Test IP: $TEST_IP"
echo

# Function to test API endpoint
test_endpoint() {
    local method="$1"
    local endpoint="$2"
    local data="$3"
    local expected_status="$4"
    local test_name="$5"
    
    echo -n "Testing $test_name... "
    
    if [ "$method" = "GET" ]; then
        response=$(curl -s -w "HTTPSTATUS:%{http_code}" "$CONTROLLER_URL$endpoint")
    else
        response=$(curl -s -w "HTTPSTATUS:%{http_code}" -X "$method" -H "Content-Type: application/json" -d "$data" "$CONTROLLER_URL$endpoint")
    fi
    
    status=$(echo "$response" | grep -o 'HTTPSTATUS:[0-9]*' | cut -d: -f2)
    body=$(echo "$response" | sed 's/HTTPSTATUS:[0-9]*$//')
    
    if [ "$status" = "$expected_status" ]; then
        echo -e "${GREEN}PASS${NC}"
        return 0
    else
        echo -e "${RED}FAIL${NC} (Status: $status, Expected: $expected_status)"
        echo "Response: $body"
        return 1
    fi
}

# Test 1: Get DNS configuration
echo -e "${YELLOW}Test 1: Get DNS configuration${NC}"
test_endpoint "GET" "/v1/dns/config" "" "200" "DNS configuration retrieval"

# Test 2: Validate DNS provider configuration
echo -e "${YELLOW}Test 2: Validate DNS provider configuration${NC}"
test_endpoint "POST" "/v1/dns/config/validate" "" "200" "DNS provider validation"

# Test 3: List DNS records
echo -e "${YELLOW}Test 3: List DNS records${NC}"
test_endpoint "GET" "/v1/dns/records?domain=$TEST_DOMAIN" "" "200" "DNS records listing"

# Test 4: Validate wildcard DNS configuration
echo -e "${YELLOW}Test 4: Validate wildcard DNS configuration${NC}"
test_endpoint "GET" "/v1/dns/wildcard/validate" "" "200" "Wildcard DNS validation"

# Test 5: Setup wildcard DNS (if credentials are available)
if [ -n "$CLOUDFLARE_API_TOKEN" ] || [ -n "$NAMECHEAP_API_KEY" ]; then
    echo -e "${YELLOW}Test 5: Setup wildcard DNS${NC}"
    wildcard_data="{\"target_ip\":\"$TEST_IP\",\"ttl\":300}"
    test_endpoint "POST" "/v1/dns/wildcard/setup" "$wildcard_data" "200" "Wildcard DNS setup"
else
    echo -e "${YELLOW}Test 5: Skipping wildcard DNS setup (no credentials)${NC}"
fi

# Test 6: Create individual DNS record
echo -e "${YELLOW}Test 6: Create individual DNS record${NC}"
record_data="{\"hostname\":\"test-$(date +%s).$TEST_DOMAIN\",\"type\":\"A\",\"value\":\"$TEST_IP\",\"ttl\":300}"
result=$(test_endpoint "POST" "/v1/dns/records" "$record_data" "200" "Individual DNS record creation")

# Test 7: Update DNS record
if [ $? -eq 0 ]; then
    echo -e "${YELLOW}Test 7: Update DNS record${NC}"
    update_data="{\"hostname\":\"test-$(date +%s).$TEST_DOMAIN\",\"type\":\"A\",\"value\":\"192.168.1.101\",\"ttl\":600}"
    test_endpoint "PUT" "/v1/dns/records" "$update_data" "200" "DNS record update"
fi

# Test 8: Delete DNS record
echo -e "${YELLOW}Test 8: Delete DNS record${NC}"
test_hostname="test-$(date +%s).$TEST_DOMAIN"
test_endpoint "DELETE" "/v1/dns/records/$test_hostname/A" "" "200" "DNS record deletion"

# Test 9: Remove wildcard DNS (cleanup)
if [ -n "$CLOUDFLARE_API_TOKEN" ] || [ -n "$NAMECHEAP_API_KEY" ]; then
    echo -e "${YELLOW}Test 9: Remove wildcard DNS (cleanup)${NC}"
    test_endpoint "DELETE" "/v1/dns/wildcard" "" "200" "Wildcard DNS removal"
else
    echo -e "${YELLOW}Test 9: Skipping wildcard DNS cleanup${NC}"
fi

echo
echo -e "${GREEN}DNS integration tests completed!${NC}"

# Additional validation: DNS resolution test
echo -e "${YELLOW}Additional validation: DNS resolution test${NC}"
if command -v dig >/dev/null 2>&1; then
    echo "Testing DNS resolution for wildcard domain..."
    test_subdomain="test-$(date +%s).${TEST_DOMAIN}"
    dig_result=$(dig +short "$test_subdomain" @8.8.8.8 2>/dev/null || echo "")
    
    if [ -n "$dig_result" ]; then
        echo -e "${GREEN}DNS resolution working: $test_subdomain -> $dig_result${NC}"
    else
        echo -e "${YELLOW}DNS resolution not yet propagated or not configured${NC}"
    fi
else
    echo -e "${YELLOW}dig command not available, skipping DNS resolution test${NC}"
fi

echo
echo -e "${GREEN}All DNS tests completed successfully!${NC}"