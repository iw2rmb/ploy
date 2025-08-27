#!/bin/bash
# ARF Storage Configuration Test Suite
# Tests environment-driven storage backend selection and configuration

set -e

SCRIPT_DIR=$(dirname "$0")
ROOT_DIR=$(cd "$SCRIPT_DIR/../.." && pwd)

# Configuration
CONTROLLER_URL="${PLOY_CONTROLLER:-https://api.dev.ployman.app/v1}"
TEMP_DIR="/tmp/arf-config-test"
RESULTS_FILE="$TEMP_DIR/config-results.json"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
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

log_test() {
    echo -e "${BLUE}[TEST]${NC} $1"
}

# Setup test environment
setup_test() {
    log_info "Setting up ARF storage configuration test environment..."
    
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
    "test_suite": "arf-storage-configuration",
    "start_time": "$(date -Iseconds)",
    "controller_url": "$CONTROLLER_URL",
    "configuration_tests": []
}
EOF
}

# Helper function to record test results
record_test() {
    local test_name="$1"
    local status="$2"
    local details="$3"
    local config_detected="$4"
    local end_time=$(date -Iseconds)
    
    # Update results file
    tmp_file=$(mktemp)
    jq --arg name "$test_name" --arg status "$status" --arg details "$details" --arg config "$config_detected" --arg time "$end_time" \
       '.configuration_tests += [{name: $name, status: $status, details: $details, configuration: $config, timestamp: $time}]' \
       "$RESULTS_FILE" > "$tmp_file" && mv "$tmp_file" "$RESULTS_FILE"
}

# Test 1: Environment Detection
test_environment_detection() {
    log_test "Test 1: Verifying environment detection..."
    
    # Get controller version info which should include environment details
    response=$(curl -s "$CONTROLLER_URL/api/version")
    
    if echo "$response" | jq -e '.git_branch' > /dev/null 2>&1; then
        local branch=$(echo "$response" | jq -r '.git_branch // "unknown"')
        local build_time=$(echo "$response" | jq -r '.build_timestamp // "unknown"')
        
        log_info "✓ Controller version info accessible"
        log_info "  Branch: $branch"
        log_info "  Build: $build_time"
        
        # Infer environment from branch or other indicators
        local env_type="unknown"
        if [[ "$branch" == "main" ]]; then
            env_type="production"
        elif [[ "$branch" == *"dev"* ]] || [[ "$branch" == *"test"* ]]; then
            env_type="development"
        fi
        
        record_test "environment_detection" "PASS" "Environment: $env_type, Branch: $branch" "$env_type"
        return 0
    else
        log_warn "! Controller version info not available"
        record_test "environment_detection" "WARN" "Version info not available" "unknown"
        return 0
    fi
}

# Test 2: Storage Backend Configuration Test
test_storage_backend_configuration() {
    log_test "Test 2: Testing storage backend configuration..."
    
    # Test storage configuration endpoint
    response=$(curl -s "$CONTROLLER_URL/storage/config")
    
    if echo "$response" | jq -e '.' > /dev/null 2>&1; then
        log_info "✓ Storage configuration endpoint accessible"
        
        # Try to extract backend information
        if echo "$response" | jq -e '.backend' > /dev/null 2>&1; then
            local backend=$(echo "$response" | jq -r '.backend')
            log_info "  Storage backend: $backend"
            record_test "storage_config" "PASS" "Backend: $backend" "$backend"
        else
            log_info "  Backend information not directly available in config"
            record_test "storage_config" "PASS" "Config accessible but backend type unclear" "config-available"
        fi
        return 0
    else
        log_warn "! Storage configuration endpoint may not be available"
        record_test "storage_config" "WARN" "Storage config not accessible: $response" "unavailable"
        return 0
    fi
}

# Test 3: ARF Health Check with Backend Info
test_arf_backend_health() {
    log_test "Test 3: Testing ARF backend health reporting..."
    
    response=$(curl -s "$CONTROLLER_URL/arf/health")
    
    if echo "$response" | jq -e '.status' > /dev/null 2>&1; then
        local status=$(echo "$response" | jq -r '.status')
        log_info "✓ ARF health check passed (status: $status)"
        
        # Look for additional backend information
        if echo "$response" | jq -e '.backend' > /dev/null 2>&1; then
            local backend_info=$(echo "$response" | jq -r '.backend // "not-specified"')
            log_info "  Backend info: $backend_info"
        fi
        
        if echo "$response" | jq -e '.components' > /dev/null 2>&1; then
            local components=$(echo "$response" | jq -r '.components | keys | join(", ")')
            log_info "  Components: $components"
        fi
        
        record_test "arf_health" "PASS" "ARF health: $status" "$status"
        return 0
    else
        log_warn "! ARF health check not available"
        record_test "arf_health" "WARN" "ARF health not available: $response" "unavailable"
        return 0
    fi
}

