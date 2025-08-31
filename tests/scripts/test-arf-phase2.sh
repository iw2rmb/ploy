#!/bin/bash

# Test script for ARF (Automated Remediation Framework) Phase 2
# Tests all ARF Phase 2 components and endpoints

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
CONTROLLER_URL="https://api.dev.ployman.app"
if [ ! -z "$1" ]; then
    CONTROLLER_URL="$1"
fi

echo -e "${BLUE}ARF Phase 2 Testing Suite${NC}"
echo "==============================="
echo -e "${YELLOW}Controller URL: $CONTROLLER_URL${NC}"
echo

# Function to test an endpoint
test_endpoint() {
    local method=$1
    local endpoint=$2
    local description=$3
    local data=$4
    local expected_status=${5:-200}
    
    echo -n "Testing $description... "
    
    if [ "$method" = "GET" ]; then
        response=$(curl -s -o /tmp/arf_response.json -w "%{http_code}" "$CONTROLLER_URL$endpoint")
    elif [ "$method" = "POST" ]; then
        if [ -z "$data" ]; then
            response=$(curl -s -X POST -o /tmp/arf_response.json -w "%{http_code}" "$CONTROLLER_URL$endpoint")
        else
            response=$(curl -s -X POST -H "Content-Type: application/json" -d "$data" -o /tmp/arf_response.json -w "%{http_code}" "$CONTROLLER_URL$endpoint")
        fi
    elif [ "$method" = "PUT" ]; then
        response=$(curl -s -X PUT -H "Content-Type: application/json" -d "$data" -o /tmp/arf_response.json -w "%{http_code}" "$CONTROLLER_URL$endpoint")
    elif [ "$method" = "DELETE" ]; then
        response=$(curl -s -X DELETE -o /tmp/arf_response.json -w "%{http_code}" "$CONTROLLER_URL$endpoint")
    fi
    
    if [ "$response" -eq "$expected_status" ]; then
        echo -e "${GREEN}PASS${NC}"
        # Show response preview for successful tests
        if command -v jq >/dev/null 2>&1; then
            echo -e "${BLUE}  Response:${NC} $(jq -c '.' < /tmp/arf_response.json 2>/dev/null | head -c 100)..."
        else
            echo -e "${BLUE}  Response:${NC} $(cat /tmp/arf_response.json | head -c 100)..."
        fi
    else
        echo -e "${RED}FAIL${NC} (Expected: $expected_status, Got: $response)"
        echo -e "${RED}  Response:${NC} $(cat /tmp/arf_response.json)"
        return 1
    fi
}

# Function to run test section
run_test_section() {
    local section_name=$1
    echo -e "\n${YELLOW}=== $section_name ===${NC}"
}

# Test counter
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0

run_test() {
    TESTS_RUN=$((TESTS_RUN + 1))
    if "$@"; then
        TESTS_PASSED=$((TESTS_PASSED + 1))
    else
        TESTS_FAILED=$((TESTS_FAILED + 1))
    fi
}

# Circuit Breaker System Testing
run_test_section "Circuit Breaker System"
run_test test_endpoint "GET" "/v1/arf/circuit-breaker/stats" "Circuit breaker statistics"
run_test test_endpoint "GET" "/v1/arf/circuit-breaker/state?id=default" "Circuit breaker state"
run_test test_endpoint "POST" "/v1/arf/circuit-breaker/reset?id=test-breaker" "Circuit breaker reset"

# Error-Driven Recipe Evolution Testing
run_test_section "Recipe Management"
run_test test_endpoint "GET" "/v1/arf/recipes" "List recipes"
# Use timestamp-based unique recipe ID to avoid conflicts from previous runs
RECIPE_ID="test-recipe-$(date +%s)"
run_test test_endpoint "GET" "/v1/arf/recipes/$RECIPE_ID" "Get specific recipe (before creation)" "" 404
run_test test_endpoint "POST" "/v1/arf/recipes" "Create recipe" "{\"id\":\"$RECIPE_ID\",\"name\":\"Test Recipe\",\"description\":\"A test recipe for validation\",\"language\":\"java\",\"source\":\"org.openrewrite.java.cleanup.UnnecessaryParentheses\",\"category\":\"cleanup\",\"confidence\":0.9}" 201
run_test test_endpoint "GET" "/v1/arf/recipes/$RECIPE_ID" "Get specific recipe (after creation)"

