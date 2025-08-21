#!/bin/bash
# Test Controller Nomad Deployment
# Tests Phase no-SPOF-3 Step 2: Controller deployment via Nomad with binary distribution

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PLOY_DIR="$(dirname "$SCRIPT_DIR")"

# Color codes
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo "=== Testing Controller Nomad Deployment ==="

# Test function
test_command() {
    local test_name="$1"
    local command="$2"
    local expected_status="${3:-0}"
    
    echo -n "Testing $test_name... "
    
    if eval "$command" >/dev/null 2>&1; then
        if [ $? -eq $expected_status ]; then
            echo -e "${GREEN}PASS${NC}"
            return 0
        else
            echo -e "${RED}FAIL${NC} (unexpected exit code)"
            return 1
        fi
    else
        if [ $expected_status -ne 0 ]; then
            echo -e "${GREEN}PASS${NC} (expected failure)"
            return 0
        else
            echo -e "${RED}FAIL${NC}"
            return 1
        fi
    fi
}

# Test counters
TESTS_PASSED=0
TESTS_FAILED=0
TOTAL_TESTS=0

run_test() {
    local test_name="$1"
    local command="$2"
    local expected_status="${3:-0}"
    
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    
    if test_command "$test_name" "$command" "$expected_status"; then
        TESTS_PASSED=$((TESTS_PASSED + 1))
    else
        TESTS_FAILED=$((TESTS_FAILED + 1))
    fi
}

cd "$PLOY_DIR"

# Verify required services are running
echo "🔍 Verifying dependencies..."

run_test "SeaweedFS master" "curl -f -s http://localhost:9333/dir/status"
run_test "SeaweedFS filer" "curl -f -s http://localhost:8888/dir/status"
run_test "Consul" "curl -f -s http://localhost:8500/v1/status/leader"
run_test "Nomad" "curl -f -s http://localhost:4646/v1/status/leader"
run_test "Vault" "curl -f -s http://localhost:8200/v1/sys/health"

# Test 568: Ansible playbook deploys controller via Nomad job
echo "🚀 Testing controller deployment..."

run_test "Controller job exists in Nomad" "curl -f -s http://localhost:4646/v1/job/ploy-controller"
run_test "Controller allocations running" "nomad job status ploy-controller | grep -q running"

# Test 569: Controller binary distributed via SeaweedFS
echo "📦 Testing binary distribution..."

run_test "Controller binaries directory exists" "curl -f -s http://localhost:8888/controller-binaries/"
run_test "ployman CLI available" "test -x ./build/ployman"
run_test "controller-dist tool available" "test -x ./build/controller-dist"

# Test 570: High availability deployment
echo "🏃 Testing high availability..."

ALLOC_COUNT=$(nomad job status ploy-controller | grep -c "running" || echo "0")
if [ "$ALLOC_COUNT" -ge 2 ]; then
    echo -e "HA deployment: ${GREEN}PASS${NC} ($ALLOC_COUNT replicas)"
    TESTS_PASSED=$((TESTS_PASSED + 1))
else
    echo -e "HA deployment: ${YELLOW}WARN${NC} ($ALLOC_COUNT replicas, expected 2+)"
    TESTS_FAILED=$((TESTS_FAILED + 1))
fi
TOTAL_TESTS=$((TOTAL_TESTS + 1))

# Test 572: Health checks integrated with Nomad
echo "🏥 Testing health checks..."

run_test "Controller health endpoint" "curl -f -s http://localhost:8081/health"
run_test "Controller readiness endpoint" "curl -f -s http://localhost:8081/ready"
run_test "Consul service registration" "curl -f -s http://localhost:8500/v1/health/service/ploy-controller"

# Test 575-579: ployman CLI commands
echo "🛠️ Testing ployman CLI commands..."

# List available versions
run_test "ployman controller list" "./build/ployman controller list"

# Test current version info
if [ -f "/opt/ploy/current-controller-version" ]; then
    CURRENT_VERSION=$(cat /opt/ploy/current-controller-version)
    echo "Current controller version: $CURRENT_VERSION"
    TESTS_PASSED=$((TESTS_PASSED + 1))
else
    echo -e "Current version tracking: ${RED}FAIL${NC}"
    TESTS_FAILED=$((TESTS_FAILED + 1))
fi
TOTAL_TESTS=$((TOTAL_TESTS + 1))

# Test 582: Controller status script
echo "📊 Testing controller status script..."

run_test "Controller status script exists" "test -x /home/ploy/controller-scripts/controller-status.sh"
run_test "Controller status script runs" "/home/ploy/controller-scripts/controller-status.sh"

# Test 584: Management scripts directory
echo "📁 Testing management scripts..."

MANAGEMENT_SCRIPTS=(
    "update-controller.sh"
    "rollback-controller.sh" 
    "controller-status.sh"
    "migrate-controller.sh"
)

for script in "${MANAGEMENT_SCRIPTS[@]}"; do
    run_test "Script $script exists" "test -x /home/ploy/controller-scripts/$script"
done

# Test 586: Deployment validation
echo "✅ Testing deployment validation..."

# Verify no manual controller processes running
MANUAL_PROCESSES=$(pgrep -f "ploy.*controller|controller.*ploy" | grep -v "nomad\|docker" || true)
if [ -z "$MANUAL_PROCESSES" ]; then
    echo -e "No manual controller processes: ${GREEN}PASS${NC}"
    TESTS_PASSED=$((TESTS_PASSED + 1))
else
    echo -e "Manual controller processes found: ${RED}FAIL${NC}"
    TESTS_FAILED=$((TESTS_FAILED + 1))
fi
TOTAL_TESTS=$((TOTAL_TESTS + 1))

# Test API endpoints
echo "🌐 Testing API endpoints..."

API_ENDPOINTS=(
    "/health:200"
    "/ready:200"
    "/v1/health:200"
)

for endpoint_status in "${API_ENDPOINTS[@]}"; do
    endpoint="${endpoint_status%:*}"
    expected_status="${endpoint_status#*:}"
    run_test "API endpoint $endpoint" "curl -f -s -o /dev/null -w '%{http_code}' http://localhost:8081$endpoint | grep -q $expected_status"
done

# Test service discovery tags
echo "🔍 Testing service discovery..."

run_test "Traefik service tags" "curl -s http://localhost:8500/v1/health/service/ploy-controller | jq -r '.[0].Service.Tags[]' | grep -q traefik.enable=true"

# Summary
echo ""
echo "=== Test Summary ==="
echo -e "Tests passed: ${GREEN}$TESTS_PASSED${NC}"
echo -e "Tests failed: ${RED}$TESTS_FAILED${NC}"
echo "Total tests: $TOTAL_TESTS"

if [ $TESTS_FAILED -eq 0 ]; then
    echo -e "🎉 ${GREEN}All tests passed!${NC}"
    exit 0
else
    echo -e "❌ ${RED}Some tests failed${NC}"
    exit 1
fi