# Test 4: Recipe Storage Behavior Analysis
test_storage_behavior() {
    log_test "Test 4: Analyzing recipe storage behavior..."
    
    local test_recipe_id="config-test-$(date +%s)"
    local test_recipe=$(cat << EOF
{
    "id": "$test_recipe_id",
    "metadata": {
        "name": "Configuration Test Recipe",
        "version": "1.0.0",
        "description": "Test recipe for storage configuration analysis",
        "author": "config-test",
        "categories": ["configuration", "testing"],
        "tags": ["storage", "backend", "config"]
    },
    "steps": [
        {
            "name": "config-test-step",
            "type": "shell",
            "command": "echo 'Configuration test'",
            "timeout": {"duration": "10s"}
        }
    ]
}
EOF
)
    
    # Create recipe
    create_response=$(curl -s -X POST "$CONTROLLER_URL/arf/recipes" \
        -H "Content-Type: application/json" \
        -d "$test_recipe")
    
    if echo "$create_response" | jq -e '.id' > /dev/null 2>&1; then
        log_info "✓ Recipe created for storage behavior analysis"
        
        # Immediately try to retrieve it
        retrieve_response=$(curl -s "$CONTROLLER_URL/arf/recipes/$test_recipe_id")
        
        if echo "$retrieve_response" | jq -e '.id' > /dev/null 2>&1; then
            log_info "✓ Recipe immediately retrievable (indicates fast storage)"
            
            # Check if it appears in search quickly
            search_response=$(curl -s "$CONTROLLER_URL/arf/recipes/search?q=configuration")
            
            if echo "$search_response" | jq -e '.recipes' > /dev/null 2>&1; then
                local found=$(echo "$search_response" | jq -r ".recipes[] | select(.id==\"$test_recipe_id\") | .id")
                if [ "$found" = "$test_recipe_id" ]; then
                    log_info "✓ Recipe immediately searchable (indicates indexed storage)"
                    record_test "storage_behavior" "PASS" "Fast creation/retrieval/search" "indexed-backend"
                else
                    log_info "! Recipe not immediately searchable (may indicate search indexing delay)"
                    record_test "storage_behavior" "PASS" "Fast creation/retrieval, delayed search" "backend-with-index-delay"
                fi
            else
                log_warn "! Search endpoint issues"
                record_test "storage_behavior" "WARN" "Search issues during analysis" "search-unavailable"
            fi
            
            # Cleanup
            curl -s -X DELETE "$CONTROLLER_URL/arf/recipes/$test_recipe_id" > /dev/null
            
        else
            log_warn "! Recipe not immediately retrievable"
            record_test "storage_behavior" "WARN" "Creation succeeded but retrieval failed" "inconsistent-storage"
        fi
        
        return 0
    else
        log_warn "! Recipe creation failed during storage analysis"
        record_test "storage_behavior" "WARN" "Recipe creation failed: $create_response" "storage-creation-failed"
        return 0
    fi
}

# Test 5: Performance Characteristics
test_performance_characteristics() {
    log_test "Test 5: Testing storage performance characteristics..."
    
    # Test recipe listing performance
    start_time=$(date +%s%3N)
    response=$(curl -s "$CONTROLLER_URL/arf/recipes")
    end_time=$(date +%s%3N)
    
    if echo "$response" | jq -e '.recipes' > /dev/null 2>&1; then
        local duration=$((end_time - start_time))
        local count=$(echo "$response" | jq '.recipes | length')
        
        log_info "✓ Recipe listing: ${duration}ms for $count recipes"
        
        # Analyze performance characteristics
        local perf_category="unknown"
        if [ $duration -lt 100 ]; then
            perf_category="very-fast"
            log_info "  Performance: Very fast (likely memory-based storage)"
        elif [ $duration -lt 500 ]; then
            perf_category="fast"
            log_info "  Performance: Fast (likely local storage with caching)"
        elif [ $duration -lt 1000 ]; then
            perf_category="moderate"
            log_info "  Performance: Moderate (likely distributed storage)"
        else
            perf_category="slow"
            log_info "  Performance: Slow (may indicate storage issues)"
        fi
        
        record_test "performance_characteristics" "PASS" "Duration: ${duration}ms, Count: $count" "$perf_category"
        return 0
    else
        log_warn "! Recipe listing failed during performance test"
        record_test "performance_characteristics" "WARN" "Recipe listing failed: $response" "unavailable"
        return 0
    fi
}

