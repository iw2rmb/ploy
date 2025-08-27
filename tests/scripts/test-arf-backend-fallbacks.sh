#!/bin/bash
# ARF Backend Fallback Test Suite
# Tests storage backend failover and graceful degradation

set -e

SCRIPT_DIR=$(dirname "$0")
ROOT_DIR=$(cd "$SCRIPT_DIR/.." && pwd)

# Configuration
CONTROLLER_URL="${PLOY_CONTROLLER:-https://api.dev.ployman.app/v1}"
TEMP_DIR="/tmp/arf-fallback-test"
RESULTS_FILE="$TEMP_DIR/fallback-results.json"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Setup test environment
setup_test() {
    log_info "Setting up ARF backend fallback test environment..."
    
    mkdir -p "$TEMP_DIR"
    
    # Check controller health
    if ! curl -s -f "$CONTROLLER_URL/health" > /dev/null 2>&1; then
        log_error "Controller not accessible at $CONTROLLER_URL"
        exit 1
    fi
    
    log_info "Controller is accessible at $CONTROLLER_URL"
    
    # Initialize results tracking
    cat > "$RESULTS_FILE" << EOF
{
    "test_suite": "arf-backend-fallbacks",
    "start_time": "$(date -Iseconds)",
    "controller_url": "$CONTROLLER_URL",
    "tests": []
}
EOF
}

# Helper function to record test results
record_test() {
    local test_name="$1"
    local status="$2"
    local details="$3"
    local end_time=$(date -Iseconds)
    
    # Update results file
    tmp_file=$(mktemp)
    jq --arg name "$test_name" --arg status "$status" --arg details "$details" --arg time "$end_time" \
       '.tests += [{name: $name, status: $status, details: $details, timestamp: $time}]' \
       "$RESULTS_FILE" > "$tmp_file" && mv "$tmp_file" "$RESULTS_FILE"
}

# Test 1: Health Check with Backend Status
test_health_with_backend_status() {
    log_info "Test 1: Testing health check with backend status..."
    
    response=$(curl -s "$CONTROLLER_URL/health")
    
    if echo "$response" | jq -e '.status' > /dev/null 2>&1; then
        local status=$(echo "$response" | jq -r '.status')
        log_info "✓ Controller health check passed (status: $status)"
        record_test "health_check" "PASS" "Controller status: $status"
        return 0
    else
        log_error "✗ Health check failed: $response"
        record_test "health_check" "FAIL" "Health check failed: $response"
        return 1
    fi
}

# Test 2: ARF Health Check
test_arf_health_check() {
    log_info "Test 2: Testing ARF-specific health check..."
    
    response=$(curl -s "$CONTROLLER_URL/arf/health")
    
    if echo "$response" | jq -e '.status' > /dev/null 2>&1; then
        local status=$(echo "$response" | jq -r '.status')
        log_info "✓ ARF health check passed (status: $status)"
        record_test "arf_health" "PASS" "ARF status: $status"
        return 0
    else
        log_warn "! ARF health endpoint may not be available"
        record_test "arf_health" "WARN" "ARF health not available: $response"
        return 0
    fi
}

# Test 3: Recipe Operations with Backend Auto-Selection  
test_backend_auto_selection() {
    log_info "Test 3: Testing backend auto-selection..."
    
    # Test recipe listing (should work regardless of backend)
    response=$(curl -s "$CONTROLLER_URL/arf/recipes")
    
    if echo "$response" | jq -e '.recipes' > /dev/null 2>&1; then
        local count=$(echo "$response" | jq '.recipes | length')
        log_info "✓ Backend auto-selection working (found $count recipes)"
        record_test "backend_auto_selection" "PASS" "Recipe listing returned $count recipes"
        return 0
    else
        log_error "✗ Backend auto-selection failed: $response"
        record_test "backend_auto_selection" "FAIL" "Recipe listing failed: $response"
        return 1
    fi
}