# Parallel Error Resolution Testing
run_test_section "Parallel Error Resolution"
run_test test_endpoint "GET" "/v1/arf/parallel-resolver/stats" "Parallel resolver statistics"
run_test test_endpoint "POST" "/v1/arf/parallel-resolver/config" "Update parallel resolver config" '{"max_workers":4}'
run_test test_endpoint "POST" "/v1/arf/parallel-resolver/config" "Invalid config (too many workers)" '{"max_workers":50}' 400

# Multi-Repository Orchestration Testing
run_test_section "Multi-Repository Orchestration"
run_test test_endpoint "GET" "/v1/arf/multi-repo/stats" "Multi-repo orchestration statistics"
run_test test_endpoint "POST" "/v1/arf/multi-repo/orchestrate" "Start batch orchestration" '{"repositories":["repo1","repo2"],"recipe_ids":["recipe1"]}'
run_test test_endpoint "GET" "/v1/arf/multi-repo/orchestrations/test-orch-123" "Get orchestration status"
run_test test_endpoint "POST" "/v1/arf/multi-repo/orchestrate" "Invalid orchestration (no repos)" '{"repositories":[],"recipe_ids":["recipe1"]}' 400

# High Availability Integration Testing
run_test_section "High Availability Integration"
run_test test_endpoint "GET" "/v1/arf/ha/stats" "HA cluster statistics"
run_test test_endpoint "GET" "/v1/arf/ha/nodes" "HA node status"

# Monitoring Infrastructure Testing
run_test_section "Monitoring Infrastructure"
run_test test_endpoint "GET" "/v1/arf/monitoring/metrics" "System monitoring metrics"
run_test test_endpoint "GET" "/v1/arf/monitoring/alerts" "Active alerts"

# Pattern Learning Database Testing
run_test_section "Pattern Learning Database"
run_test test_endpoint "GET" "/v1/arf/patterns/stats" "Pattern learning statistics"
run_test test_endpoint "GET" "/v1/arf/patterns/recommendations?error_type=import&language=java" "Pattern recommendations"

# Transformation Execution Testing
run_test_section "Transformation Execution"
run_test test_endpoint "POST" "/v1/arf/transform" "Execute transformation" '{"recipe_id":"org.openrewrite.java.cleanup.UnusedImports","recipe_type":"openrewrite","codebase":{"language":"java","build_tool":"maven"}}'
run_test test_endpoint "GET" "/v1/arf/transforms/test-transform-123" "Get transformation result" "" 404

# Sandbox Management Testing
run_test_section "Sandbox Management"
run_test test_endpoint "GET" "/v1/arf/sandboxes" "List sandboxes"
run_test test_endpoint "POST" "/v1/arf/sandboxes" "Create sandbox" '{"repository":"https://github.com/example/test.git","language":"java","build_tool":"maven"}' 201
run_test test_endpoint "DELETE" "/v1/arf/sandboxes/test-sandbox-123" "Destroy sandbox" "" 500

# System Integration Testing
run_test_section "System Integration"
run_test test_endpoint "GET" "/v1/arf/health" "ARF health check"
run_test test_endpoint "GET" "/v1/arf/cache/stats" "Cache statistics"
run_test test_endpoint "POST" "/v1/arf/cache/clear" "Clear cache"

# Cleanup
rm -f /tmp/arf_response.json

echo -e "\n${BLUE}Test Summary${NC}"
echo "=============="
echo -e "Total Tests: $TESTS_RUN"
echo -e "${GREEN}Passed: $TESTS_PASSED${NC}"
echo -e "${RED}Failed: $TESTS_FAILED${NC}"

if [ $TESTS_FAILED -eq 0 ]; then
    echo -e "\n${GREEN}🎉 All ARF Phase 2 tests passed!${NC}"
    exit 0
else
    echo -e "\n${RED}❌ Some ARF Phase 2 tests failed${NC}"
    exit 1
fi