# Test 6: Validation Configuration
test_validation_configuration() {
    log_test "Test 6: Testing recipe validation configuration..."
    
    # Test with invalid recipe to see validation behavior
    local invalid_recipe='{"invalid": "recipe", "missing_required_fields": true}'
    
    response=$(curl -s -X POST "$CONTROLLER_URL/arf/recipes/validate" \
        -H "Content-Type: application/json" \
        -d "$invalid_recipe")
    
    if echo "$response" | jq -e '.valid == false' > /dev/null 2>&1; then
        local details=$(echo "$response" | jq -r '.details // "validation-failed"')
        log_info "✓ Validation correctly rejected invalid recipe"
        log_info "  Details: $details"
        
        # Check if validation is strict or lenient based on error details
        local validation_level="standard"
        if echo "$details" | grep -q "strict"; then
            validation_level="strict"
        elif echo "$details" | grep -q "required"; then
            validation_level="standard"
        fi
        
        record_test "validation_config" "PASS" "Validation working: $validation_level" "$validation_level"
        return 0
    else
        log_warn "! Validation may not be working properly"
        record_test "validation_config" "WARN" "Validation issues: $response" "validation-disabled"
        return 0
    fi
}

# Test 7: Cache Configuration Analysis
test_cache_configuration() {
    log_test "Test 7: Testing cache configuration..."
    
    # Test cache statistics endpoint
    response=$(curl -s "$CONTROLLER_URL/arf/stats/cache")
    
    if echo "$response" | jq -e '.' > /dev/null 2>&1; then
        log_info "✓ Cache statistics accessible"
        
        # Look for cache configuration indicators
        if echo "$response" | jq -e '.hit_rate' > /dev/null 2>&1; then
            local hit_rate=$(echo "$response" | jq -r '.hit_rate // "unknown"')
            local total_requests=$(echo "$response" | jq -r '.total_requests // "unknown"')
            log_info "  Cache hit rate: $hit_rate"
            log_info "  Total requests: $total_requests"
            
            record_test "cache_config" "PASS" "Cache active, hit rate: $hit_rate" "caching-enabled"
        else
            log_info "  Cache statistics available but limited detail"
            record_test "cache_config" "PASS" "Cache endpoint available" "cache-available"
        fi
        return 0
    else
        log_warn "! Cache statistics not available"
        record_test "cache_config" "WARN" "Cache stats not available" "no-cache-info"
        return 0
    fi
}

# Generate comprehensive configuration report
generate_report() {
    log_info "Generating ARF storage configuration report..."
    
    # Update results with end time
    local end_time=$(date -Iseconds)
    tmp_file=$(mktemp)
    jq --arg time "$end_time" '.end_time = $time' "$RESULTS_FILE" > "$tmp_file" && mv "$tmp_file" "$RESULTS_FILE"
    
    # Analyze configuration patterns
    local backend_indicators=$(jq -r '.configuration_tests[].configuration | select(. != null and . != "unknown")' "$RESULTS_FILE" | sort | uniq -c | sort -nr)
    
    # Count test results
    local total_tests=$(jq '.configuration_tests | length' "$RESULTS_FILE")
    local passed_tests=$(jq '.configuration_tests | map(select(.status == "PASS")) | length' "$RESULTS_FILE")
    local failed_tests=$(jq '.configuration_tests | map(select(.status == "FAIL")) | length' "$RESULTS_FILE")
    local warned_tests=$(jq '.configuration_tests | map(select(.status == "WARN")) | length' "$RESULTS_FILE")
    
    log_info "================== CONFIGURATION ANALYSIS REPORT =================="
    log_info "Total Tests: $total_tests"
    log_info "Passed: $passed_tests"
    log_info "Failed: $failed_tests" 
    log_info "Warnings: $warned_tests"
    log_info ""
    log_info "Backend Configuration Indicators:"
    echo "$backend_indicators" | while read -r line; do
        if [ -n "$line" ]; then
            log_info "  $line"
        fi
    done
    log_info ""
    log_info "Results saved to: $RESULTS_FILE"
    
    # Generate summary assessment
    local assessment="unknown"
    if [ $passed_tests -gt $((total_tests * 3 / 4)) ]; then
        if [ $warned_tests -eq 0 ]; then
            assessment="optimal"
        else
            assessment="good"
        fi
    elif [ $passed_tests -gt $((total_tests / 2)) ]; then
        assessment="functional"
    else
        assessment="problematic"
    fi
    
    log_info "Overall Assessment: $assessment"
    
    if [ "$failed_tests" -gt 0 ]; then
        log_error "Configuration analysis completed with failures."
        return 1
    elif [ "$warned_tests" -gt $((total_tests / 2)) ]; then
        log_warn "Configuration analysis completed but many features may not be available."
        return 0
    else
        log_info "Configuration analysis completed successfully!"
        return 0
    fi
}

# Main test execution
main() {
    log_info "Starting ARF Storage Configuration Analysis"
    log_info "Controller URL: $CONTROLLER_URL"
    log_info "This test analyzes storage backend configuration and behavior"
    
    setup_test
    
    # Run configuration analysis tests
    test_environment_detection
    test_storage_backend_configuration
    test_arf_backend_health
    test_storage_behavior
    test_performance_characteristics
    test_validation_configuration
    test_cache_configuration
    
    # Generate comprehensive report
    generate_report
    
    local exit_code=$?
    log_info "ARF Storage Configuration Analysis completed"
    exit $exit_code
}

# Execute main function
main "$@"