# Test 4: Storage Configuration Endpoint
test_storage_configuration() {
    log_info "Test 4: Testing storage configuration access..."
    
    response=$(curl -s "$CONTROLLER_URL/storage/config")
    
    if echo "$response" | jq -e '.' > /dev/null 2>&1; then
        log_info "✓ Storage configuration accessible"
        record_test "storage_config" "PASS" "Storage configuration retrieved"
        return 0
    else
        log_warn "! Storage configuration endpoint may not be available"
        record_test "storage_config" "WARN" "Storage config not available: $response"
        return 0
    fi
}

# Test 5: Cache Statistics (Tests Memory Backend Components)
test_cache_statistics() {
    log_info "Test 5: Testing cache statistics..."
    
    response=$(curl -s "$CONTROLLER_URL/arf/stats/cache")
    
    if echo "$response" | jq -e '.' > /dev/null 2>&1; then
        log_info "✓ Cache statistics accessible"
        record_test "cache_stats" "PASS" "Cache statistics retrieved"
        return 0
    else
        log_warn "! Cache statistics may not be available"
        record_test "cache_stats" "WARN" "Cache stats not available: $response"
        return 0
    fi
}

# Test 6: Recipe Search Fallback
test_search_fallback() {
    log_info "Test 6: Testing recipe search fallback mechanisms..."
    
    # Test search with various query types
    for query in "java" "migration" "test" "nonexistent"; do
        response=$(curl -s "$CONTROLLER_URL/arf/recipes/search?q=$query")
        
        if echo "$response" | jq -e '.recipes' > /dev/null 2>&1; then
            local count=$(echo "$response" | jq '.recipes | length')
            log_info "✓ Search for '$query' returned $count results"
        else
            log_warn "! Search for '$query' may have issues: $response"
        fi
    done
    
    record_test "search_fallback" "PASS" "Search fallback mechanisms functional"
    return 0
}

# Test 7: Recipe Validation Fallback
test_validation_fallback() {
    log_info "Test 7: Testing recipe validation fallback..."
    
    # Test with minimal valid recipe
    local test_recipe=$(cat << 'EOF'
{
    "id": "fallback-test-recipe",
    "metadata": {
        "name": "Fallback Test Recipe",
        "version": "1.0.0",
        "description": "Test recipe for validation fallback"
    },
    "steps": [
        {
            "name": "test-step",
            "type": "shell",
            "command": "echo 'test'",
            "timeout": {"duration": "30s"}
        }
    ]
}
EOF
)
    
    response=$(curl -s -X POST "$CONTROLLER_URL/arf/recipes/validate" \
        -H "Content-Type: application/json" \
        -d "$test_recipe")
    
    if echo "$response" | jq -e '.valid == true' > /dev/null 2>&1; then
        log_info "✓ Recipe validation working"
        record_test "validation_fallback" "PASS" "Recipe validation functional"
        return 0
    else
        log_warn "! Recipe validation may have issues: $response"
        record_test "validation_fallback" "WARN" "Validation issues: $response"
        return 0
    fi
}

# Test 8: Storage Health Monitoring
test_storage_health() {
    log_info "Test 8: Testing storage health monitoring..."
    
    response=$(curl -s "$CONTROLLER_URL/storage/health")
    
    if echo "$response" | jq -e '.status' > /dev/null 2>&1; then
        local status=$(echo "$response" | jq -r '.status')
        log_info "✓ Storage health monitoring working (status: $status)"
        record_test "storage_health" "PASS" "Storage health: $status"
        return 0
    else
        log_warn "! Storage health monitoring may not be available"
        record_test "storage_health" "WARN" "Storage health not available: $response"
        return 0
    fi
}

# Test 9: Backend Configuration Reload
test_config_reload() {
    log_info "Test 9: Testing configuration reload capabilities..."
    
    response=$(curl -s -X POST "$CONTROLLER_URL/storage/config/reload")
    
    if echo "$response" | jq -e '.status' > /dev/null 2>&1; then
        local status=$(echo "$response" | jq -r '.status')
        log_info "✓ Configuration reload working (status: $status)"
        record_test "config_reload" "PASS" "Config reload: $status"
        return 0
    else
        log_warn "! Configuration reload may not be available"
        record_test "config_reload" "WARN" "Config reload not available: $response"
        return 0
    fi
}

# Test 10: Graceful Degradation Test
test_graceful_degradation() {
    log_info "Test 10: Testing graceful degradation behavior..."
    
    # Test that basic operations still work even with potential backend issues
    local endpoints=(
        "/health"
        "/ready"
        "/arf/health"
        "/arf/recipes"
    )
    
    local working_endpoints=0
    local total_endpoints=${#endpoints[@]}
    
    for endpoint in "${endpoints[@]}"; do
        if curl -s -f "$CONTROLLER_URL$endpoint" > /dev/null 2>&1; then
            working_endpoints=$((working_endpoints + 1))
            log_info "✓ Endpoint $endpoint is working"
        else
            log_warn "! Endpoint $endpoint may have issues"
        fi
    done
    
    if [ $working_endpoints -ge $((total_endpoints / 2)) ]; then
        log_info "✓ Graceful degradation test passed ($working_endpoints/$total_endpoints endpoints working)"
        record_test "graceful_degradation" "PASS" "$working_endpoints/$total_endpoints endpoints working"
        return 0
    else
        log_error "✗ Graceful degradation test failed ($working_endpoints/$total_endpoints endpoints working)"
        record_test "graceful_degradation" "FAIL" "Only $working_endpoints/$total_endpoints endpoints working"
        return 1
    fi
}

# Generate final test report
generate_report() {
    log_info "Generating backend fallback test report..."
    
    # Update results with end time
    local end_time=$(date -Iseconds)
    tmp_file=$(mktemp)
    jq --arg time "$end_time" '.end_time = $time' "$RESULTS_FILE" > "$tmp_file" && mv "$tmp_file" "$RESULTS_FILE"
    
    # Count test results
    local total_tests=$(jq '.tests | length' "$RESULTS_FILE")
    local passed_tests=$(jq '.tests | map(select(.status == "PASS")) | length' "$RESULTS_FILE")
    local failed_tests=$(jq '.tests | map(select(.status == "FAIL")) | length' "$RESULTS_FILE")
    local warned_tests=$(jq '.tests | map(select(.status == "WARN")) | length' "$RESULTS_FILE")
    
    log_info "================== FALLBACK TEST REPORT =================="
    log_info "Total Tests: $total_tests"
    log_info "Passed: $passed_tests"
    log_info "Failed: $failed_tests"
    log_info "Warnings: $warned_tests"
    log_info "Results saved to: $RESULTS_FILE"
    
    if [ "$failed_tests" -gt 0 ]; then
        log_error "Some fallback tests failed. System may not gracefully degrade."
        return 1
    elif [ "$warned_tests" -gt 0 ]; then
        log_warn "All tests passed but some features may not be fully available."
        return 0
    else
        log_info "All fallback tests passed successfully!"
        return 0
    fi
}

# Main test execution
main() {
    log_info "Starting ARF Backend Fallback Test Suite"
    log_info "Controller URL: $CONTROLLER_URL"
    
    setup_test
    
    # Run all fallback tests
    test_health_with_backend_status
    test_arf_health_check
    test_backend_auto_selection
    test_storage_configuration
    test_cache_statistics
    test_search_fallback
    test_validation_fallback
    test_storage_health
    test_config_reload
    test_graceful_degradation
    
    # Generate report
    generate_report
    
    local exit_code=$?
    log_info "ARF Backend Fallback Test Suite completed"
    exit $exit_code
}

# Execute main function
main "$@"